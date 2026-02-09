// Package ui provides consistent styled output for the forge CLI.
package ui

import (
	"fmt"
	"io"
	"os"
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// Writer provides styled output methods that respect color settings.
type Writer struct {
	out     io.Writer
	errOut  io.Writer
	noColor bool
}

// NewWriter creates a Writer that writes to stdout/stderr.
// Color is disabled when noColor is true or the NO_COLOR env var is set.
func NewWriter(noColor bool) *Writer {
	return &Writer{
		out:     os.Stdout,
		errOut:  os.Stderr,
		noColor: noColor || os.Getenv("NO_COLOR") != "",
	}
}

// NewWriterWithOutputs creates a Writer with custom output destinations.
// Intended for testing.
func NewWriterWithOutputs(out, errOut io.Writer, noColor bool) *Writer {
	return &Writer{
		out:     out,
		errOut:  errOut,
		noColor: noColor,
	}
}

// Success prints a success message with a green checkmark prefix.
func (w *Writer) Success(msg string) {
	writeLine(w.out, w.styled(colorGreen, "\u2713"), msg)
}

// Warning prints a warning message to stderr with a yellow prefix.
func (w *Writer) Warning(msg string) {
	writeLine(w.errOut, w.styled(colorYellow, "warning:"), msg)
}

// Error prints an error message to stderr with a red prefix.
func (w *Writer) Error(msg string) {
	writeLine(w.errOut, w.styled(colorRed, "error:"), msg)
}

// Info prints an informational message with a cyan prefix.
func (w *Writer) Info(msg string) {
	writeLine(w.out, w.styled(colorCyan, "info:"), msg)
}

// Bold prints text in bold.
func (w *Writer) Bold(msg string) string {
	return w.styled(colorBold, msg)
}

// Successf prints a formatted success message.
func (w *Writer) Successf(format string, args ...any) {
	w.Success(fmt.Sprintf(format, args...))
}

// Warningf prints a formatted warning message.
func (w *Writer) Warningf(format string, args ...any) {
	w.Warning(fmt.Sprintf(format, args...))
}

// Errorf prints a formatted error message.
func (w *Writer) Errorf(format string, args ...any) {
	w.Error(fmt.Sprintf(format, args...))
}

// Infof prints a formatted informational message.
func (w *Writer) Infof(format string, args ...any) {
	w.Info(fmt.Sprintf(format, args...))
}

func (w *Writer) styled(color, text string) string {
	if w.noColor {
		return text
	}

	return color + text + colorReset
}

func writeLine(out io.Writer, prefix, msg string) {
	if _, err := fmt.Fprintf(out, "%s %s\n", prefix, msg); err != nil {
		// Best-effort output; if stderr fails there's nothing useful to do.
		return
	}
}
