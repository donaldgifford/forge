package getter

import "fmt"

// RegistryURL constructs a go-getter URL for fetching a registry subpath.
//
// The double-slash separates the repository from the subpath, which is
// native go-getter syntax. For example:
//
//	RegistryURL("github.com/acme/blueprints", "go/api", "v2.1.0")
//	→ "github.com/acme/blueprints//go/api?ref=v2.1.0"
func RegistryURL(baseURL, subpath, ref string) string {
	url := baseURL + "//" + subpath

	if ref != "" {
		url += "?ref=" + ref
	}

	return url
}

// ToolReleaseURL constructs a go-getter URL for downloading a GitHub release asset.
func ToolReleaseURL(repo, version, asset string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", repo, version, asset)
}
