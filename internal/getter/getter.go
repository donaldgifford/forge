// Package getter wraps hashicorp/go-getter for fetching registries and tool binaries.
package getter

import (
	"context"
	"fmt"
	"log/slog"

	getter "github.com/hashicorp/go-getter/v2"
)

// Getter wraps go-getter to fetch sources from git, HTTP, and other protocols.
type Getter struct {
	client *getter.Client
	logger *slog.Logger
}

// New creates a Getter with default configuration.
func New(logger *slog.Logger) *Getter {
	if logger == nil {
		logger = slog.Default()
	}

	return &Getter{
		client: &getter.Client{
			DisableSymlinks: true,
		},
		logger: logger,
	}
}

// FetchOpts configures a fetch operation.
type FetchOpts struct {
	// Ref is appended as ?ref= for git sources.
	Ref string

	// Checksum is appended as ?checksum=sha256: for verification.
	Checksum string

	// Pwd is the working directory for relative path detection.
	Pwd string
}

// Fetch downloads a source (directory) to the destination path.
// The src string uses go-getter URL syntax including // for subpath extraction.
func (g *Getter) Fetch(ctx context.Context, src, dest string, opts FetchOpts) error {
	fullSrc := appendQueryParams(src, opts)
	g.logger.Debug("fetching source", "src", fullSrc, "dest", dest)

	req := &getter.Request{
		Src:             fullSrc,
		Dst:             dest,
		Pwd:             opts.Pwd,
		GetMode:         getter.ModeDir,
		DisableSymlinks: true,
	}

	_, err := g.client.Get(ctx, req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", src, err)
	}

	return nil
}

// FetchFile downloads a single file from src to dest.
func (g *Getter) FetchFile(ctx context.Context, src, dest string, opts FetchOpts) error {
	fullSrc := appendQueryParams(src, opts)
	g.logger.Debug("fetching file", "src", fullSrc, "dest", dest)

	req := &getter.Request{
		Src:             fullSrc,
		Dst:             dest,
		Pwd:             opts.Pwd,
		GetMode:         getter.ModeFile,
		DisableSymlinks: true,
	}

	_, err := g.client.Get(ctx, req)
	if err != nil {
		return fmt.Errorf("fetching file %s: %w", src, err)
	}

	return nil
}

// appendQueryParams adds ref and checksum query parameters to a source URL.
func appendQueryParams(src string, opts FetchOpts) string {
	sep := "?"
	for _, c := range src {
		if c == '?' {
			sep = "&"

			break
		}
	}

	result := src

	if opts.Ref != "" {
		result += sep + "ref=" + opts.Ref
		sep = "&"
	}

	if opts.Checksum != "" {
		result += sep + "checksum=sha256:" + opts.Checksum
	}

	return result
}
