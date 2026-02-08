package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/donaldgifford/forge/internal/lockfile"
)

var toolsListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List installed tools",
	Long:    `List tools tracked in the project's .forge-lock.yaml.`,
	Aliases: []string{"ls"},
	RunE:    runToolsList,
}

func init() {
	toolsCmd.AddCommand(toolsListCmd)
}

func runToolsList(_ *cobra.Command, _ []string) error {
	lock, err := lockfile.Read(lockfile.FileName)
	if err != nil {
		return fmt.Errorf("reading lockfile: %w (is this a forge project?)", err)
	}

	if len(lock.Tools) == 0 {
		fmt.Println("No tools declared in this project.")

		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "TOOL\tVERSION\tSOURCE"); err != nil {
		return err
	}

	for i := range lock.Tools {
		t := &lock.Tools[i]
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Name, t.Version, t.Source); err != nil {
			return err
		}
	}

	return tw.Flush()
}
