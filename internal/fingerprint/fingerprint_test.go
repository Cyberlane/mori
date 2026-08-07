package fingerprint

import (
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestBagIsDeterministicAndUsesGoldenEncoding(t *testing.T) {
	t.Parallel()

	first := Bag(model.FeatureBag{
		"node:return":   1,
		"semantic:trim": 2,
	})
	second := Bag(model.FeatureBag{
		"semantic:trim": 2,
		"node:return":   1,
	})
	if first != second {
		t.Fatalf("map iteration changed fingerprint: %q != %q", first, second)
	}
	if first != "15b9cd8711e89676" {
		t.Fatalf("fingerprint = %q, want golden value", first)
	}
}

func TestPairIsOrderIndependent(t *testing.T) {
	t.Parallel()

	if got, want := Pair("bbbbbbbbbbbbbbbb", "aaaaaaaaaaaaaaaa"), "aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbb"; got != want {
		t.Fatalf("Pair = %q, want %q", got, want)
	}
}
