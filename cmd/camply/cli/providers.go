package cli

import (
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "List the different camply providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"Provider Name", "Description"})

		t.AppendRows([]table.Row{
			{"RecreationDotGov", "Recreation.gov API (US Federal)"},
			{"ReserveCalifornia", "Reserve California API"},
			{"GoingToCamp", "GoingToCamp.com API (Canada & US)"},
			{"Yellowstone", "Yellowstone National Park Lodges API"},
		})

		t.SetStyle(table.StyleRounded)
		t.Render()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(providersCmd)
}
