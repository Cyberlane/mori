package cli

import (
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestRedactReportPathsUsesStableExtensionPreservingPlaceholders(t *testing.T) {
	t.Parallel()
	report := model.Report{
		Groups: []model.MatchGroup{{
			Profiles: []model.FragmentProfile{{Occurrences: []model.FragmentSummary{{
				Location: model.Location{Path: "/private/project/z.go"},
				Parent:   &model.Location{Path: "/private/project/a.ts"},
			}}}},
			PathPairs: []model.LocationPair{{
				Left: model.Location{Path: "/private/project/z.go"}, Right: model.Location{Path: "/private/project/a.ts"},
			}},
		}},
		Warnings:     []model.Warning{{Path: "/private/project/z.go", Message: "parse error"}},
		FileCoverage: []model.FileCoverage{{Path: "/private/project/a.ts"}},
		Configuration: model.EffectiveConfig{
			ConfigPath:  "/private/project/.mori.json",
			IgnoreFiles: []string{"/private/project/.gitignore"},
			IgnoreEvidence: []model.IgnoreFileEvidence{{
				Path: "/private/project/.gitignore", Digest: "digest",
			}},
			Focus: &model.FocusConfig{
				ExplicitPaths: []string{"/private/project/z.go"},
				Worktrees: []model.WorktreeFocusConfig{{
					Root: "/private/project", ChangedPaths: []string{"/private/project/a.ts"},
				}},
			},
		},
	}

	redactReportPaths(&report)
	if got := report.Groups[0].Profiles[0].Occurrences[0].Location.Path; got != "<path-005>.go" {
		t.Fatalf("source path = %q", got)
	}
	if got := report.Groups[0].Profiles[0].Occurrences[0].Parent.Path; got != "<path-004>.ts" {
		t.Fatalf("parent path = %q", got)
	}
	if report.Warnings[0].Path != "<path-005>.go" || report.Groups[0].PathPairs[0].Left.Path != "<path-005>.go" {
		t.Fatal("same path did not receive the same placeholder")
	}
	if got := report.Configuration.IgnoreEvidence[0].Digest; got != "digest" {
		t.Fatalf("redaction changed non-path evidence: %q", got)
	}
	if strings.Contains(report.Configuration.Focus.Worktrees[0].Root, "private") {
		t.Fatalf("worktree root was not redacted: %q", report.Configuration.Focus.Worktrees[0].Root)
	}
}
