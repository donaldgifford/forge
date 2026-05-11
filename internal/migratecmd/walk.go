package migratecmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	blueprintFileName = "blueprint.yaml"
	registryFileName  = "registry.yaml"
	tmplExtension     = ".tmpl"

	apiVersionV2 = "v2"
)

// runMigrate is the package-internal driver invoked by RunMigrate. It is
// split out so tests can call it directly without the dirty-worktree
// guard plumbing in RunMigrate proper.
func runMigrate(opts *MigrateOpts) (*MigrateResult, error) {
	root := opts.Path
	if root == "" {
		root = "."
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving path %s: %w", root, err)
	}

	result := &MigrateResult{}

	// 1. Bump registry.yaml apiVersion if present (per OQ-5).
	registryPath := filepath.Join(abs, registryFileName)
	if _, statErr := os.Stat(registryPath); statErr == nil {
		if err := bumpRegistryAPIVersion(registryPath, opts.DryRun); err != nil {
			return nil, fmt.Errorf("bumping registry.yaml: %w", err)
		}
	}

	// 2. Discover and migrate every blueprint under root.
	blueprints, err := walkBlueprints(abs)
	if err != nil {
		return nil, fmt.Errorf("walking blueprints: %w", err)
	}

	covered := make(map[string]struct{})

	for _, bpDir := range blueprints {
		report, err := migrateBlueprint(bpDir, opts.DryRun)
		if err != nil {
			return nil, fmt.Errorf("migrating %s: %w", bpDir, err)
		}

		for _, f := range report.FilesRewritten {
			covered[f] = struct{}{}
		}

		result.Blueprints = append(result.Blueprints, *report)
	}

	// 3. Migrate any .tmpl files under _defaults/ (registry-wide and
	// category-level) that the per-blueprint walk didn't cover.
	defaultHits, defaultFiles, err := migrateDefaults(abs, covered, opts.DryRun)
	if err != nil {
		return nil, fmt.Errorf("migrating _defaults: %w", err)
	}

	if len(defaultFiles) > 0 {
		result.Blueprints = append(result.Blueprints, BlueprintReport{
			Path:             filepath.Join(abs, "_defaults"),
			Migrated:         !opts.DryRun,
			FilesRewritten:   defaultFiles,
			UntranslatedHits: defaultHits,
		})
	}

	return result, nil
}

// migrateDefaults walks every _defaults/ directory under root and
// rewrites .tmpl files the per-blueprint pass didn't already cover. The
// covered set is used to dedupe — a _defaults dir nested *inside* a
// blueprint already gets migrated by migrateBlueprint.
func migrateDefaults(root string, covered map[string]struct{}, dryRun bool) ([]UntranslatedHit, []string, error) {
	var (
		hits  []UntranslatedHit
		files []string
	)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(d.Name(), tmplExtension) {
			return nil
		}

		if _, done := covered[path]; done {
			return nil
		}

		// gosec: this is a developer-driven migration tool; the path comes
		// from filepath.WalkDir under a user-supplied root, and we must
		// read every .tmpl file we discover to rewrite it in place.
		src, readErr := os.ReadFile(filepath.Clean(path)) //nolint:gosec // G122: walker rewrites files in user's registry
		if readErr != nil {
			return readErr
		}

		out, fileHits, rewriteErr := RewriteTemplate(path, string(src))
		if rewriteErr != nil {
			return fmt.Errorf("rewriting %s: %w", path, rewriteErr)
		}

		hits = append(hits, fileHits...)

		if string(src) == out {
			return nil
		}

		files = append(files, path)

		if dryRun {
			return nil
		}

		return os.WriteFile(path, []byte(out), 0o644) //nolint:gosec // G122: walker rewrites files in user's registry
	})

	return hits, files, err
}

