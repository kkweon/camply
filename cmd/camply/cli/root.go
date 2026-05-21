package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "camply",
	Short: "camply, the campsite finder ⛺️",
	Long:  `camply is a tool to find available campsites at your favorite campgrounds.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Add global flags here if needed
}
