// Package main is the entry point for the forge CLI.
package main

import (
	"os"

	"github.com/donaldgifford/forge/cmd"
)

// Build-time variables set via ldflags.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	cmd.SetVersionInfo(version, commit)

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
