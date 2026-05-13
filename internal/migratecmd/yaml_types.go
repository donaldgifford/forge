package migratecmd

// YAML-tagged shadow types for the migrate-config rewriter.
//
// internal/config/blueprint.go and registry.go dropped their `yaml:"..."`
// struct tags as part of the IMPL-0005 cutover (C.6) — the live config
// types are HCL-only, so YAML tags would be dead weight. The migrate
// tool still has to parse legacy YAML inputs, so it keeps its own
// tag-bearing copies and converts them into the public config shape
// for the emit helpers.
//
// These are intentionally near-duplicates of the config types: the
// rewriter is a one-shot tool, and a small static schema duplication is
// the right trade for keeping `internal/config/` free of yaml.v3
// references.

import "github.com/donaldgifford/forge/internal/config"

type yamlBlueprint struct {
	Name        string                `yaml:"name"`
	Description string                `yaml:"description"`
	Version     string                `yaml:"version"`
	Tags        []string              `yaml:"tags"`
	Defaults    yamlBlueprintDefaults `yaml:"defaults"`
	Variables   []yamlVariable        `yaml:"variables"`
	Conditions  []yamlCondition       `yaml:"conditions"`
	Hooks       yamlHooks             `yaml:"hooks"`
	Sync        yamlSync              `yaml:"sync"`
	Rename      map[string]string     `yaml:"rename"`
}

type yamlBlueprintDefaults struct {
	Exclude          []string          `yaml:"exclude"`
	OverrideStrategy map[string]string `yaml:"override_strategy"`
}

type yamlVariable struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Type        string   `yaml:"type"`
	Default     string   `yaml:"default"`
	Required    bool     `yaml:"required"`
	Validate    string   `yaml:"validate"`
	Choices     []string `yaml:"choices"`
}

type yamlCondition struct {
	When    string   `yaml:"when"`
	Exclude []string `yaml:"exclude"`
}

type yamlHooks struct {
	PostCreate []string `yaml:"post_create"`
}

type yamlSync struct {
	ManagedFiles []yamlManagedFile `yaml:"managed_files"`
	Ignore       []string          `yaml:"ignore"`
}

type yamlManagedFile struct {
	Path     string `yaml:"path"`
	Strategy string `yaml:"strategy"`
}

type yamlRegistry struct {
	Name        string               `yaml:"name"`
	Description string               `yaml:"description"`
	Maintainers []yamlMaintainer     `yaml:"maintainers"`
	Defaults    yamlRegistryDefaults `yaml:"defaults"`
	Blueprints  []yamlBlueprintEntry `yaml:"blueprints"`
}

type yamlMaintainer struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

type yamlRegistryDefaults struct {
	SyncStrategy string `yaml:"sync_strategy"`
	Managed      bool   `yaml:"managed"`
}

type yamlBlueprintEntry struct {
	Name         string   `yaml:"name"`
	Path         string   `yaml:"path"`
	Description  string   `yaml:"description"`
	Version      string   `yaml:"version"`
	Tags         []string `yaml:"tags"`
	LatestCommit string   `yaml:"latest_commit"`
}

// toBlueprint converts a YAML-decoded blueprint into the public
// config.Blueprint shape the emit helpers expect. Condition.WhenSource
// is populated from the YAML `when` string so the emitter can write it
// out verbatim.
func (y *yamlBlueprint) toBlueprint() *config.Blueprint {
	bp := &config.Blueprint{
		Name:        y.Name,
		Description: y.Description,
		Version:     y.Version,
		Tags:        y.Tags,
		Defaults: config.Defaults{
			Exclude:          y.Defaults.Exclude,
			OverrideStrategy: y.Defaults.OverrideStrategy,
		},
		Hooks:  config.Hooks{PostCreate: y.Hooks.PostCreate},
		Rename: y.Rename,
	}

	for _, v := range y.Variables {
		bp.Variables = append(bp.Variables, config.Variable{
			Name:        v.Name,
			Description: v.Description,
			Type:        v.Type,
			Default:     v.Default,
			Required:    v.Required,
			Validate:    v.Validate,
			Choices:     v.Choices,
		})
	}

	for _, c := range y.Conditions {
		bp.Conditions = append(bp.Conditions, config.Condition{
			WhenSource: c.When,
			Exclude:    c.Exclude,
		})
	}

	bp.Sync = config.SyncConfig{Ignore: y.Sync.Ignore}
	for _, mf := range y.Sync.ManagedFiles {
		bp.Sync.ManagedFiles = append(bp.Sync.ManagedFiles, config.ManagedFile{
			Path:     mf.Path,
			Strategy: mf.Strategy,
		})
	}

	return bp
}

// toRegistry converts a YAML-decoded registry into the public
// config.Registry shape.
func (y *yamlRegistry) toRegistry() *config.Registry {
	reg := &config.Registry{
		Name:        y.Name,
		Description: y.Description,
		Defaults: config.RegistryDefaults{
			SyncStrategy: y.Defaults.SyncStrategy,
			Managed:      y.Defaults.Managed,
		},
	}

	for _, m := range y.Maintainers {
		reg.Maintainers = append(reg.Maintainers, config.Maintainer{
			Name:  m.Name,
			Email: m.Email,
		})
	}

	for _, e := range y.Blueprints {
		reg.Blueprints = append(reg.Blueprints, config.BlueprintEntry{
			Name:         e.Name,
			Path:         e.Path,
			Description:  e.Description,
			Version:      e.Version,
			Tags:         e.Tags,
			LatestCommit: e.LatestCommit,
		})
	}

	return reg
}
