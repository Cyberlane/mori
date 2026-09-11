package cli

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Cyberlane/mori/internal/parser"
)

const parseCacheSlots = 1024
const parseCacheMaxBytes = 256 * 1024

type diskParseCache struct {
	directory, executable string
	secret                []byte
}

// Cache authority and entries live outside scanned roots and the working
// directory. Cache unavailability is always a silent miss, never a policy change.
func openParseCache(paths []string) (*diskParseCache, error) {
	directory, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	directory = parser.CanonicalSelectionPath(filepath.Join(directory, "mori", "parse-cache-v1"))
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	roots := append(append([]string{}, paths...), cwd)
	for _, root := range append([]string{}, roots...) {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		absolute = parser.CanonicalSelectionPath(absolute)
		for current := absolute; ; current = filepath.Dir(current) {
			if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
				roots = append(roots, current)
				break
			}
			if filepath.Dir(current) == current {
				break
			}
		}
	}
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		absolute = parser.CanonicalSelectionPath(absolute)
		relative, err := filepath.Rel(absolute, parser.CanonicalSelectionPath(directory))
		if err != nil || relative == "." || relative != ".." && !stringsHasParentPrefix(relative) {
			return nil, errors.New("parse cache must be outside source roots")
		}
	}
	if err := stagedCachePrivateDirectory(directory); err != nil {
		return nil, err
	}
	secret, err := stagedCacheSecret(filepath.Join(directory, "key"))
	if err != nil {
		return nil, err
	}
	executable, err := stagedCacheExecutableDigest()
	if err != nil {
		return nil, err
	}
	return &diskParseCache{directory: directory, executable: executable, secret: secret}, nil
}

func (cache *diskParseCache) path(key string) string {
	digest := sha256.Sum256([]byte(key))
	slot := (int(digest[0])<<8 | int(digest[1])) % parseCacheSlots
	return filepath.Join(cache.directory, fmt.Sprintf("%04x", slot))
}

func (cache *diskParseCache) mac(key string, content []byte) []byte {
	mac := hmac.New(sha256.New, cache.secret)
	mac.Write([]byte("mori-parse-cache-v1\x00" + cache.executable + "\x00" + key + "\x00"))
	mac.Write(content)
	return mac.Sum(nil)
}

func (cache *diskParseCache) Load(ctx context.Context, key string) (parser.CacheEntry, bool) {
	if ctx.Err() != nil || !validParseCacheKey(key) {
		return parser.CacheEntry{}, false
	}
	content, err := stagedCacheRead(cache.path(key), parseCacheMaxBytes+sha256.Size)
	if err != nil || len(content) < sha256.Size || !hmac.Equal(content[:sha256.Size], cache.mac(key, content[sha256.Size:])) {
		return parser.CacheEntry{}, false
	}
	var entry parser.CacheEntry
	decoder := json.NewDecoder(bytes.NewReader(content[sha256.Size:]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil || ctx.Err() != nil {
		return parser.CacheEntry{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return parser.CacheEntry{}, false
	}
	return entry, true
}

func (cache *diskParseCache) Store(ctx context.Context, key string, entry parser.CacheEntry) {
	if ctx.Err() != nil || !validParseCacheKey(key) {
		return
	}
	var buffer bytes.Buffer
	if err := json.NewEncoder(stagedCacheBoundedWriter{writer: &buffer, limit: parseCacheMaxBytes}).Encode(entry); err != nil {
		return
	}
	path := cache.path(key)
	temporary := path + ".pending"
	file, err := stagedCacheCreatePending(temporary)
	if err != nil {
		return
	}
	identity, err := file.Stat()
	if err != nil {
		file.Close()
		return
	}
	renamed := false
	defer func() {
		if !renamed {
			stagedCacheRemoveOwnedPending(temporary, identity)
		}
	}()
	_, err = file.Write(cache.mac(key, buffer.Bytes()))
	if err == nil {
		_, err = file.Write(buffer.Bytes())
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil || ctx.Err() != nil {
		return
	}
	if err = os.Rename(temporary, path); err == nil {
		renamed = true
	}
}

// Keep the key format fixed-size so accidental non-digest callers cannot create
// unbounded authority messages. The slot path itself never contains caller data.
func validParseCacheKey(key string) bool {
	decoded, err := hex.DecodeString(key)
	return err == nil && len(decoded) == sha256.Size
}
