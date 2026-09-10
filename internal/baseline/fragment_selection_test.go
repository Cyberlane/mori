package baseline

import "testing"

func TestFragmentSelectionProfileCompatibility(t *testing.T) {
	original := ScanProfile{}
	all := original
	all.FragmentSelection = "all"
	if Digest(original) != Digest(all) {
		t.Fatal("explicit all changed existing profile digest")
	}
	production := original
	production.FragmentSelection = "production"
	tests := original
	tests.FragmentSelection = "tests"
	if Digest(production) == Digest(original) || Digest(tests) == Digest(original) || Digest(production) == Digest(tests) {
		t.Fatal("selection is absent from profile identity")
	}
	if err := validateProfile(ScanProfile{FragmentSelection: "invalid"}); err == nil {
		t.Fatal("accepted invalid selection")
	}
}
