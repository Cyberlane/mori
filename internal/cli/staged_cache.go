package cli

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/Cyberlane/mori/internal/cachefs"
	"github.com/Cyberlane/mori/internal/hookcontract"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/vcs"
)

const stagedCacheMaxBytes = 16 * 1024 * 1024
const stagedCacheRevision = 2

// Bound reconstruction independently of the serialized size.
const stagedCacheMaxPairs = 1_000_000

type stagedCachePayload struct {
	Report    model.Report     `json:"report"`
	Locations []model.Location `json:"locations"`
	PathPairs [][][2]int       `json:"path_pairs"`
}

type stagedAnalysisCache struct {
	path   string
	key    string
	secret []byte
}

// openStagedAnalysisCache is only used after normal compatibility and baseline
// validation, and only for an explicitly requested canonical immutable review.
// Every error is a cache miss: cache availability never changes review policy.
func openStagedAnalysisCache(ctx context.Context, paths []string, options scanOptions, baselineDigest string, warnings []model.Warning) (*stagedAnalysisCache, error) {
	if !options.staged || options.stagedSnapshot == nil || !options.focusedOnly || !options.includeFocused || options.stdinPath != "" || len(options.stdinContent) != 0 {
		return nil, errors.New("cache requires canonical immutable staged review")
	}
	executable, err := stagedCacheExecutableDigest()
	if err != nil {
		return nil, err
	}
	key, err := stagedAnalysisCacheKey(paths, options, baselineDigest, warnings, executable)
	if err != nil {
		return nil, err
	}
	path, err := vcs.LocalMetadataPath(ctx, *options.stagedSnapshot, "mori/analysis-cache-v1/report")
	if err != nil {
		return nil, err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	secretPath := filepath.Join(config, "mori", "analysis-cache-v1", "key")
	// A redirected user configuration location within the checkout must never
	// become a repository-controlled authority for cached analysis.
	for _, candidate := range []string{config, secretPath} {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(options.stagedSnapshot.Root, absolute)
		if err != nil || relative == "." || (relative != ".." && !stringsHasParentPrefix(relative)) {
			return nil, errors.New("cache authority must be outside the checkout")
		}
	}
	if err := stagedCachePrivateDirectory(filepath.Dir(secretPath)); err != nil {
		return nil, err
	}
	secret, err := stagedCacheSecret(secretPath)
	if err != nil {
		return nil, err
	}
	if err := stagedCachePrivateDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return &stagedAnalysisCache{path: path, key: key, secret: secret}, nil
}

func stringsHasParentPrefix(path string) bool {
	return len(path) > 3 && path[:3] == ".."+string(filepath.Separator)
}

// All scan option fields participate automatically. New unsupported field kinds
// disable caching rather than accidentally leaving an input outside the key.
func stagedAnalysisCacheKey(paths []string, options scanOptions, baselineDigest string, warnings []model.Warning, executable string) (string, error) {
	values := make(map[string]any)
	reflected := reflect.ValueOf(options)
	for i := 0; i < reflected.NumField(); i++ {
		name := reflected.Type().Field(i).Name
		if name == "diagnostics" || name == "diagnosticsPath" || name == "stagedSnapshot" || name == "stagedCache" || name == "outputPath" || name == "format" || name == "color" || name == "redactPaths" || name == "reviewReceiptPath" {
			continue
		}
		field := reflected.Field(i)
		switch field.Kind() {
		case reflect.Bool:
			values[name] = field.Bool()
		case reflect.String:
			values[name] = field.String()
		case reflect.Int, reflect.Int64:
			values[name] = field.Int()
		case reflect.Float64:
			values[name] = field.Float()
		case reflect.Slice:
			if field.Type().Elem().Kind() != reflect.String && field.Type().Elem().Kind() != reflect.Uint8 {
				return "", fmt.Errorf("unsupported cache option %s", name)
			}
			entries := make([]string, field.Len())
			for j := 0; j < field.Len(); j++ {
				if field.Type().Elem().Kind() == reflect.String {
					entries[j] = field.Index(j).String()
				} else {
					entries[j] = fmt.Sprint(field.Index(j).Uint())
				}
			}
			values[name] = entries
		default:
			return "", fmt.Errorf("unsupported cache option %s", name)
		}
	}
	payload := struct {
		Revision      int
		Executable    string
		Schema        int
		Normalization int
		Hook          string
		Paths         []string
		Options       map[string]any
		Snapshot      *vcs.IndexSnapshot
		Baseline      string
		Warnings      []model.Warning
	}{stagedCacheRevision, executable, model.SchemaVersion, normalize.Version, hookcontract.Revision, paths, values, options.stagedSnapshot, baselineDigest, warnings}
	content, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]), nil
}

