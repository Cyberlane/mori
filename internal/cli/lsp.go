package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cyberlane/mori/internal/model"
)

const (
	maxLSPMessageBytes = 16 * 1024 * 1024
	lspDebounce        = 350 * time.Millisecond
)

type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type lspDocument struct {
	URI     string
	Path    string
	Version int
	Text    []byte
}

type lspServer struct {
	ctx       context.Context
	writer    io.Writer
	stderr    io.Writer
	writeMu   sync.Mutex
	stateMu   sync.Mutex
	root      string
	documents map[string]lspDocument
	cancels   map[string]context.CancelFunc
	closed    bool
}

func runLSP(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		return usageError(stderr, "lsp does not accept arguments; communicate over standard input/output")
	}
	server := &lspServer{ctx: ctx, writer: stdout, stderr: stderr, documents: map[string]lspDocument{}, cancels: map[string]context.CancelFunc{}}
	reader := bufio.NewReader(stdin)
	for {
		content, err := readLSPFrame(reader)
		if errors.Is(err, io.EOF) {
			return exitSuccess
		}
		if err != nil {
			fmt.Fprintf(stderr, "mori: lsp: %v\n", err)
			return exitError
		}
		var message lspMessage
		if err := json.Unmarshal(content, &message); err != nil {
			server.respondError(nil, -32700, "invalid JSON")
			continue
		}
		if stop := server.handle(message); stop {
			return exitSuccess
		}
	}
}

func readLSPFrame(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			return nil, errors.New("malformed protocol header")
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 0 || parsed > maxLSPMessageBytes {
				return nil, errors.New("invalid Content-Length")
			}
			contentLength = parsed
		}
	}
	if contentLength < 0 {
		return nil, errors.New("missing Content-Length")
	}
	content := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, content); err != nil {
		return nil, err
	}
	return content, nil
}

func (server *lspServer) handle(message lspMessage) bool {
	switch message.Method {
	case "initialize":
		var params struct {
			RootURI          string `json:"rootUri"`
			WorkspaceFolders []struct {
				URI string `json:"uri"`
			} `json:"workspaceFolders"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			server.respondError(message.ID, -32602, "invalid initialize parameters")
			return false
		}
		rootURI := params.RootURI
		if len(params.WorkspaceFolders) > 0 {
			rootURI = params.WorkspaceFolders[0].URI
		}
		if path, err := fileURIPath(rootURI); err == nil && path != "" {
			server.root = path
		} else if cwd, err := os.Getwd(); err == nil {
			server.root = cwd
		}
		server.respond(message.ID, map[string]any{
			"capabilities": map[string]any{"textDocumentSync": map[string]any{"openClose": true, "change": 1, "save": map[string]any{"includeText": true}}},
			"serverInfo":   map[string]any{"name": "mori", "version": "local"},
		})
	case "shutdown":
		server.stateMu.Lock()
		server.closed = true
		for _, cancel := range server.cancels {
			cancel()
		}
		server.stateMu.Unlock()
		server.respond(message.ID, nil)
	case "exit":
		server.stateMu.Lock()
		closed := server.closed
		server.stateMu.Unlock()
		return closed
	case "textDocument/didOpen":
		var params struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
				Text    string `json:"text"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			server.updateDocument(params.TextDocument.URI, params.TextDocument.Version, []byte(params.TextDocument.Text))
		}
	case "textDocument/didChange":
		var params struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(message.Params, &params) == nil && len(params.ContentChanges) > 0 {
			server.updateDocument(params.TextDocument.URI, params.TextDocument.Version, []byte(params.ContentChanges[len(params.ContentChanges)-1].Text))
		}
	case "textDocument/didSave":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Text *string `json:"text,omitempty"`
		}
		if json.Unmarshal(message.Params, &params) == nil && params.Text != nil {
			server.stateMu.Lock()
			document, ok := server.documents[params.TextDocument.URI]
			server.stateMu.Unlock()
			if ok {
				server.updateDocument(document.URI, document.Version, []byte(*params.Text))
			}
		}
	case "textDocument/didClose":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(message.Params, &params) == nil {
			server.closeDocument(params.TextDocument.URI)
		}
	default:
		if len(message.ID) > 0 {
			server.respondError(message.ID, -32601, "method not found")
		}
	}
	return false
}

func (server *lspServer) updateDocument(uri string, version int, text []byte) {
	path, err := fileURIPath(uri)
	if err != nil || path == "" || len(text) > maxStdinOverlayBytes {
		server.publish(uri, version, []lspDiagnostic{{Severity: 2, Source: "mori", Code: "MORI002", Message: "Mori could not analyze this document: invalid path or overlay exceeds 16 MiB."}})
		return
	}
	document := lspDocument{URI: uri, Path: path, Version: version, Text: append([]byte{}, text...)}
	server.stateMu.Lock()
	if cancel := server.cancels[uri]; cancel != nil {
		cancel()
	}
	analysisContext, cancel := context.WithCancel(server.ctx)
	server.cancels[uri] = cancel
	server.documents[uri] = document
	server.stateMu.Unlock()
	go func() {
		timer := time.NewTimer(lspDebounce)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-analysisContext.Done():
			return
		}
		server.analyzeDocument(analysisContext, document)
	}()
}

