package migratecmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// errDirtyWorktree is returned by checkCleanWorktree when the migration
// target lives inside a git worktree with uncommitted changes.
var errDirtyWorktree = errors.New(
	"refusing to migrate inside a dirty git worktree (use --force to override)",
)

// errNotAGitWorktree is returned when the migration target is not inside
// a git worktree at all. Per OQ-4, the guard fails closed: non-git
// targets must opt in via --force.
var errNotAGitWorktree = errors.New(
	"migration target is not inside a git worktree (use --force to override)",
)

// checkCleanWorktree runs `git status --porcelain` against path and
// returns nil iff path is inside a git worktree with no uncommitted or
// untracked changes. Uses git's `-C` flag so we don't need to chdir.
func checkCleanWorktree(path string) error {
	ctx := context.Background()

	//nolint:gosec // path comes from validated CLI input
	insideCmd := exec.CommandContext(
		ctx, "git", "-C", path, "rev-parse", "--is-inside-work-tree",
	)
	if err := insideCmd.Run(); err != nil {
		return errNotAGitWorktree
	}

	statusCmd := exec.CommandContext(ctx, "git", "-C", path, "status", "--porcelain") //nolint:gosec // path comes from validated CLI input

	out, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if strings.TrimSpace(string(out)) != "" {
		return errDirtyWorktree
	}

	return nil
}