func (cache *stagedAnalysisCache) Load() (model.Report, bool) {
	content, err := stagedCacheRead(cache.path, stagedCacheMaxBytes+sha256.Size)
	if err != nil || len(content) < sha256.Size {
		return model.Report{}, false
	}
	payload, signature := content[sha256.Size:], content[:sha256.Size]
	mac := hmac.New(sha256.New, cache.secret)
	mac.Write([]byte(cache.key))
	mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return model.Report{}, false
	}
	var stored stagedCachePayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return model.Report{}, false
	}
	report := stored.Report
	if report.SchemaVersion != model.SchemaVersion || len(stored.PathPairs) != len(report.Groups) {
		return model.Report{}, false
	}
	count := 0
	for index, pairs := range stored.PathPairs {
		if len(pairs) > stagedCacheMaxPairs-count {
			return model.Report{}, false
		}
		count += len(pairs)
		if pairs != nil {
			report.Groups[index].PathPairs = make([]model.LocationPair, len(pairs))
		}
		for pairIndex, pair := range pairs {
			if pair[0] < 0 || pair[0] >= len(stored.Locations) || pair[1] < 0 || pair[1] >= len(stored.Locations) {
				return model.Report{}, false
			}
			report.Groups[index].PathPairs[pairIndex] = model.LocationPair{Left: stored.Locations[pair[0]], Right: stored.Locations[pair[1]]}
		}
	}
	return report, true
}

