package cli

import (
	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/logger"
)

var rootCmd = &cobra.Command{
	Use:   "camply",
	Short: "camply, the campsite finder ⛺️",
	Long:  `camply is a tool to find available campsites at your favorite campgrounds.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logger.Setup()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&logger.DebugMode, "debug", false, "Enable extra debugging output")
}
