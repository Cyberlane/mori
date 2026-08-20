// Package hookcontract identifies Mori's canonical pre-commit hook behavior.
package hookcontract

import "crypto/sha256"

// Revision changes when the canonical staged-review invocation or its fixed
// policy changes. Project hooks remain project-owned and are never installed.
const Revision = "mori-hook-pre-commit/v1"

const definition = Revision + "\x00" +
	"parse canonical staged-review options; resolve the immutable Git index; " +
	"run review staged check; optionally validate MORI_STAGED_REVIEW_RECEIPT=1 " +
	"from private Git metadata at mori/staged-review.json"

// Digest is the stable SHA-256 identity of the hook contract definition.
func Digest() string {
	sum := sha256.Sum256([]byte(definition))
	return stringHex(sum[:])
}

func stringHex(value []byte) string {
	const hex = "0123456789abcdef"
	output := make([]byte, len(value)*2)
	for index, item := range value {
		output[index*2] = hex[item>>4]
		output[index*2+1] = hex[item&15]
	}
	return string(output)
}
