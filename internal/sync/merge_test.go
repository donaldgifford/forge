package sync_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	forgesync "github.com/donaldgifford/forge/internal/sync"
)

func TestThreeWayMerge_NoChanges(t *testing.T) {
	t.Parallel()

	base := []byte("line1\nline2\nline3\n")
	local := []byte("line1\nline2\nline3\n")
	remote := []byte("line1\nline2\nline3\n")

	result := forgesync.ThreeWayMerge(base, local, remote)

	require.NotNil(t, result)
	assert.False(t, result.HasConflicts)
	assert.Empty(t, result.Conflicts)
	// When base == remote, local is returned as-is.
	assert.Equal(t, local, result.Content)
}

func TestThreeWayMerge_OnlyRemoteChanged(t *testing.T) {
	t.Parallel()

	base := []byte("line1\nline2\nline3\n")
	local := []byte("line1\nline2\nline3\n")
	remote := []byte("line1\nline2-updated\nline3\n")

	result := forgesync.ThreeWayMerge(base, local, remote)

	require.NotNil(t, result)
	assert.False(t, result.HasConflicts)
	// When base == local, remote is accepted.
	assert.Equal(t, remote, result.Content)
}

func TestThreeWayMerge_OnlyLocalChanged(t *testing.T) {
	t.Parallel()

	base := []byte("line1\nline2\nline3\n")
	local := []byte("line1\nline2-local\nline3\n")
	remote := []byte("line1\nline2\nline3\n")

	result := forgesync.ThreeWayMerge(base, local, remote)

	require.NotNil(t, result)
	assert.False(t, result.HasConflicts)
	// When base == remote, local is kept.
	assert.Equal(t, local, result.Content)
}

func TestThreeWayMerge_BothChangedDifferentLines(t *testing.T) {
	t.Parallel()

	base := []byte("line1\nline2\nline3\n")
	local := []byte("line1-local\nline2\nline3\n")
	remote := []byte("line1\nline2\nline3-remote\n")

	result := forgesync.ThreeWayMerge(base, local, remote)

	require.NotNil(t, result)
	assert.False(t, result.HasConflicts)
	assert.Empty(t, result.Conflicts)
	assert.Contains(t, string(result.Content), "line1-local")
	assert.Contains(t, string(result.Content), "line3-remote")
}

func TestThreeWayMerge_BothChangedSameLine(t *testing.T) {
	t.Parallel()

	base := []byte("line1\nline2\nline3\n")
	local := []byte("line1\nline2-local\nline3\n")
	remote := []byte("line1\nline2-remote\nline3\n")

	result := forgesync.ThreeWayMerge(base, local, remote)

	require.NotNil(t, result)
	assert.True(t, result.HasConflicts)
	assert.Len(t, result.Conflicts, 1)
	assert.Equal(t, []string{"line2-local"}, result.Conflicts[0].LocalLines)
	assert.Equal(t, []string{"line2-remote"}, result.Conflicts[0].RemoteLines)

	content := string(result.Content)
	assert.Contains(t, content, "<<<<<<< local")
	assert.Contains(t, content, "line2-local")
	assert.Contains(t, content, "=======")
	assert.Contains(t, content, "line2-remote")
	assert.Contains(t, content, ">>>>>>> remote")
}

func TestThreeWayMerge_BothChangedToSame(t *testing.T) {
	t.Parallel()

	base := []byte("line1\nline2\nline3\n")
	local := []byte("line1\nline2-same\nline3\n")
	remote := []byte("line1\nline2-same\nline3\n")

	result := forgesync.ThreeWayMerge(base, local, remote)

	require.NotNil(t, result)
	assert.False(t, result.HasConflicts)
	assert.Contains(t, string(result.Content), "line2-same")
}

func TestThreeWayMerge_EmptyBase(t *testing.T) {
	t.Parallel()

	base := []byte("")
	local := []byte("local-line\n")
	remote := []byte("remote-line\n")

	result := forgesync.ThreeWayMerge(base, local, remote)

	require.NotNil(t, result)
	// Both sides added content — conflict expected.
	assert.True(t, result.HasConflicts)
}

func TestThreeWayMerge_MultipleConflicts(t *testing.T) {
	t.Parallel()

	base := []byte("a\nb\nc\nd\n")
	local := []byte("a-local\nb\nc-local\nd\n")
	remote := []byte("a-remote\nb\nc-remote\nd\n")

	result := forgesync.ThreeWayMerge(base, local, remote)

	require.NotNil(t, result)
	assert.True(t, result.HasConflicts)
	assert.Len(t, result.Conflicts, 2)
}
