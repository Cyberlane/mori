package support

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const bundleReadme = "Mori support session, schema 1.\nThis opt-in artifact contains allowlisted build metadata, aggregate scan settings/counts/timing, and optional reviewed finding ranks.\nIt contains no source, paths, function names, repository identifiers, environment variables, raw errors, or command arguments.\nInspect session.json before manually sharing. Nothing is uploaded by Mori.\n"

// Marshal returns canonical, reviewable JSON after strict export validation.
func Marshal(s Session) ([]byte, error) {
	if err := Validate(s); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(canonical(s), "", "  ")
	if err != nil {
		return nil, errInvalid
	}
	data = append(data, '\n')
	if len(data) > MaxSessionBytes {
		return nil, errInvalid
	}
	return data, nil
}
func WriteSession(path string, s Session) error {
	data, err := Marshal(s)
	if err != nil {
		return err
	}
	return writePrivate(path, data)
}
func ReadSession(path string) (Session, error) {
	data, err := readRegular(path, MaxSessionBytes)
	if err != nil {
		return Session{}, err
	}
	return decode(data)
}
func WriteBundle(path string, s Session) error {
	data, err := bundle(s)
	if err != nil {
		return err
	}
	return writePrivate(path, data)
}
func ReadBundle(path string) (Session, error) {
	data, err := readRegular(path, MaxBundleBytes)
	if err != nil {
		return Session{}, err
	}
	return decodeBundle(data)
}
func Inspect(path string) (Session, error) {
	data, err := readRegular(path, MaxBundleBytes)
	if err != nil {
		return Session{}, err
	}
	if bytes.HasPrefix(data, []byte("PK")) {
		return decodeBundle(data)
	}
	return decode(data)
}

func decode(data []byte) (Session, error) {
	if len(data) == 0 || len(data) > MaxSessionBytes {
		return Session{}, errInvalid
	}
	// Token traversal rejects duplicate keys, excessive nesting, and trailing JSON.
	tokenDecoder := json.NewDecoder(bytes.NewReader(data))
	tokenDecoder.UseNumber()
	if err := uniqueJSON(tokenDecoder, 0); err != nil {
		return Session{}, errInvalid
	}
	if _, err := tokenDecoder.Token(); err != io.EOF {
		return Session{}, errInvalid
	}
	var s Session
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return Session{}, errInvalid
	}
	var raw any
	if json.Unmarshal(data, &raw) != nil || !required(raw, reflect.TypeOf(s)) {
		return Session{}, errInvalid
	}
	if err := Validate(s); err != nil {
		return Session{}, err
	}
	return canonical(s), nil
}
func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 12 {
		return errInvalid
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errInvalid
			}
			seen[name] = true
			if err := uniqueJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errInvalid
	}
	_, err = d.Token()
	return err
}

// required enforces the same required-field/nullability contract as the schema.
func required(raw any, t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		if raw == nil {
			return true
		}
		return required(raw, t.Elem())
	}
	if raw == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Struct:
		object, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		allowed := make(map[string]bool, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			allowed[strings.Split(t.Field(i).Tag.Get("json"), ",")[0]] = true
		}
		for key := range object {
			if !allowed[key] {
				return false
			}
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			parts := strings.Split(field.Tag.Get("json"), ",")
			value, exists := object[parts[0]]
			if !exists {
				if len(parts) > 1 && parts[1] == "omitempty" {
					continue
				}
				return false
			}
			if !required(value, field.Type) {
				return false
			}
		}
	case reflect.Slice:
		items, ok := raw.([]any)
		if !ok {
			return false
		}
		for _, item := range items {
			if !required(item, t.Elem()) {
				return false
			}
		}
	}
	return true
}
func bundle(s Session) ([]byte, error) {
	data, err := Marshal(s)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, entry := range []struct {
		name string
		data []byte
	}{{"session.json", data}, {"README.txt", []byte(bundleReadme)}} {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		header.SetMode(0600)
		f, err := w.CreateHeader(header)
		if err != nil {
			return nil, errInvalid
		}
		if _, err = f.Write(entry.data); err != nil {
			return nil, errInvalid
		}
	}
	if err := w.Close(); err != nil {
		return nil, errInvalid
	}
	if buf.Len() > MaxBundleBytes {
		return nil, errInvalid
	}
	return buf.Bytes(), nil
}
func decodeBundle(data []byte) (Session, error) {
	if len(data) > MaxBundleBytes {
		return Session{}, errInvalid
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(r.File) != 2 || r.Comment != "" {
		return Session{}, errInvalid
	}
	var session []byte
	seen := map[string]bool{}
	for _, f := range r.File {
		if seen[f.Name] || (f.Name != "session.json" && f.Name != "README.txt") || !f.Mode().IsRegular() || f.Method != zip.Store || f.Comment != "" || f.UncompressedSize64 > MaxSessionBytes {
			return Session{}, errInvalid
		}
		seen[f.Name] = true
		stream, err := f.Open()
		if err != nil {
			return Session{}, errInvalid
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, MaxSessionBytes+1))
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil || len(content) > MaxSessionBytes {
			return Session{}, errInvalid
		}
		if f.Name == "session.json" {
			session = content
		} else if string(content) != bundleReadme {
			return Session{}, errInvalid
		}
	}
	s, err := decode(session)
	if err != nil {
		return Session{}, err
	}
	canonicalBundle, err := bundle(s)
	if err != nil || !bytes.Equal(data, canonicalBundle) {
		return Session{}, errInvalid
	}
	return s, nil
}

var errRead = errors.New("cannot read support artifact: expected a bounded regular file without symlinks")
var errWrite = errors.New("cannot write support artifact: output must be new and its parent must exist")

func cleanPath(path string) (string, error) {
	if path == "" {
		return "", errRead
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errRead
	}
	// Resolve existing ancestor directories first (including macOS /tmp and
	// /var aliases). The final component is deliberately not resolved.
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", errRead
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", errRead
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func readRegular(path string, limit int64) ([]byte, error) {
	path, err := cleanPath(path)
	if err != nil {
		return nil, errRead
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, errRead
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errRead
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, errRead
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errRead
	}
	return data, nil
}
func writePrivate(path string, data []byte) error {
	path, err := cleanPath(path)
	if err != nil {
		return errWrite
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return errWrite
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".mori-support-*")
	if err != nil {
		return errWrite
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return errWrite
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return errWrite
	}
	if err = f.Close(); err != nil {
		return errWrite
	}
	// Linking the completed private file is atomic and refuses an existing target.
	if err = os.Link(tmp, path); err != nil {
		return errWrite
	}
	return nil
}