// walkBlueprints returns every directory under root that contains a
// blueprint.yaml file.
func walkBlueprints(root string) ([]string, error) {
	var out []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || d.Name() != blueprintFileName {
			return nil
		}

		out = append(out, filepath.Dir(path))

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// matchPathIdentifier returns the identifier embedded in a v1 path segment
// like `{{name}}` or `{{ .name }}`, or empty string if the segment is not a
// pure-template name. Requires that the whole segment match the pattern.
func matchPathIdentifier(name string) string {
	if match := pathShorthandPattern.FindStringSubmatch(name); len(match) == 2 && match[0] == name {
		return match[1]
	}

	if match := pathDottedPattern.FindStringSubmatch(name); len(match) == 2 && match[0] == name {
		return match[1]
	}

	return ""
}

// renamePathShorthandDirs walks root and renames any directory whose
// name matches the v1 `{{name}}` shorthand or `{{ .name }}` dotted form
// to the v2 `${name}` form. Walks bottom-up so deeper renames don't
// break shallower ones.
func renamePathShorthandDirs(root string, dryRun bool) ([]string, error) {
	type rename struct{ from, to string }

	var renames []rename

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		name := d.Name()

		ident := matchPathIdentifier(name)
		if ident == "" {
			return nil
		}

		newName := "${" + ident + "}"
		renames = append(renames, rename{path, filepath.Join(filepath.Dir(path), newName)})

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Walk bottom-up.
	out := make([]string, 0, len(renames))

	for i := len(renames) - 1; i >= 0; i-- {
		r := renames[i]
		out = append(out, r.from)

		if dryRun {
			continue
		}

		if err := os.Rename(r.from, r.to); err != nil {
			return nil, fmt.Errorf("renaming %s → %s: %w", r.from, r.to, err)
		}
	}

	return out, nil
}

// migrateBlueprint runs the migration for one blueprint directory:
//  1. Read blueprint.yaml; if apiVersion is already v2, return AlreadyV2.
//  2. Rewrite expression fields (variable.default, condition.when, rename).
//  3. Rewrite all .tmpl files under the directory.
//  4. Bump apiVersion v1 → v2 (last so a mid-run failure doesn't claim
//     v2 on a partially converted blueprint).
func migrateBlueprint(dir string, dryRun bool) (*BlueprintReport, error) {
	report := &BlueprintReport{Path: dir}
	bpPath := filepath.Join(dir, blueprintFileName)

	data, err := os.ReadFile(filepath.Clean(bpPath))
	if err != nil {
		return nil, fmt.Errorf("reading blueprint.yaml: %w", err)
	}

	current := readAPIVersion(data)
	if current == apiVersionV2 {
		report.AlreadyV2 = true

		return report, nil
	}

	rewritten, hits, err := rewriteBlueprintYAML(bpPath, data)
	if err != nil {
		return nil, fmt.Errorf("rewriting blueprint.yaml: %w", err)
	}

	report.UntranslatedHits = append(report.UntranslatedHits, hits...)

	tmplFiles, err := findTemplateFiles(dir)
	if err != nil {
		return nil, err
	}

	tmplOutputs := make(map[string][]byte, len(tmplFiles))

	for _, tmplPath := range tmplFiles {
		src, readErr := os.ReadFile(filepath.Clean(tmplPath))
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", tmplPath, readErr)
		}

		out, fileHits, rewriteErr := RewriteTemplate(tmplPath, string(src))
		if rewriteErr != nil {
			return nil, fmt.Errorf("rewriting %s: %w", tmplPath, rewriteErr)
		}

		tmplOutputs[tmplPath] = []byte(out)
		report.UntranslatedHits = append(report.UntranslatedHits, fileHits...)

		if string(src) != out {
			report.FilesRewritten = append(report.FilesRewritten, tmplPath)
		}
	}

	if dryRun {
		return report, nil
	}

	for path, content := range tmplOutputs {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", path, err)
		}
	}

	if err := os.WriteFile(bpPath, rewritten, 0o644); err != nil {
		return nil, fmt.Errorf("writing blueprint.yaml: %w", err)
	}

	renamed, err := renamePathShorthandDirs(dir, dryRun)
	if err != nil {
		return nil, fmt.Errorf("renaming dirs: %w", err)
	}

	report.FilesRewritten = append(report.FilesRewritten, renamed...)

	report.Migrated = true
	report.FilesRewritten = append(report.FilesRewritten, bpPath)

	return report, nil
}

// findTemplateFiles returns every .tmpl file under root.
func findTemplateFiles(root string) ([]string, error) {
	var out []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), tmplExtension) {
			out = append(out, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// readAPIVersion returns the apiVersion value from a YAML document, or
// the empty string if the field is absent or unparseable.
func readAPIVersion(data []byte) string {
	var probe struct {
		APIVersion string `yaml:"apiVersion"`
	}

	if err := yaml.Unmarshal(data, &probe); err != nil {
		return ""
	}

	return probe.APIVersion
}

// rewriteBlueprintYAML walks the blueprint.yaml node tree, rewrites the
// string fields that hold v1 template expressions (variable.default,
// condition.when, rename keys+values), and bumps apiVersion to v2.
//
// Uses yaml.Node to preserve structure and comments. Untranslated hits
// from expression-field rewrites are surfaced so the summary table
// reflects them.
func rewriteBlueprintYAML(path string, data []byte) ([]byte, []UntranslatedHit, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parsing yaml: %w", err)
	}

	root := documentRoot(&doc)
	if root == nil {
		return nil, nil, fmt.Errorf("blueprint.yaml has no document root")
	}

	var hits []UntranslatedHit

	rewriteScalar(root, "apiVersion", func(_ string) (string, []UntranslatedHit, error) {
		return apiVersionV2, nil, nil
	})

	rewriteVariableDefaults(root, path, &hits)
	rewriteConditionWhens(root, path, &hits)
	rewriteRenameMap(root, path, &hits)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling yaml: %w", err)
	}

	return out, hits, nil
}

