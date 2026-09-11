package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotRootsAcceptWorktreeAliasesWithoutFollowingSourceLinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "worktree")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "src")); err != nil {
		t.Fatal(err)
	}
	content := []byte("package staged\nfunc Saved() int { return 7 }\n")
	entries := []SnapshotEntry{{Path: "src/index-only.go", Mode: "100644", Size: int64(len(content)), Content: content}}
	for _, requested := range [][]string{{alias}, {filepath.Join(alias, "src")}, {filepath.Join(alias, "src", "index-only.go")}, {filepath.Join(root, "src")}} {
		result, err := DiscoverSnapshotAt(context.Background(), root, root, requested, entries, Options{})
		if err != nil || len(result.Files) != 1 {
			t.Fatalf("%v: files=%v err=%v", requested, result.Files, err)
		}
		if result.Files[0].ClassificationPath != "src/index-only.go" {
			t.Fatalf("classification path=%q", result.Files[0].ClassificationPath)
		}
		if string(result.Files[0].Content) != string(content) {
			t.Fatal("snapshot content changed")
		}
	}
	if _, err := DiscoverSnapshotAt(context.Background(), root, root, []string{outside}, entries, Options{}); err == nil {
		t.Fatal("accepted root outside worktree")
	}
}
