package migratecmd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/migratecmd"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()

	ctx := context.Background()

	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-q", "-m", "init"},
	} {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		require.NoError(t, cmd.Run(), "git %v", args)
	}
}

func TestRunMigrate_GuardRejectsNonGitWorktree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtureBlueprintV1(t, root)

	_, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not inside a git worktree")
}

func TestRunMigrate_GuardRejectsDirtyWorktree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)
	fixtureBlueprintV1(t, root)

	_, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dirty git worktree")
}

func TestRunMigrate_GuardPassesOnCleanWorktree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)
	fixtureBlueprintV1(t, root)

	ctx := context.Background()

	for _, args := range [][]string{
		{"add", "-A"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "fixture"},
	} {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		require.NoError(t, cmd.Run())
	}

	result, err := migratecmd.RunMigrate(&migratecmd.MigrateOpts{Path: root})
	require.NoError(t, err)
	require.Len(t, result.Blueprints, 1)
	assert.True(t, result.Blueprints[0].Migrated)
}

func TestRunMigrate_DryRunSkipsGuard(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtureBlueprintV1(t, root)

	bpPath := filepath.Join(root, "go", "api", "blueprint.yaml")
	before, err := os.ReadFile(bpPath)
	require.NoError(t, err)

	_, err = migratecmd.RunMigrate(&migratecmd.MigrateOpts{
		Path:   root,
		DryRun: true,
	})
	require.NoError(t, err)

	after, err := os.ReadFile(bpPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}
