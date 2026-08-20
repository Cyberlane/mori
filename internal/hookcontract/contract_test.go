package hookcontract

import (
	"regexp"
	"testing"
)

func TestContractIdentityIsStable(t *testing.T) {
	if Revision == "" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(Digest()) {
		t.Fatalf("invalid hook contract identity: %q %q", Revision, Digest())
	}
	const expected = "a12b16adf11655b72146d0c34966434a12f5ae7257ab10fc93f735c6c9035cfa"
	if Digest() != expected {
		t.Fatalf("hook contract changed without an explicit revision decision: %q", Digest())
	}
}