// documentRoot returns the underlying mapping node of a yaml.Node loaded
// via Unmarshal — yaml.v3 wraps the actual content in a DocumentNode.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}

	return doc
}

// findMappingValue returns the value node for the given key inside a
// mapping node, or nil when the key is absent.
func findMappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}

	return nil
}

// rewriteScalar replaces the value of a scalar field inside a mapping
// using the given transform. The transform may surface UntranslatedHits;
// they are returned via the *[]UntranslatedHit caller.
func rewriteScalar(m *yaml.Node, key string, transform func(string) (string, []UntranslatedHit, error)) {
	v := findMappingValue(m, key)
	if v == nil || v.Kind != yaml.ScalarNode {
		return
	}

	out, _, err := transform(v.Value)
	if err != nil {
		return
	}

	v.Value = out
	v.Style = 0
}

func rewriteVariableDefaults(root *yaml.Node, fileName string, hits *[]UntranslatedHit) {
	vars := findMappingValue(root, "variables")
	if vars == nil || vars.Kind != yaml.SequenceNode {
		return
	}

	for _, varNode := range vars.Content {
		rewriteExpressionField(varNode, "default", fileName, hits)
	}
}

func rewriteConditionWhens(root *yaml.Node, fileName string, hits *[]UntranslatedHit) {
	conds := findMappingValue(root, "conditions")
	if conds == nil || conds.Kind != yaml.SequenceNode {
		return
	}

	for _, condNode := range conds.Content {
		v := findMappingValue(condNode, "when")
		if v == nil || v.Kind != yaml.ScalarNode || v.Value == "" {
			continue
		}

		out, fieldHits, err := RewriteCondition(fileName+":when", v.Value)
		if err != nil {
			continue
		}

		*hits = append(*hits, fieldHits...)
		v.Value = out
	}
}

// rewriteExpressionField rewrites one string-valued expression field.
// Empty values are left alone.
func rewriteExpressionField(parent *yaml.Node, key, fileName string, hits *[]UntranslatedHit) {
	v := findMappingValue(parent, key)
	if v == nil || v.Kind != yaml.ScalarNode || v.Value == "" {
		return
	}

	out, fieldHits, err := RewriteTemplate(fileName+":"+key, v.Value)
	if err != nil {
		return
	}

	*hits = append(*hits, fieldHits...)
	v.Value = out
}

// rewriteRenameMap rewrites both keys and values in the rename map. The
// rename map declares pattern → replacement pairs; both sides may
// contain template expressions.
func rewriteRenameMap(root *yaml.Node, fileName string, hits *[]UntranslatedHit) {
	rename := findMappingValue(root, "rename")
	if rename == nil || rename.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i+1 < len(rename.Content); i += 2 {
		key := rename.Content[i]
		val := rename.Content[i+1]

		if key.Kind == yaml.ScalarNode && key.Value != "" {
			out, fieldHits, err := RewriteTemplate(fileName+":rename.key", key.Value)
			if err == nil {
				*hits = append(*hits, fieldHits...)
				key.Value = out
			}
		}

		if val.Kind == yaml.ScalarNode && val.Value != "" {
			out, fieldHits, err := RewriteTemplate(fileName+":rename.value", val.Value)
			if err == nil {
				*hits = append(*hits, fieldHits...)
				val.Value = out
			}
		}
	}
}

// bumpRegistryAPIVersion rewrites registry.yaml's apiVersion from v1 to v2
// in place. registry.yaml has no expression fields so only the version
// literal changes.
func bumpRegistryAPIVersion(path string, dryRun bool) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}

	if readAPIVersion(data) == apiVersionV2 {
		return nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing yaml: %w", err)
	}

	root := documentRoot(&doc)

	rewriteScalar(root, "apiVersion", func(_ string) (string, []UntranslatedHit, error) {
		return apiVersionV2, nil, nil
	})

	if dryRun {
		return nil
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshalling yaml: %w", err)
	}

	return os.WriteFile(path, out, 0o644)
}