func (cache *stagedAnalysisCache) Save(report model.Report) error {
	var buffer bytes.Buffer
	stored := stagedCachePayload{Report: report, PathPairs: make([][][2]int, len(report.Groups))}
	locations := make(map[model.Location]int)
	locationIndex := func(location model.Location) int {
		if index, ok := locations[location]; ok {
			return index
		}
		index := len(stored.Locations)
		locations[location] = index
		stored.Locations = append(stored.Locations, location)
		return index
	}
	count := 0
	for index, group := range report.Groups {
		if len(group.PathPairs) > stagedCacheMaxPairs-count {
			return errors.New("cache report exceeds pair bound")
		}
		count += len(group.PathPairs)
		if group.PathPairs != nil {
			stored.PathPairs[index] = make([][2]int, len(group.PathPairs))
		}
		for pairIndex, pair := range group.PathPairs {
			stored.PathPairs[index][pairIndex] = [2]int{locationIndex(pair.Left), locationIndex(pair.Right)}
		}
	}
	if err := json.NewEncoder(stagedCacheBoundedWriter{writer: &buffer, limit: stagedCacheMaxBytes}).Encode(stored); err != nil {
		return err
	}
	mac := hmac.New(sha256.New, cache.secret)
	mac.Write([]byte(cache.key))
	mac.Write(buffer.Bytes())
	temporary := cache.path + ".pending"
	file, err := stagedCacheCreatePending(temporary)
	if err != nil {
		return err
	}
	identity, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	renamed := false
	defer func() {
		if !renamed {
			stagedCacheRemoveOwnedPending(temporary, identity)
		}
	}()
	if _, err = file.Write(mac.Sum(nil)); err == nil {
		_, err = file.Write(buffer.Bytes())
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	err = os.Rename(temporary, cache.path)
	renamed = err == nil
	return err
}

// Recover only old, private regular pending files. One retry keeps concurrent
// writers bounded; recent writes and symlinks are never deliberately removed.
func stagedCacheCreatePending(path string) (*os.File, error) {
	file, err := cachefs.Create(path)
	if !errors.Is(err, os.ErrExist) {
		return file, err
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || !info.Mode().IsRegular() || !cachefs.Private(path, info) || time.Since(info.ModTime()) <= 10*time.Minute {
		return nil, err
	}
	opened, openErr := os.Open(path)
	if openErr != nil {
		return nil, err
	}
	identity, identityErr := opened.Stat()
	opened.Close()
	if identityErr != nil || !os.SameFile(info, identity) {
		return nil, err
	}
	current, currentErr := os.Lstat(path)
	if currentErr != nil || !os.SameFile(info, current) || !current.Mode().IsRegular() || !cachefs.Private(path, current) || !current.ModTime().Equal(info.ModTime()) || current.Size() != info.Size() || time.Since(current.ModTime()) <= 10*time.Minute {
		return nil, err
	}
	if removeErr := os.Remove(path); removeErr != nil {
		return nil, err
	}
	return cachefs.Create(path)
}

func stagedCacheRemoveOwnedPending(path string, identity os.FileInfo) {
	current, err := os.Lstat(path)
	if err == nil && current.Mode().IsRegular() && os.SameFile(current, identity) {
		_ = os.Remove(path)
	}
}

type stagedCacheBoundedWriter struct {
	writer *bytes.Buffer
	limit  int
}

func (writer stagedCacheBoundedWriter) Write(content []byte) (int, error) {
	if len(content) > writer.limit-writer.writer.Len() {
		return 0, errors.New("cache report exceeds size bound")
	}
	return writer.writer.Write(content)
}

func stagedCacheRead(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !cachefs.Private(path, info) || info.Size() > limit {
		return nil, errors.New("unsafe cache file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("cache file changed while opening")
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if int64(len(content)) > limit {
		return nil, errors.New("cache file exceeds size bound")
	}
	return content, err
}

func stagedCacheSecret(path string) ([]byte, error) {
	content, err := stagedCacheRead(path, 32)
	if err == nil {
		if len(content) != 32 {
			return nil, errors.New("invalid cache authority")
		}
		return content, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	file, err := cachefs.Create(path)
	if errors.Is(err, os.ErrExist) {
		return stagedCacheSecret(path)
	}
	if err != nil {
		return nil, err
	}
	_, err = file.Write(secret)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// Validate each existing ancestor before creating private cache directories.
// Existing project/Git/config ancestors keep their permissions unchanged.
func stagedCachePrivateDirectory(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	var missing []string
	current := path
	for {
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			missing = append(missing, current)
		} else if err != nil {
			return err
		} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("cache directory contains a symlink or non-directory")
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := cachefs.Mkdir(missing[i]); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !cachefs.Private(path, info) {
		return errors.New("cache directory is not private")
	}
	return nil
}

var stagedCacheBinary struct {
	sync.Once
	digest string
	err    error
}

func stagedCacheExecutableDigest() (string, error) {
	stagedCacheBinary.Do(func() {
		path, err := os.Executable()
		if err != nil {
			stagedCacheBinary.err = err
			return
		}
		file, err := os.Open(path)
		if err != nil {
			stagedCacheBinary.err = err
			return
		}
		defer file.Close()
		hash := sha256.New()
		count, err := io.Copy(hash, io.LimitReader(file, 256*1024*1024+1))
		if err != nil || count > 256*1024*1024 {
			stagedCacheBinary.err = errors.New("cannot identify bounded cache executable")
			return
		}
		stagedCacheBinary.digest = hex.EncodeToString(hash.Sum(nil))
	})
	return stagedCacheBinary.digest, stagedCacheBinary.err
}
