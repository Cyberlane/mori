package hookcontract

import (
	"regexp"
	"testing"
)

func TestContractIdentityIsStable(t *testing.T) {
	if Revision == "" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(Digest()) {
		t.Fatalf("invalid hook contract identity: %q %q", Revision, Digest())
	}
	const expected = "8767b5f76ad73534abfe0f41ec06909b994d2b2b3b059c55306b6e673c7a3855"
	if Digest() != expected {
		t.Fatalf("hook contract changed without an explicit revision decision: %q", Digest())
	}
}
