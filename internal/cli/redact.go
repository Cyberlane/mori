package cli

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Cyberlane/mori/internal/model"
)

func redactReportPaths(report *model.Report) {
	paths := make(map[string]struct{})
	add := func(path string) {
		if path != "" {
			paths[path] = struct{}{}
		}
	}
	for _, group := range report.Groups {
		for _, profile := range group.Profiles {
			for _, occurrence := range profile.Occurrences {
				add(occurrence.Location.Path)
				if occurrence.Parent != nil {
					add(occurrence.Parent.Path)
				}
			}
		}
		for _, pair := range group.PathPairs {
			add(pair.Left.Path)
			add(pair.Right.Path)
		}
	}
	for _, warning := range report.Warnings {
		add(warning.Path)
	}
	for _, coverage := range report.FileCoverage {
		add(coverage.Path)
	}
	add(report.Configuration.ConfigPath)
	add(report.Configuration.BaselinePath)
	add(report.Configuration.StdinPath)
	for _, path := range report.Configuration.IgnoreFiles {
		add(path)
	}
	for _, evidence := range report.Configuration.IgnoreEvidence {
		add(evidence.Path)
	}
	if focus := report.Configuration.Focus; focus != nil {
		addPaths(add, focus.ExplicitPaths)
		addPaths(add, focus.ChangedPaths)
		addPaths(add, focus.DeletedPaths)
		for _, worktree := range focus.Worktrees {
			add(worktree.Root)
			addPaths(add, worktree.ChangedPaths)
			addPaths(add, worktree.DeletedPaths)
		}
	}

	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	replacements := make(map[string]string, len(ordered))
	for index, path := range ordered {
		extension := filepath.Ext(path)
		if len(extension) > 16 {
			extension = ""
		}
		replacements[path] = fmt.Sprintf("<path-%03d>%s", index+1, extension)
	}
	replace := func(path string) string {
		if replacement, ok := replacements[path]; ok {
			return replacement
		}
		return path
	}

	for groupIndex := range report.Groups {
		group := &report.Groups[groupIndex]
		for profileIndex := range group.Profiles {
			profile := &group.Profiles[profileIndex]
			for occurrenceIndex := range profile.Occurrences {
				occurrence := &profile.Occurrences[occurrenceIndex]
				occurrence.Location.Path = replace(occurrence.Location.Path)
				if occurrence.Parent != nil {
					occurrence.Parent.Path = replace(occurrence.Parent.Path)
				}
			}
		}
		for pairIndex := range group.PathPairs {
			group.PathPairs[pairIndex].Left.Path = replace(group.PathPairs[pairIndex].Left.Path)
			group.PathPairs[pairIndex].Right.Path = replace(group.PathPairs[pairIndex].Right.Path)
		}
	}
	for index := range report.Warnings {
		report.Warnings[index].Path = replace(report.Warnings[index].Path)
	}
	for index := range report.FileCoverage {
		report.FileCoverage[index].Path = replace(report.FileCoverage[index].Path)
	}
	report.Configuration.ConfigPath = replace(report.Configuration.ConfigPath)
	report.Configuration.BaselinePath = replace(report.Configuration.BaselinePath)
	report.Configuration.StdinPath = replace(report.Configuration.StdinPath)
	replacePaths(replace, report.Configuration.IgnoreFiles)
	for index := range report.Configuration.IgnoreEvidence {
		report.Configuration.IgnoreEvidence[index].Path = replace(report.Configuration.IgnoreEvidence[index].Path)
	}
	if focus := report.Configuration.Focus; focus != nil {
		replacePaths(replace, focus.ExplicitPaths)
		replacePaths(replace, focus.ChangedPaths)
		replacePaths(replace, focus.DeletedPaths)
		for index := range focus.Worktrees {
			worktree := &focus.Worktrees[index]
			worktree.Root = replace(worktree.Root)
			replacePaths(replace, worktree.ChangedPaths)
			replacePaths(replace, worktree.DeletedPaths)
		}
	}
}

func addPaths(add func(string), paths []string) {
	for _, path := range paths {
		add(path)
	}
}

func replacePaths(replace func(string) string, paths []string) {
	for index := range paths {
		paths[index] = replace(paths[index])
	}
}
