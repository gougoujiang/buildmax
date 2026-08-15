package tool

import (
	"fmt"
	"sort"
	"strings"
)

// pageSlice returns the sub-slice of items after skipping offset elements and
// capping at limit (0 = unlimited).
func pageSlice[T any](items []T, offset, limit int) []T {
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

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
	items := pageSlice(results, offset, limit)
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
	items := pageSlice(results, offset, limit)
	if len(items) == 0 {
		return "No matches found."
	}
	var sb strings.Builder
	for i, r := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "%s: %d", r.path, len(r.matches))
	}
	return sb.String()
}

// formatContent returns ripgrep-style grouped output with optional context lines.
// offset and limit apply to match entries (not output lines).
func formatContent(results []fileResult, before, after int, showLineNumbers bool, offset, limit int) string {
	if len(results) == 0 {
		return "No matches found."
	}

	// Flatten all match entries across files to apply offset/limit.
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

	visible := pageSlice(allMatches, offset, limit)
	if len(visible) == 0 {
		return "No matches found."
	}

	// Group visible matches by file index, preserving order.
	type fileGroup struct {
		fileIdx    int
		matchLines map[int]bool
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

		matchNums := make([]int, 0, len(grp.matchLines))
		for ln := range grp.matchLines {
			matchNums = append(matchNums, ln)
		}
		sort.Ints(matchNums)

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

		merged := mergeRanges(ranges)

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
					fmt.Fprintf(&sb, "%d%s%s\n", ln, sep, lineText)
				} else {
					fmt.Fprintf(&sb, "%s%s\n", sep, lineText)
				}
			}
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
