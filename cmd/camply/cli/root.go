package cli

import (
	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/logger"
	"github.com/kkweon/camply/internal/providers"
)

// newRootCmd builds the command tree against the real provider registry.
func newRootCmd() *cobra.Command {
	return newRootCmdWithRegistry(providers.Descriptors())
}

// newRootCmdWithRegistry builds a complete, independent command tree.
//
// Commands are built by constructors rather than registered from init(): cobra
// runs every init() regardless of which command executes, so package-level
// command variables end up sharing state between siblings. That is not
// hypothetical here — a single `providerStr` was bound to the --provider flag
// of three different commands at once.
//
// Taking the registry as a parameter also lets tests inject a fake provider and
// build a fresh tree per case, which matters because pflag values persist
// across Execute() calls on a reused tree.
func newRootCmdWithRegistry(descs []providers.Descriptor) *cobra.Command {
	var debug bool

	root := &cobra.Command{
		Use:   "camply",
		Short: "camply, the campsite finder ⛺️",
		Long:  `camply is a tool to find available campsites at your favorite campgrounds.`,
		// Validation errors are the point of this CLI; burying them under a
		// wall of usage text defeats that. main() prints the error itself.
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger.SetDebug(debug)
		},
	}

	root.PersistentFlags().BoolVar(&debug, "debug", false, "Enable extra debugging output")

	// Cobra's "unknown flag: --state" names the failure but not the fix.
	root.SetFlagErrorFunc(providerAwareFlagError(descs))

	// One group per usable provider, so a flag that means nothing for the
	// chosen provider is never registered.
	for _, d := range descs {
		if d.Status == providers.StatusSupported {
			root.AddCommand(newProviderGroupCmd(d))
		}
	}

	// Deprecated top-level commands; see legacy.go for why they stay.
	root.AddCommand(
		newCampsitesCmd(descs),
		newCampgroundsCmd(descs),
		newRecreationAreasCmd(descs),
	)

	root.AddCommand(
		newProvidersCmd(descs),
		newTestNotificationsCmd(),
	)

	return root
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return newRootCmd().Execute()
}
