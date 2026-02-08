package tools

import (
	"runtime"
	"strings"
)

// Platform holds OS/architecture information for tool downloads.
type Platform struct {
	OS   string // lowercase: "linux", "darwin", "windows"
	Arch string // lowercase: "amd64", "arm64"
}

// DetectPlatform returns the current platform information.
func DetectPlatform() Platform {
	return Platform{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
}

// ResolveAssetURL renders platform-specific variables in an asset pattern.
// Supported variables: {{os}}, {{arch}}, {{goos}}, {{goarch}}, {{version}},
// {{OS}} (capitalized), {{ARCH}} (capitalized).
func ResolveAssetURL(pattern, version string, platform Platform) string {
	r := strings.NewReplacer(
		"{{os}}", platform.OS,
		"{{arch}}", platform.Arch,
		"{{goos}}", platform.OS,
		"{{goarch}}", platform.Arch,
		"{{OS}}", capitalize(platform.OS),
		"{{ARCH}}", capitalize(platform.Arch),
		"{{version}}", version,
	)

	return r.Replace(pattern)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}

	return strings.ToUpper(s[:1]) + s[1:]
}
