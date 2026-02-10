package create

import (
	"path/filepath"
	"strings"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/defaults"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

// EvaluateConditions processes blueprint conditions and removes excluded files
// from the FileSet. Each condition has a when template expression that is
// rendered against the variables. If the result is "true", files matching
// the exclude glob patterns are removed.
func EvaluateConditions(conditions []config.Condition, vars map[string]any, fileSet *defaults.FileSet) error {
	if len(conditions) == 0 {
		return nil
	}

	renderer := tmpl.NewRenderer()

	for i := range conditions {
		if err := evaluateCondition(renderer, &conditions[i], vars, fileSet); err != nil {
			return err
		}
	}

	return nil
}

func evaluateCondition(
	renderer *tmpl.Renderer,
	cond *config.Condition,
	vars map[string]any,
	fileSet *defaults.FileSet,
) error {
	result, err := renderer.RenderString(cond.When, vars)
	if err != nil {
		return err
	}

	// Condition is active when the rendered result is "true".
	if strings.TrimSpace(result) != "true" {
		return nil
	}

	// Remove files matching the exclude patterns.
	for _, entry := range fileSet.Entries() {
		if matchesAnyPattern(entry.RelPath, cond.Exclude) {
			fileSet.Remove(entry.RelPath)
		}
	}

	return nil
}

// matchesAnyPattern checks if a relative path matches any of the given glob patterns.
// Patterns can match directories (e.g., "proto/*") or specific files.
func matchesAnyPattern(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, relPath)
		if err != nil {
			continue // Skip invalid glob patterns.
		}

		if matched {
			return true
		}

		// Also check if the pattern matches a parent directory prefix.
		// This allows patterns like "proto/" to match "proto/service.proto".
		dir := strings.TrimSuffix(pattern, "*")
		dir = strings.TrimSuffix(dir, "/")

		if dir != "" && strings.HasPrefix(relPath, dir+"/") {
			return true
		}
	}

	return false
}
