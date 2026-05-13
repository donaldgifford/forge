package create

import (
	"path/filepath"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/donaldgifford/forge/internal/config"
	"github.com/donaldgifford/forge/internal/defaults"
	tmpl "github.com/donaldgifford/forge/internal/template"
)

// EvaluateConditions processes blueprint conditions and removes excluded files
// from the FileSet. Each condition has a when expression that is evaluated
// as a bool against the variables; when true, files matching the exclude
// glob patterns are removed.
func EvaluateConditions(
	conditions []config.Condition,
	ctyVars map[string]cty.Value,
	fileSet *defaults.FileSet,
	renderer tmpl.Renderer,
) error {
	for i := range conditions {
		if err := evaluateCondition(renderer, &conditions[i], ctyVars, fileSet); err != nil {
			return err
		}
	}

	return nil
}

func evaluateCondition(
	renderer tmpl.Renderer,
	cond *config.Condition,
	vars map[string]cty.Value,
	fileSet *defaults.FileSet,
) error {
	active, err := renderer.EvaluateBoolExpr(cond.When, vars)
	if err != nil {
		return err
	}

	if !active {
		return nil
	}

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
