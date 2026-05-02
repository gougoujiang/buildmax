// Package tools provides concrete agent tools (e.g. read_file, glob, grep).
//
// grep_format.go — output formatting for the Grep tool.
// Three modes: content (ripgrep-style with context), files_with_matches, count.
package agenttool

import (
	"fmt"
	"sort"
	"strings"
)

// lineRange represents a range of 1-based line numbers [start, end] (inclusive).
type lineRange struct {
	start, end int
}

// mergeRanges merges overlapping or adjacent ranges.
func mergeRanges(ranges []lineRange) []lineRange {
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
	merged := []lineRange{ranges[0]}
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.start <= last.end+1 {
			// Overlapping or adjacent: extend
			if r.end > last.end {
				last.end = r.end
			}
		} else {
			merged = append(merged, r)
		}
	}
	return merged
}

// formatFilesWithMatches returns one absolute path per line for files with matches.
func formatFilesWithMatches(results []fileResult, offset, limit int) string {
	if len(results) == 0 {
		return "No matches found."
	}

	// Apply offset
	start := offset
	if start > len(results) {
		start = len(results)
	}
	items := results[start:]

	// Apply limit
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}

	if len(items) == 0 {
		return "No matches found."
	}

	var sb strings.Builder
	for i, r := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(r.path)
	}
	return sb.String()
}

// formatCount returns "filepath: N" per line for files with matches.
func formatCount(results []fileResult, offset, limit int) string {
	if len(results) == 0 {
		return "No matches found."
	}

	// Apply offset
	start := offset
	if start > len(results) {
		start = len(results)
	}
	items := results[start:]

	// Apply limit
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}

	if len(items) == 0 {
		return "No matches found."
	}

	var sb strings.Builder
	for i, r := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(fmt.Sprintf("%s: %d", r.path, len(r.matches)))
	}
	return sb.String()
}

// formatContent returns ripgrep-style grouped output with optional context lines.
// offset and limit apply to match entries (not output lines).
func formatContent(results []fileResult, before, after int, showLineNumbers bool, offset, limit int) string {
	if len(results) == 0 {
		return "No matches found."
	}

	// First, flatten all match entries across files to apply offset/limit.
	type indexedMatch struct {
		fileIdx  int
		matchIdx int
	}
	var allMatches []indexedMatch
	for fi, r := range results {
		for mi := range r.matches {
			allMatches = append(allMatches, indexedMatch{fileIdx: fi, matchIdx: mi})
		}
	}

	// Apply offset
	start := offset
	if start > len(allMatches) {
		start = len(allMatches)
	}
	visible := allMatches[start:]

	// Apply limit
	if limit > 0 && limit < len(visible) {
		visible = visible[:limit]
	}

	if len(visible) == 0 {
		return "No matches found."
	}

	// Group visible matches by file index, preserving order
	type fileGroup struct {
		fileIdx    int
		matchLines map[int]bool // set of 1-based line numbers that are actual matches
	}
	var groups []fileGroup
	groupMap := make(map[int]int) // fileIdx → index in groups

	for _, vm := range visible {
		idx, ok := groupMap[vm.fileIdx]
		if !ok {
			idx = len(groups)
			groupMap[vm.fileIdx] = idx
			groups = append(groups, fileGroup{
				fileIdx:    vm.fileIdx,
				matchLines: make(map[int]bool),
			})
		}
		lineNum := results[vm.fileIdx].matches[vm.matchIdx].lineNum
		groups[idx].matchLines[lineNum] = true
	}

	var sb strings.Builder
	for gi, grp := range groups {
		if gi > 0 {
			sb.WriteByte('\n')
		}
		fr := results[grp.fileIdx]
		totalLines := len(fr.lines)

		// Compute display ranges from the visible match lines + context
		matchNums := make([]int, 0, len(grp.matchLines))
		for ln := range grp.matchLines {
			matchNums = append(matchNums, ln)
		}
		sort.Ints(matchNums)

		// Build ranges [start, end] (1-based, inclusive)
		var ranges []lineRange
		for _, m := range matchNums {
			s := m - before
			if s < 1 {
				s = 1
			}
			e := m + after
			if e > totalLines {
				e = totalLines
			}
			ranges = append(ranges, lineRange{s, e})
		}

		// Merge overlapping or adjacent ranges
		merged := mergeRanges(ranges)

		// Emit file header
		sb.WriteString(fr.path)
		sb.WriteByte('\n')

		for ri, rng := range merged {
			if ri > 0 {
				sb.WriteString("--\n")
			}
			for ln := rng.start; ln <= rng.end; ln++ {
				lineText := ""
				if ln >= 1 && ln <= totalLines {
					lineText = fr.lines[ln-1]
				}
				isMatch := grp.matchLines[ln]
				sep := "-"
				if isMatch {
					sep = ":"
				}
				if showLineNumbers {
					sb.WriteString(fmt.Sprintf("%d%s%s\n", ln, sep, lineText))
				} else {
					sb.WriteString(fmt.Sprintf("%s%s\n", sep, lineText))
				}
			}
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
