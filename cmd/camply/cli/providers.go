package cli

import (
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/providers"
)

// newProvidersCmd lists providers from the registry rather than a hardcoded
// table. The old table advertised GoingToCamp and Yellowstone with no hint that
// selecting either one just errors out.
func newProvidersCmd(descs []providers.Descriptor) *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List the different camply providers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"Provider Name", "Description", "Status"})

			for _, d := range descs {
				status := "available"
				if d.Status != providers.StatusSupported {
					status = "not implemented yet"
				}
				t.AppendRow(table.Row{d.DisplayName, d.Description, status})
			}

			t.SetStyle(table.StyleRounded)
			t.Render()
			return nil
		},
	}
}
