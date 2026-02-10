# Tools Guide

Forge can manage CLI tool dependencies declared in blueprints. Tools are downloaded, cached, and installed automatically.

## Declaring Tools

Tools are declared in `blueprint.yaml`:

```yaml
tools:
  - name: golangci-lint
    version: v1.60.0
    description: Go linter aggregator
    source:
      type: github-release
      repo: golangci/golangci-lint
      asset_pattern: "golangci-lint-{{version}}-{{os}}-{{arch}}.tar.gz"
    checksum:
      sha256:
        darwin-arm64: "abc123..."
        linux-amd64: "def456..."
  - name: goreleaser
    version: v2.1.0
    source:
      type: github-release
      repo: goreleaser/goreleaser
      asset_pattern: "goreleaser_{{OS}}_{{arch}}.tar.gz"
```

## Source Types

### github-release

Download from GitHub release assets:

```yaml
source:
  type: github-release
  repo: owner/repo
  asset_pattern: "tool-{{version}}-{{os}}-{{arch}}.tar.gz"
```

### url

Download from a direct URL:

```yaml
source:
  type: url
  url: "https://example.com/tool-{{version}}-{{os}}-{{arch}}.tar.gz"
```

### go-install

Install using `go install`:

```yaml
source:
  type: go-install
  module: github.com/owner/tool
```

## Template Variables

Asset patterns and URLs support these template variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `{{version}}` | Tool version (with `v` prefix) | `v1.60.0` |
| `{{os}}` | Operating system (lowercase) | `darwin`, `linux` |
| `{{arch}}` | Architecture | `arm64`, `amd64` |
| `{{goos}}` | Go OS name (same as `os`) | `darwin`, `linux` |
| `{{goarch}}` | Go architecture (same as `arch`) | `arm64`, `amd64` |
| `{{OS}}` | Capitalized OS | `Darwin`, `Linux` |
| `{{ARCH}}` | Capitalized arch | `ARM64`, `X86_64` |

## Commands

```bash
# List tools declared in the current project
forge tools list

# Install all declared tools
forge tools install

# Sync also updates tools with --include-tools
forge sync --include-tools
```

## Caching

Downloaded tools are cached in `~/.cache/forge/tools/` (or `$XDG_CACHE_HOME/forge/tools/`). Each tool version is stored in its own directory:

```
~/.cache/forge/tools/
  golangci-lint/
    v1.60.0/
      golangci-lint
  goreleaser/
    v2.1.0/
      goreleaser
```

To clear the tool cache:

```bash
forge cache clean --tools
```

## Inheritance

Tools can be declared at multiple levels:

1. **Registry-level** (`_tools.yaml` in registry root)
2. **Category-level** (`_tools.yaml` in category `_defaults/`)
3. **Blueprint-level** (`tools` section in `blueprint.yaml`)

More specific declarations override less specific ones (same name). Use `condition` to conditionally include tools:

```yaml
tools:
  - name: docker-compose
    version: v2.29.0
    condition: '{{ eq .use_docker "true" }}'
    source:
      type: github-release
      repo: docker/compose
      asset_pattern: "docker-compose-{{os}}-{{arch}}"
```

## Checksums

For security, declare SHA256 checksums per platform:

```yaml
checksum:
  sha256:
    darwin-arm64: "abc123def456..."
    darwin-amd64: "789012ghi345..."
    linux-amd64: "jkl678mno901..."
```

The download is verified against the checksum before installation.
