package sync

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// applyOverwrite replaces a local file with new content.
// If dryRun is true, records the change without writing.
func applyOverwrite(localPath string, newContent []byte, dryRun bool, result *Result) error {
	// Check if file exists and content differs.
	existing, err := os.ReadFile(filepath.Clean(localPath))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading local file %s: %w", localPath, err)
	}

	if bytes.Equal(existing, newContent) {
		result.Skipped = append(result.Skipped, localPath)

		return nil
	}

	if dryRun {
		result.Updated = append(result.Updated, localPath)

		return nil
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(localPath), 0o750); err != nil {
		return fmt.Errorf("creating directory for %s: %w", localPath, err)
	}

	if err := os.WriteFile(localPath, newContent, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", localPath, err)
	}

	result.Updated = append(result.Updated, localPath)

	return nil
}
