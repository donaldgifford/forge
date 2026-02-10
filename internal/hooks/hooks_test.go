package hooks_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donaldgifford/forge/internal/hooks"
)

func TestRunPostCreate_Success(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	opts := &hooks.Opts{
		Hooks:   []string{"echo hello"},
		WorkDir: t.TempDir(),
		Stdout:  &stdout,
		Stderr:  &stderr,
	}

	errs := hooks.RunPostCreate(t.Context(), opts)
	assert.Empty(t, errs)
	assert.Contains(t, stdout.String(), "hello")
}

func TestRunPostCreate_WorkDir(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	var stdout bytes.Buffer

	opts := &hooks.Opts{
		Hooks:   []string{"pwd"},
		WorkDir: workDir,
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	}

	errs := hooks.RunPostCreate(t.Context(), opts)
	assert.Empty(t, errs)
	assert.Contains(t, stdout.String(), workDir)
}

func TestRunPostCreate_FailureContinues(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	opts := &hooks.Opts{
		Hooks:   []string{"exit 1", "echo after-failure"},
		WorkDir: t.TempDir(),
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	}

	errs := hooks.RunPostCreate(t.Context(), opts)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "exit 1")

	// Second hook should still run.
	assert.Contains(t, stdout.String(), "after-failure")
}

func TestRunPostCreate_NoHooks(t *testing.T) {
	t.Parallel()

	opts := &hooks.Opts{
		Hooks:   nil,
		WorkDir: t.TempDir(),
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}

	errs := hooks.RunPostCreate(t.Context(), opts)
	assert.Empty(t, errs)
}

func TestRunPostCreate_MultipleHooks(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	opts := &hooks.Opts{
		Hooks:   []string{"echo first", "echo second", "echo third"},
		WorkDir: t.TempDir(),
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	}

	errs := hooks.RunPostCreate(t.Context(), opts)
	assert.Empty(t, errs)
	assert.Contains(t, stdout.String(), "first")
	assert.Contains(t, stdout.String(), "second")
	assert.Contains(t, stdout.String(), "third")
}
