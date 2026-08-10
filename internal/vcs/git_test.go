package vcs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestResolveChangedIncludesCommittedAndWorkingTreeState(t *testing.T) {
	root := initRepository(t)
	writeFile(t, root, "kept.go", "package sample\n")
	writeFile(t, root, "rename.go", "package sample\n// rename source\n")
	writeFile(t, root, "delete.go", "package sample\n// delete source with distinct content\n")
	writeFile(t, root, ".gitignore", "ignored.go\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	base := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))

	writeFile(t, root, "kept.go", "package sample\n// committed\n")
	runGit(t, root, "add", "kept.go")
	runGit(t, root, "commit", "-m", "change")
	runGit(t, root, "mv", "rename.go", "renamed.go")
	if err := os.Remove(filepath.Join(root, "delete.go")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	writeFile(t, root, "staged.go", "package sample\n")
	runGit(t, root, "add", "staged.go")
	writeFile(t, root, "unstaged.go", "package sample\n")
	writeFile(t, root, "ignored.go", "package sample\n")

	files := []string{
		filepath.Join(root, "kept.go"),
		filepath.Join(root, "renamed.go"),
		filepath.Join(root, "staged.go"),
		filepath.Join(root, "unstaged.go"),
	}
	changes, err := ResolveChanged(context.Background(), files, base)
	if err != nil {
		t.Fatalf("ResolveChanged: %v", err)
	}
	wantChanged := []string{"kept.go", "renamed.go", "staged.go", "unstaged.go"}
	if !reflect.DeepEqual(changes.ChangedPaths, wantChanged) {
		t.Fatalf("changed = %#v, want %#v", changes.ChangedPaths, wantChanged)
	}
	if !reflect.DeepEqual(changes.DeletedPaths, []string{"delete.go"}) {
		t.Fatalf("deleted = %#v", changes.DeletedPaths)
	}
	if changes.BaseCommit != base || changes.MergeBase != base || !isFullSHA(changes.HeadCommit) {
		t.Fatalf("commits = %#v", changes)
	}
}

func TestResolveChangedRejectsInvalidRevisionAndMissingGit(t *testing.T) {
	root := initRepository(t)
	writeFile(t, root, "main.go", "package sample\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	file := filepath.Join(root, "main.go")

	if _, err := ResolveChanged(context.Background(), []string{file}, "missing"); err == nil ||
		!strings.Contains(err.Error(), "resolve --changed-since") {
		t.Fatalf("invalid revision error = %v", err)
	}
	_, err := resolveChanged(context.Background(), []string{file}, "HEAD", resolverOptions{
		executable: filepath.Join(root, "missing-git"), timeout: time.Second,
		maxOutput: 1024, maxPaths: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "resolve Git worktree") {
		t.Fatalf("missing Git error = %v", err)
	}
}

func TestResolveChangedRejectsNestedRepository(t *testing.T) {
	root := initRepository(t)
	writeFile(t, root, "main.go", "package sample\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	runGit(t, nested, "init", "--initial-branch=main")
	writeFile(t, nested, "nested.go", "package nested\n")

	_, err := ResolveChanged(context.Background(), []string{
		filepath.Join(root, "main.go"), filepath.Join(nested, "nested.go"),
	}, "HEAD")
	if err == nil || !strings.Contains(err.Error(), "multiple Git worktrees at nested") ||
		!strings.Contains(err.Error(), "scan that worktree separately") {
		t.Fatalf("nested repository error = %v", err)
	}
}

func TestResolveChangedAtRootRequiresExactWorktreeAndKeepsRootLocalPaths(t *testing.T) {
	root := initRepository(t)
	writeFile(t, root, "main.go", "package sample\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	base := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	writeFile(t, root, "main.go", "package sample\n// changed\n")
	writeFile(t, root, "new.go", "package sample\n")

	changes, err := ResolveChangedAtRoot(context.Background(), root, base)
	if err != nil {
		t.Fatalf("ResolveChangedAtRoot: %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if changes.Root != canonicalRoot || !reflect.DeepEqual(changes.ChangedPaths, []string{"main.go", "new.go"}) {
		t.Fatalf("changes = %#v", changes)
	}
	if _, err := ResolveChangedAtRoot(context.Background(), filepath.Join(root, "subdir"), base); err == nil ||
		!strings.Contains(err.Error(), "canonicalize explicit Git worktree") {
		t.Fatalf("missing explicit root error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := ResolveChangedAtRoot(context.Background(), filepath.Join(root, "subdir"), base); err == nil ||
		!strings.Contains(err.Error(), "resolves to parent worktree") {
		t.Fatalf("parent worktree error = %v", err)
	}
}

func TestResolveChangedHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := resolveChanged(ctx, nil, "HEAD", resolverOptions{
		executable: "git", timeout: time.Second, maxOutput: 1024, maxPaths: 10,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestParseNameStatusRejectsUnsafeAndUnboundedPaths(t *testing.T) {
	if _, _, err := parseNameStatus([]byte("M\x00../outside.go\x00"), 10); err == nil {
		t.Fatal("unsafe path returned nil")
	}
	if _, _, err := parseNameStatus([]byte("M\x00a.go\x00M\x00b.go\x00"), 1); err == nil {
		t.Fatal("path limit returned nil")
	}
}

func TestResolveIndexReadsOnlyStageZeroSnapshot(t *testing.T) {
	root := initRepository(t)
	writeFile(t, root, "sample.go", "package sample\nfunc First() int { return 1 }\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	writeFile(t, root, "sample.go", "package sample\nfunc Staged() int { return 2 }\n")
	runGit(t, root, "add", "sample.go")
	writeFile(t, root, "sample.go", "package sample\nfunc Working() int { return 3 }\n")

	snapshot, err := ResolveIndex(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveIndex: %v", err)
	}
	if !reflect.DeepEqual(snapshot.ChangedPaths, []string{"sample.go"}) || snapshot.IndexDigest == "" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	entry, ok := IndexEntryForPath(snapshot, "sample.go")
	if !ok {
		t.Fatal("sample.go missing from index")
	}
	content, size, err := ReadIndexBlob(context.Background(), snapshot, entry, 1024)
	if err != nil {
		t.Fatalf("ReadIndexBlob: %v", err)
	}
	if !strings.Contains(string(content), "Staged") || strings.Contains(string(content), "Working") || int64(len(content)) != size {
		t.Fatalf("content = %q, size = %d", content, size)
	}
}

func TestResolveIndexSupportsUnbornRepository(t *testing.T) {
	root := initRepository(t)
	writeFile(t, root, "first.go", "package sample\n")
	runGit(t, root, "add", "first.go")
	snapshot, err := ResolveIndex(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveIndex: %v", err)
	}
	if snapshot.HeadCommit != "" || !reflect.DeepEqual(snapshot.ChangedPaths, []string{"first.go"}) {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestParseIndexEntriesRejectsUnsafeAndUnmergedEntries(t *testing.T) {
	oid := strings.Repeat("a", 40)
	if _, err := parseIndexEntries([]byte("100644 "+oid+" 0\t../outside.go\x00"), 10); err == nil {
		t.Fatal("unsafe index path returned nil")
	}
	if _, err := parseIndexEntries([]byte("100644 "+oid+" 2\tconflict.go\x00"), 10); err == nil || !strings.Contains(err.Error(), "unmerged") {
		t.Fatalf("unmerged index error = %v", err)
	}
}

func TestRelativeIndexPathAcceptsDescendantsAndRejectsOutside(t *testing.T) {
	root := t.TempDir()
	snapshot := IndexSnapshot{Root: root}
	path := filepath.Join(root, "nested", "config.json")
	relative, err := RelativeIndexPath(snapshot, path)
	if err != nil || relative != "nested/config.json" {
		t.Fatalf("RelativeIndexPath(descendant) = %q, %v", relative, err)
	}
	if _, err := RelativeIndexPath(snapshot, filepath.Join(filepath.Dir(root), "outside.json")); err == nil ||
		!strings.Contains(err.Error(), "outside the Git worktree") {
		t.Fatalf("RelativeIndexPath(outside) error = %v", err)
	}
}

func TestRelativeIndexPathCanonicalizesExistingPath(t *testing.T) {
	root := t.TempDir()
	linked := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, linked); err != nil {
		t.Skipf("Symlink unavailable: %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root): %v", err)
	}
	snapshot := IndexSnapshot{Root: canonicalRoot}
	relative, err := RelativeIndexPath(snapshot, linked)
	if err != nil || relative != "." {
		t.Fatalf("RelativeIndexPath(link) = %q, %v", relative, err)
	}
}

func initRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Mori Test")
	runGit(t, root, "config", "user.email", "mori@example.invalid")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func writeFile(t *testing.T, root string, name string, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
