package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFeedbackConsentDoesNotFollowExternalConfiguration(t *testing.T) {
	opted := t.TempDir()
	other := t.TempDir()
	if got := scanFeedbackRoot(scanOptions{configPath: filepath.Join(opted, ".mori.json")}, []string{other}); got != other {
		t.Fatalf("consent root %q must follow actual scan %q", got, other)
	}
	if got := scanFeedbackRoot(scanOptions{configPath: filepath.Join(opted, ".mori.json")}, []string{opted, other}); got != "" {
		t.Fatalf("multi-project capture: %q", got)
	}
}

func TestFeedbackCaptureDoesNotChangeScanOutputOrDecision(t *testing.T) {
	base := t.TempDir()
	base, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", base)
	// macOS UserConfigDir uses HOME/Library/Application Support.
	t.Setenv("HOME", base)
	t.Setenv("AppData", base)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\nfunc Keep(x int) int { return x+1 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"scan", "--no-config", "--format", "json", "--min-tokens", "1", root}
	var offOut, offErr, onOut, onErr bytes.Buffer
	off := Run(context.Background(), args, &offOut, &offErr)
	store, err := localFeedbackStore(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Read()
	if err != nil || state.Enabled || len(state.Samples) != 0 {
		t.Fatalf("default state %+v %v", state, err)
	}
	if err := store.Enable(true); err != nil {
		t.Fatal(err)
	}
	on := Run(context.Background(), args, &onOut, &onErr)
	if off != on || offOut.String() != onOut.String() || offErr.String() != onErr.String() {
		t.Fatal("feedback changed analysis output or exit policy")
	}
	state, err = store.Read()
	if err != nil || len(state.Samples) != 1 {
		t.Fatalf("enabled capture %+v %v", state, err)
	}
}

func TestFeedbackExpandedChangedScopeDoesNotBorrowSubdirectoryConsent(t *testing.T) {
	root := t.TempDir()
	// include-focused can expand repository-wide Git changes beyond this root
	// even when there are no explicit --focus paths.
	for _, options := range []scanOptions{
		{includeFocused: true, changedSince: "HEAD"},
		{includeFocused: true, focusPaths: stringList{"../another-project/source.go"}},
		{includeFocused: true},
	} {
		if got := scanFeedbackRoot(options, []string{root}); got != "" {
			t.Fatalf("expanded scan borrowed consent from %q", got)
		}
	}
	if got := scanFeedbackRoot(scanOptions{changedSince: "HEAD"}, []string{root}); got != root {
		t.Fatal("bounded changed scan lost its consent root")
	}
}
