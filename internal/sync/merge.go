package sync

import (
	"strings"
)

// Conflict describes a merge conflict region.
type Conflict struct {
	LocalLines  []string
	RemoteLines []string
}

// MergeResult holds the output of a three-way merge.
type MergeResult struct {
	Content      []byte
	Conflicts    []Conflict
	HasConflicts bool
}

// ThreeWayMerge performs a line-based three-way merge.
//
// Inputs:
//   - base: the common ancestor (last synced version)
//   - local: the current local file
//   - remote: the latest version from the registry
//
// When both sides change the same lines, conflict markers are inserted.
func ThreeWayMerge(base, local, remote []byte) *MergeResult {
	baseLines := splitLines(string(base))
	localLines := splitLines(string(local))
	remoteLines := splitLines(string(remote))

	// If base == remote, no upstream changes — keep local as-is.
	if linesEqual(baseLines, remoteLines) {
		return &MergeResult{Content: local}
	}

	// If base == local, no local changes — accept remote.
	if linesEqual(baseLines, localLines) {
		return &MergeResult{Content: remote}
	}

	// Both sides have changes — perform line-by-line merge.
	return mergeLines(baseLines, localLines, remoteLines)
}

func mergeLines(base, local, remote []string) *MergeResult {
	var result []string
	var conflicts []Conflict

	maxLen := max(len(base), len(local), len(remote))

	for i := range maxLen {
		baseLine := getLine(base, i)
		localLine := getLine(local, i)
		remoteLine := getLine(remote, i)

		switch {
		case localLine == remoteLine && baseLine == localLine:
			// No changes on this line.
			result = append(result, baseLine)
		case baseLine == localLine:
			// Only remote changed.
			result = append(result, remoteLine)
		case baseLine == remoteLine:
			// Only local changed.
			result = append(result, localLine)
		case localLine == remoteLine:
			// Both changed to the same thing.
			result = append(result, localLine)
		default:
			// Conflict: both sides changed differently.
			result = append(result,
				"<<<<<<< local",
				localLine,
				"=======",
				remoteLine,
				">>>>>>> remote",
			)
			conflicts = append(conflicts, Conflict{
				LocalLines:  []string{localLine},
				RemoteLines: []string{remoteLine},
			})
		}
	}

	merged := strings.Join(result, "\n")
	if merged != "" && !strings.HasSuffix(merged, "\n") {
		merged += "\n"
	}

	return &MergeResult{
		Content:      []byte(merged),
		Conflicts:    conflicts,
		HasConflicts: len(conflicts) > 0,
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}

	lines := strings.Split(s, "\n")

	// Remove trailing empty line from final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

func getLine(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}

	return ""
}

func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
