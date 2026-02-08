package sync

import (
	"fmt"
	"io"
	"strings"
)

// ConflictFile pairs a file path with its merge conflicts.
type ConflictFile struct {
	Path      string
	Conflicts []Conflict
}

// ConflictError is returned when sync completes with unresolved conflicts.
type ConflictError struct {
	Files []ConflictFile
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%d file(s) have merge conflicts", len(e.Files))
}

// ReportConflicts writes a summary of conflicted files to w.
// Returns a ConflictError if any conflicts exist.
func ReportConflicts(w io.Writer, files []ConflictFile) error {
	if len(files) == 0 {
		return nil
	}

	if _, err := fmt.Fprintf(w, "\nConflicts detected in %d file(s):\n", len(files)); err != nil {
		return fmt.Errorf("writing conflict report: %w", err)
	}

	for _, f := range files {
		if _, err := fmt.Fprintf(w, "  CONFLICT %s (%d conflict region(s))\n", f.Path, len(f.Conflicts)); err != nil {
			return fmt.Errorf("writing conflict report: %w", err)
		}
	}

	if _, err := fmt.Fprintln(w, "\nResolve conflicts manually, then run 'forge sync' again."); err != nil {
		return fmt.Errorf("writing conflict report: %w", err)
	}

	return &ConflictError{Files: files}
}

// StripConflictMarkers resolves all conflict regions in content by keeping
// the specified side ("local" or "remote").
func StripConflictMarkers(content, keepSide string) string {
	var result []string

	lines := strings.Split(content, "\n")
	inConflict := false
	inLocal := false
	inRemote := false

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "<<<<<<< "):
			inConflict = true
			inLocal = true
			inRemote = false
		case inConflict && line == "=======":
			inLocal = false
			inRemote = true
		case strings.HasPrefix(line, ">>>>>>> "):
			inConflict = false
			inLocal = false
			inRemote = false
		case inConflict && inLocal && keepSide == "local":
			result = append(result, line)
		case inConflict && inRemote && keepSide == "remote":
			result = append(result, line)
		case !inConflict:
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
