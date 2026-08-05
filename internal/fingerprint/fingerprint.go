// Package fingerprint creates stable content identities for normalized
// fragments and pairs of fragments.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
)

const (
	featureSeparator  = '\x1f'
	featureTerminator = '\x1e'
	shortHashBytes    = 8
)

// Bag returns a deterministic, truncated SHA-256 identity for a feature bag.
// Feature names are sorted before encoding so map iteration order cannot
// affect the result. The 64-bit output is sufficient for Mori's bounded scan
// sizes and keeps report identities compact.
func Bag(features model.FeatureBag) string {
	keys := make([]string, 0, len(features))
	for feature := range features {
		keys = append(keys, feature)
	}
	sort.Strings(keys)

	var encoded strings.Builder
	for _, feature := range keys {
		encoded.WriteString(feature)
		encoded.WriteRune(featureSeparator)
		encoded.WriteString(strconv.Itoa(features[feature]))
		encoded.WriteRune(featureTerminator)
	}

	digest := sha256.Sum256([]byte(encoded.String()))
	return hex.EncodeToString(digest[:shortHashBytes])
}

// Pair returns an order-independent identity for two fragment identities.
func Pair(left string, right string) string {
	if right < left {
		left, right = right, left
	}
	return left + ":" + right
}