func (server *lspServer) closeDocument(uri string) {
	server.stateMu.Lock()
	if cancel := server.cancels[uri]; cancel != nil {
		cancel()
	}
	delete(server.cancels, uri)
	delete(server.documents, uri)
	server.stateMu.Unlock()
	server.publish(uri, 0, []lspDiagnostic{})
}

func (server *lspServer) analyzeDocument(ctx context.Context, document lspDocument) {
	root := server.root
	if root == "" {
		root = filepath.Dir(document.Path)
	}
	_, _, exists, options, err := projectConfiguration(root)
	if err != nil {
		server.publishIncomplete(document, err.Error())
		return
	}
	if !exists {
		options = defaultScanOptions()
		_ = applyScanProfile(&options, profileReview)
	}
	options.stdinPath = document.Path
	options.stdinContent = document.Text
	options.focusPaths = append(options.focusPaths, document.Path)
	result, err := executeScan(ctx, []string{root}, options, nil, nil)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			server.publishIncomplete(document, err.Error())
		}
		return
	}
	diagnostics := diagnosticsForLSPDocument(result, document.Path, root)
	server.stateMu.Lock()
	current, ok := server.documents[document.URI]
	server.stateMu.Unlock()
	if !ok || current.Version != document.Version {
		return
	}
	server.publish(document.URI, document.Version, diagnostics)
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}
type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}
type lspRelated struct {
	Location lspLocation `json:"location"`
	Message  string      `json:"message"`
}
type lspDiagnostic struct {
	Range    lspRange     `json:"range"`
	Severity int          `json:"severity"`
	Code     string       `json:"code"`
	Source   string       `json:"source"`
	Message  string       `json:"message"`
	Related  []lspRelated `json:"relatedInformation,omitempty"`
}

func diagnosticsForLSPDocument(report model.Report, documentPath, root string) []lspDiagnostic {
	diagnostics := []lspDiagnostic{}
	for _, group := range report.Groups {
		locations := []model.Location{}
		for _, profile := range group.Profiles {
			for _, occurrence := range profile.Occurrences {
				locations = append(locations, occurrence.Location)
			}
		}
		for _, location := range locations {
			if canonicalPath(resolveReportPath(location.Path, root)) != canonicalPath(documentPath) {
				continue
			}
			related := []lspRelated{}
			for _, other := range locations {
				if other == location {
					continue
				}
				related = append(related, lspRelated{Location: lspLocation{URI: pathFileURI(resolveReportPath(other.Path, root)), Range: locationRange(other)}, Message: fmt.Sprintf("Related %s fragment", other.Language)})
			}
			diagnostics = append(diagnostics, lspDiagnostic{Range: locationRange(location), Severity: 3, Code: "MORI001", Source: "mori", Message: fmt.Sprintf("%.1f%% structural similarity; review both locations (%s)", group.Similarity*100, group.ID), Related: related})
		}
	}
	for _, warning := range report.Warnings {
		if warning.Path != "" && canonicalPath(resolveReportPath(warning.Path, root)) != canonicalPath(documentPath) {
			continue
		}
		diagnostics = append(diagnostics, lspDiagnostic{Range: lspRange{}, Severity: 2, Code: "MORI002", Source: "mori", Message: warning.Message})
	}
	return diagnostics
}

func locationRange(location model.Location) lspRange {
	start := location.StartLine - 1
	if start < 0 {
		start = 0
	}
	end := location.EndLine
	if end < start {
		end = start
	}
	return lspRange{Start: lspPosition{Line: start}, End: lspPosition{Line: end}}
}

func resolveReportPath(path, root string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, filepath.FromSlash(path))
		if _, err := os.Lstat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(root, filepath.FromSlash(path))
}

func fileURIPath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "file" {
		return "", errors.New("only file URIs are supported")
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		path = "//" + parsed.Host + path
	}
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.Clean(filepath.FromSlash(path)), nil
}

func pathFileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func (server *lspServer) publishIncomplete(document lspDocument, detail string) {
	server.publish(document.URI, document.Version, []lspDiagnostic{{Severity: 2, Code: "MORI002", Source: "mori", Message: "Mori analysis is incomplete: " + detail}})
}

func (server *lspServer) publish(uri string, version int, diagnostics []lspDiagnostic) {
	server.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "version": version, "diagnostics": diagnostics})
}

func (server *lspServer) respond(id json.RawMessage, result any) {
	server.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}
func (server *lspServer) respondError(id json.RawMessage, code int, message string) {
	server.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
}
func (server *lspServer) notify(method string, params any) {
	server.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (server *lspServer) write(value any) {
	content, err := json.Marshal(value)
	if err != nil {
		return
	}
	server.writeMu.Lock()
	defer server.writeMu.Unlock()
	_, _ = fmt.Fprintf(server.writer, "Content-Length: %d\r\n\r\n", len(content))
	_, _ = io.Copy(server.writer, bytes.NewReader(content))
}
