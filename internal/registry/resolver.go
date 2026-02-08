// Package registry handles blueprint resolution, registry indexing, and caching.
package registry

import (
	"fmt"
	"strings"
)

// ResolvedBlueprint holds the fully resolved location of a blueprint.
type ResolvedBlueprint struct {
	// RegistryURL is the base URL of the registry (e.g., "github.com/acme/blueprints").
	RegistryURL string

	// BlueprintPath is the subpath within the registry (e.g., "go/api").
	BlueprintPath string

	// Ref is the git ref to fetch (tag, branch, commit). Empty means latest/HEAD.
	Ref string

	// Standalone indicates this is a standalone blueprint (no registry.yaml expected).
	Standalone bool
}

// Resolve parses a user-provided blueprint reference into a ResolvedBlueprint.
//
// Supported input formats:
//   - "go/api" — short name, resolved against the default registry
//   - "go/api@v2.1.0" — short name with pinned ref
//   - "github.com/acme/blueprints//go/api" — full go-getter URL with subpath
//   - "github.com/acme/blueprints//go/api?ref=v2.1.0" — full URL with ref
//   - "git@github.com:someone/blueprint.git" — standalone (SSH)
//   - "https://github.com/someone/blueprint.git" — standalone (HTTPS)
func Resolve(input, defaultRegistryURL string) (*ResolvedBlueprint, error) {
	if input == "" {
		return nil, fmt.Errorf("blueprint reference cannot be empty")
	}

	// SSH git URL: git@host:owner/repo.git
	if strings.HasPrefix(input, "git@") {
		return &ResolvedBlueprint{
			RegistryURL: input,
			Standalone:  true,
		}, nil
	}

	// HTTPS .git URL: standalone blueprint
	if strings.HasSuffix(input, ".git") {
		return &ResolvedBlueprint{
			RegistryURL: input,
			Standalone:  true,
		}, nil
	}

	// Full go-getter URL with double-slash subpath separator
	if strings.Contains(input, "//") {
		return resolveFullURL(input)
	}

	// Short name format: "go/api" or "go/api@v2.1.0"
	return resolveShortName(input, defaultRegistryURL)
}

// resolveFullURL parses a go-getter URL like "github.com/acme/blueprints//go/api?ref=v2.1.0".
func resolveFullURL(input string) (*ResolvedBlueprint, error) {
	parts := strings.SplitN(input, "//", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid go-getter URL %q: expected format host/repo//subpath", input)
	}

	registryURL := parts[0]
	subpathAndQuery := parts[1]

	blueprintPath, ref := splitQueryRef(subpathAndQuery)

	if blueprintPath == "" {
		return nil, fmt.Errorf("invalid go-getter URL %q: empty blueprint path after //", input)
	}

	return &ResolvedBlueprint{
		RegistryURL:   registryURL,
		BlueprintPath: blueprintPath,
		Ref:           ref,
	}, nil
}

// resolveShortName parses "go/api" or "go/api@v2.1.0" against the default registry.
func resolveShortName(input, defaultRegistryURL string) (*ResolvedBlueprint, error) {
	if defaultRegistryURL == "" {
		return nil, fmt.Errorf("no default registry configured; use a full URL or configure a default registry")
	}

	name, ref := splitAtRef(input)

	if name == "" {
		return nil, fmt.Errorf("invalid blueprint reference %q: empty name", input)
	}

	return &ResolvedBlueprint{
		RegistryURL:   defaultRegistryURL,
		BlueprintPath: name,
		Ref:           ref,
	}, nil
}

// splitAtRef splits "go/api@v2.1.0" into ("go/api", "v2.1.0").
// If no @ is present, ref is empty.
func splitAtRef(input string) (name, ref string) {
	idx := strings.LastIndex(input, "@")
	if idx < 0 {
		return input, ""
	}

	return input[:idx], input[idx+1:]
}

// splitQueryRef extracts the ref query parameter from a subpath string.
// For example, "go/api?ref=v2.1.0" returns ("go/api", "v2.1.0").
func splitQueryRef(subpathAndQuery string) (subpath, ref string) {
	subpath, query, hasQuery := strings.Cut(subpathAndQuery, "?")
	if !hasQuery {
		return subpath, ""
	}

	for param := range strings.SplitSeq(query, "&") {
		if v, found := strings.CutPrefix(param, "ref="); found {
			ref = v

			break
		}
	}

	return subpath, ref
}

// GetterURL constructs the full go-getter URL from a resolved blueprint.
func (r *ResolvedBlueprint) GetterURL() string {
	if r.Standalone {
		return r.RegistryURL
	}

	url := r.RegistryURL + "//" + r.BlueprintPath

	if r.Ref != "" {
		url += "?ref=" + r.Ref
	}

	return url
}
