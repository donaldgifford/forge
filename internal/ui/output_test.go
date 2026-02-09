package ui_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/donaldgifford/forge/internal/ui"
)

func TestWriter_Success_NoColor(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := ui.NewWriterWithOutputs(&buf, &bytes.Buffer{}, true)

	w.Success("done")

	assert.Contains(t, buf.String(), "\u2713")
	assert.Contains(t, buf.String(), "done")
	assert.NotContains(t, buf.String(), "\033[")
}

func TestWriter_Success_WithColor(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := ui.NewWriterWithOutputs(&buf, &bytes.Buffer{}, false)

	w.Success("done")

	assert.Contains(t, buf.String(), "\033[32m") // green
	assert.Contains(t, buf.String(), "done")
}

func TestWriter_Warning(t *testing.T) {
	t.Parallel()

	var errBuf bytes.Buffer
	w := ui.NewWriterWithOutputs(&bytes.Buffer{}, &errBuf, true)

	w.Warning("caution")

	assert.Contains(t, errBuf.String(), "warning:")
	assert.Contains(t, errBuf.String(), "caution")
}

func TestWriter_Error(t *testing.T) {
	t.Parallel()

	var errBuf bytes.Buffer
	w := ui.NewWriterWithOutputs(&bytes.Buffer{}, &errBuf, true)

	w.Error("something broke")

	assert.Contains(t, errBuf.String(), "error:")
	assert.Contains(t, errBuf.String(), "something broke")
}

func TestWriter_Info(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := ui.NewWriterWithOutputs(&buf, &bytes.Buffer{}, true)

	w.Info("status update")

	assert.Contains(t, buf.String(), "info:")
	assert.Contains(t, buf.String(), "status update")
}

func TestWriter_Formatted(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := ui.NewWriterWithOutputs(&buf, &bytes.Buffer{}, true)

	w.Successf("created %d files", 5)

	assert.Contains(t, buf.String(), "created 5 files")
}

func TestWriter_Bold_NoColor(t *testing.T) {
	t.Parallel()

	w := ui.NewWriterWithOutputs(&bytes.Buffer{}, &bytes.Buffer{}, true)

	result := w.Bold("text")
	assert.Equal(t, "text", result)
}

func TestWriter_Bold_WithColor(t *testing.T) {
	t.Parallel()

	w := ui.NewWriterWithOutputs(&bytes.Buffer{}, &bytes.Buffer{}, false)

	result := w.Bold("text")
	assert.Contains(t, result, "\033[1m")
}
