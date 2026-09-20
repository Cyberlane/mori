package cli

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/agentskill"
	"github.com/Cyberlane/mori/internal/projectcontract"
	"github.com/Cyberlane/mori/skills"
)

func TestReviewGuidanceUpgradePreservesOwnerPolicyAndStrictGate(t *testing.T) {
	for _, policy := range []string{"# Project\n", "# Project\nAgents may use one-commit Mori receipts for fully reviewed intentional findings during authorized commits.\n"} {
		t.Run(strings.TrimSpace(policy), func(t *testing.T) {
			root := t.TempDir()
			previous, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(root); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(previous) })
			cliTestGit(t, root, "init", "--initial-branch=main")
			config := []byte(`{"profile":"review","min_tokens":1,"require_coverage":true}`)
			for name, content := range map[string][]byte{"AGENTS.md": []byte(policy), ".mori.json": config, ".moriignore": []byte("**/*_test.go\n")} {
				if err := os.WriteFile(filepath.Join(root, name), content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			run := func(want int, args ...string) string {
				t.Helper()
				var out, stderr bytes.Buffer
				if code := Run(context.Background(), args, &out, &stderr); code != want {
					t.Fatalf("%v: %d want %d: %s %s", args, code, want, &out, &stderr)
				}
				return out.String()
			}
			run(exitSuccess, "project", "upgrade", "--apply", ".")
			contractPath := filepath.Join(root, projectcontract.FileName)
			old, _, err := projectcontract.Load(contractPath)
			if err != nil {
				t.Fatal(err)
			}
			skillRoot := filepath.Join(root, ".agents", "skills", skills.ReviewSimilarityName)
			reference := filepath.Join(skillRoot, "references", "baselines-receipts.md")
			// Simulate a prior package whose exact bytes were recorded by the project.
			prior := []byte("# Staged receipts\nReceipt creation/use requires owner authorization.\n")
			if err := os.WriteFile(reference, prior, 0600); err != nil {
				t.Fatal(err)
			}
			old.EmbeddedSkill.Digest, err = installedSkillDigest(skillRoot)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := projectcontract.Marshal(old)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(contractPath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			run(exitUpgrade, "project", "upgrade", "--check", ".")
			run(exitSuccess, "project", "upgrade", "--apply", ".")
			run(exitSuccess, "project", "upgrade", "--check", ".")
			current, _, err := projectcontract.Load(contractPath)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := agentskill.PackageDigest()
			if err != nil {
				t.Fatal(err)
			}
			if current != desiredProjectContract(projectPinnedVersion(), digest) || current.EmbeddedSkill.Digest == old.EmbeddedSkill.Digest {
				t.Fatal("updated guidance is not bound to desired contract")
			}
			if current.SchemaVersion != old.SchemaVersion || current.ReviewReceiptSchema != old.ReviewReceiptSchema || current.ReportSchemaVersion != old.ReportSchemaVersion {
				t.Fatal("presentation-only upgrade changed data schemas")
			}
			embedded, err := skills.ReviewSimilarity()
			if err != nil {
				t.Fatal(err)
			}
			expected, err := fs.ReadFile(embedded, "references/baselines-receipts.md")
			if err != nil {
				t.Fatal(err)
			}
			installed, err := os.ReadFile(reference)
			if err != nil || !bytes.Equal(expected, installed) {
				t.Fatal("installed guidance differs from embedded package")
			}
			for name, expected := range map[string][]byte{"AGENTS.md": []byte(policy), ".mori.json": config, ".moriignore": []byte("**/*_test.go\n")} {
				actual, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || !bytes.Equal(actual, expected) {
					t.Fatalf("upgrade changed project policy %s", name)
				}
			}
			cliTestGit(t, root, "add", "-f", "AGENTS.md", ".mori.json", ".moriignore", ".mori-version", ".mori-project.json", ".agents/skills/mori-review-similarity")
			cliTestGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
			for name, content := range map[string]string{"source.go": "package sample\nfunc First(x int) int { return x + 1 }\n", "source_test.go": "package sample\nfunc Second(x int) int { return x + 2 }\n"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cliTestGit(t, root, "add", "source.go", "source_test.go")
			output := run(exitFindings, "review", "staged", "check", "--format", "agent", ".")
			for _, text := range []string{"focus: 2/2", "staged scope:", "next: review focused findings", "acknowledged false"} {
				if !strings.Contains(output, text) {
					t.Fatalf("missing %q: %s", text, output)
				}
			}
			// Prose authorization is for the agent; the CLI never infers acceptance.
			if _, err := os.Stat(filepath.Join(root, ".git", "mori", "staged-review.json")); !os.IsNotExist(err) {
				t.Fatalf("gate created a receipt: %v", err)
			}
		})
	}
}
