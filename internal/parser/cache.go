package parser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/source"
)

// Cache is an optional local optimization. A miss must preserve normal parsing.
// Implementations authenticate entries and bound storage and decoding.
type Cache interface {
	Load(context.Context, string) (CacheEntry, bool)
	Store(context.Context, string, CacheEntry)
}

// CachedFragment includes internal scoring data omitted by public report JSON.
type CachedFragment struct {
	Fragment       model.Fragment   `json:"fragment"`
	Features       model.FeatureBag `json:"features"`
	StartByte      uint             `json:"start_byte"`
	EndByte        uint             `json:"end_byte"`
	LiteralDigests [][]byte         `json:"literal_digests"`
}

type CacheEntry struct {
	Fragments []CachedFragment `json:"fragments"`
	Warnings  []model.Warning  `json:"warnings"`
	Coverage  Coverage         `json:"coverage"`
}

func FileWithCoverage(ctx context.Context, file source.File, options Options) ([]model.Fragment, []model.Warning, Coverage) {
	if options.Cache == nil {
		return fileWithCoverageUncached(ctx, file, options)
	}
	file, options = prepareSelectionPaths(file, options)
	content, err := readSource(ctx, file)
	if err != nil || ctx.Err() != nil {
		return fileWithCoverageUncached(ctx, file, options)
	}
	// Always read current bytes before looking up a result. Content is also reused
	// on a miss, so the key and parse cannot refer to different file revisions.
	file.Content = content
	key := parseCacheKey(file, options, content)
	if key == "" {
		return fileWithCoverageUncached(ctx, file, options)
	}
	if cached, ok := options.Cache.Load(ctx, key); ok && ctx.Err() == nil {
		var fragments []model.Fragment
		if cached.Fragments != nil {
			fragments = make([]model.Fragment, len(cached.Fragments))
		}
		for i, fragment := range cached.Fragments {
			fragments[i] = fragment.Fragment
			fragments[i].Features = fragment.Features
			fragments[i].StartByte = fragment.StartByte
			fragments[i].EndByte = fragment.EndByte
			if fragment.LiteralDigests != nil {
				fragments[i].LiteralDigests = make([]string, len(fragment.LiteralDigests))
				for j, digest := range fragment.LiteralDigests {
					fragments[i].LiteralDigests[j] = string(digest)
				}
			}
		}
		return fragments, cached.Warnings, cached.Coverage
	}
	fragments, warnings, coverage := fileWithCoverageUncached(ctx, file, options)
	if ctx.Err() == nil {
		entry := CacheEntry{Warnings: warnings, Coverage: coverage}
		if fragments != nil {
			entry.Fragments = make([]CachedFragment, len(fragments))
		}
		for i, fragment := range fragments {
			entry.Fragments[i] = CachedFragment{Fragment: fragment, Features: fragment.Features, StartByte: fragment.StartByte, EndByte: fragment.EndByte, LiteralDigests: cacheLiteralDigests(fragment.LiteralDigests)}
		}
		options.Cache.Store(ctx, key, entry)
	}
	return fragments, warnings, coverage
}

func parseCacheKey(file source.File, options Options, content []byte) string {
	options.Cache = nil

	// Options is serialized whole so future extraction settings invalidate by
	// default. The disk implementation additionally binds the executable digest.
	encoded, err := json.Marshal(struct {
		Revision                                                int
		Path, DisplayPath, ClassificationPath, Language, Domain string
		Options                                                 Options
		Digest                                                  [32]byte
	}{1, file.Path, file.DisplayPath, file.ClassificationPath, file.Language.ID, file.AnalysisDomain, options, sha256.Sum256(content)})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func cacheLiteralDigests(digests []string) [][]byte {
	if digests == nil {
		return nil
	}
	encoded := make([][]byte, len(digests))
	for i, digest := range digests {
		encoded[i] = []byte(digest)
	}
	return encoded
}

func prepareSelectionPaths(file source.File, options Options) (source.File, Options) {
	if options.selectionPathsResolved {
		return file, options
	}
	options.selectionPathsResolved = true
	if len(options.ProductionPaths)+len(options.TestPaths) > 0 {
		file.Path = CanonicalSelectionPath(file.Path)
		options.ProductionPaths = append([]string(nil), options.ProductionPaths...)
		options.TestPaths = append([]string(nil), options.TestPaths...)
		for i, path := range options.ProductionPaths {
			options.ProductionPaths[i] = CanonicalSelectionPath(path)
		}
		for i, path := range options.TestPaths {
			options.TestPaths[i] = CanonicalSelectionPath(path)
		}
	}
	return file, options
}
