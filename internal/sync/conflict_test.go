package sync_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	forgesync "github.com/donaldgifford/forge/internal/sync"
)

func TestReportConflicts_NoConflicts(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := forgesync.ReportConflicts(&buf, nil)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestReportConflicts_WithConflicts(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	files := []forgesync.ConflictFile{
		{
			Path: "Makefile",
			Conflicts: []forgesync.Conflict{
				{LocalLines: []string{"local"}, RemoteLines: []string{"remote"}},
			},
		},
		{
			Path: "config.yaml",
			Conflicts: []forgesync.Conflict{
				{LocalLines: []string{"a"}, RemoteLines: []string{"b"}},
				{LocalLines: []string{"c"}, RemoteLines: []string{"d"}},
			},
		},
	}

	err := forgesync.ReportConflicts(&buf, files)

	var conflictErr *forgesync.ConflictError

	require.ErrorAs(t, err, &conflictErr)
	assert.Len(t, conflictErr.Files, 2)

	output := buf.String()
	assert.Contains(t, output, "Conflicts detected in 2 file(s)")
	assert.Contains(t, output, "CONFLICT Makefile (1 conflict region(s))")
	assert.Contains(t, output, "CONFLICT config.yaml (2 conflict region(s))")
	assert.Contains(t, output, "Resolve conflicts manually")
}

func TestStripConflictMarkers_KeepLocal(t *testing.T) {
	t.Parallel()

	content := "line1\n<<<<<<< local\nlocal-change\n=======\nremote-change\n>>>>>>> remote\nline3\n"

	result := forgesync.StripConflictMarkers(content, "local")

	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "local-change")
	assert.NotContains(t, result, "remote-change")
	assert.NotContains(t, result, "<<<<<<<")
	assert.NotContains(t, result, ">>>>>>>")
	assert.Contains(t, result, "line3")
}

func TestStripConflictMarkers_KeepRemote(t *testing.T) {
	t.Parallel()

	content := "line1\n<<<<<<< local\nlocal-change\n=======\nremote-change\n>>>>>>> remote\nline3\n"

	result := forgesync.StripConflictMarkers(content, "remote")

	assert.Contains(t, result, "line1")
	assert.NotContains(t, result, "local-change")
	assert.Contains(t, result, "remote-change")
	assert.NotContains(t, result, "<<<<<<<")
	assert.NotContains(t, result, ">>>>>>>")
	assert.Contains(t, result, "line3")
}

func TestStripConflictMarkers_MultipleConflicts(t *testing.T) {
	t.Parallel()

	content := "a\n<<<<<<< local\nlocal1\n=======\nremote1\n>>>>>>> remote\nb\n<<<<<<< local\nlocal2\n=======\nremote2\n>>>>>>> remote\nc\n"

	result := forgesync.StripConflictMarkers(content, "local")

	assert.Contains(t, result, "a")
	assert.Contains(t, result, "local1")
	assert.Contains(t, result, "b")
	assert.Contains(t, result, "local2")
	assert.Contains(t, result, "c")
	assert.NotContains(t, result, "remote1")
	assert.NotContains(t, result, "remote2")
}

func TestStripConflictMarkers_NoConflicts(t *testing.T) {
	t.Parallel()

	content := "line1\nline2\nline3\n"

	result := forgesync.StripConflictMarkers(content, "local")

	assert.Equal(t, content, result)
}

func TestConflictError_Message(t *testing.T) {
	t.Parallel()

	err := &forgesync.ConflictError{
		Files: []forgesync.ConflictFile{
			{Path: "a.txt"},
			{Path: "b.txt"},
		},
	}

	assert.Equal(t, "2 file(s) have merge conflicts", err.Error())
}
