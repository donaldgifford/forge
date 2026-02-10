// Package hooks executes lifecycle hooks for blueprint operations.
package hooks

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
)

// Opts configures hook execution.
type Opts struct {
	// Hooks is the list of shell commands to execute.
	Hooks []string
	// WorkDir is the directory in which hooks are executed.
	WorkDir string
	// Stdout receives hook standard output.
	Stdout io.Writer
	// Stderr receives hook standard error.
	Stderr io.Writer
	// Logger for debug output.
	Logger *slog.Logger
}

// RunPostCreate executes post-create hooks in order. If a hook fails,
// a warning is logged but execution continues — the project files are
// already written so aborting would not help.
func RunPostCreate(ctx context.Context, opts *Opts) []error {
	if len(opts.Hooks) == 0 {
		return nil
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var errs []error

	for _, hook := range opts.Hooks {
		logger.Debug("running post-create hook", "cmd", hook)

		if err := runHook(ctx, hook, opts); err != nil {
			logger.Warn("post-create hook failed", "cmd", hook, "err", err)
			errs = append(errs, fmt.Errorf("hook %q: %w", hook, err))
		}
	}

	return errs
}

func runHook(ctx context.Context, hook string, opts *Opts) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", hook)
	cmd.Dir = opts.WorkDir
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	return cmd.Run()
}
