package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/kkweon/camply/internal/logger"
	"github.com/kkweon/camply/internal/providers"
)

// The top-level campsites/campgrounds/recreation-areas commands stay for now,
// deprecated in favour of `camply <provider> <command>`.
//
// They cannot simply be deleted: the CronJobs run ghcr.io/kkweon/camply:latest-go
// with imagePullPolicy Always, so a new image reaches them before anyone can
// edit a manifest. Removing the old surface would take every scheduled search
// down at the moment of an unrelated release.
const deprecationNotice = "use 'camply <provider> %s' instead, e.g. " +
	"'camply recdotgov %s'. This command will be removed in a future release."

func newCampsitesCmd(descs []providers.Descriptor) *cobra.Command {
	r := &campsitesRunner{registry: descs}

	cmd := &cobra.Command{
		Use:        "campsites",
		Short:      "Find available campsites (deprecated)",
		Deprecated: fmt.Sprintf(deprecationNotice, "campsites", "campsites"),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return warnLegacyUsage(cmd, "campsites", descs, r.provider)
		},
		RunE: r.run,
	}

	addSearchFlags(cmd, r, nil)

	return cmd
}

func newCampgroundsCmd(descs []providers.Descriptor) *cobra.Command {
	r := &campgroundsRunner{registry: descs}

	cmd := &cobra.Command{
		Use:        "campgrounds",
		Short:      "Search for Campgrounds (deprecated)",
		Deprecated: fmt.Sprintf(deprecationNotice, "campgrounds", "campgrounds"),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return warnLegacyUsage(cmd, "campgrounds", descs, r.provider)
		},
		RunE: r.run,
	}

	addCampgroundLookupFlags(cmd, r, nil)

	return cmd
}

func newRecreationAreasCmd(descs []providers.Descriptor) *cobra.Command {
	r := &recAreasRunner{registry: descs}

	cmd := &cobra.Command{
		Use:        "recreation-areas",
		Short:      "Search for Recreation Areas (deprecated)",
		Deprecated: fmt.Sprintf(deprecationNotice, "recreation-areas", "recreation-areas"),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return warnLegacyUsage(cmd, "recreation-areas", descs, r.provider)
		},
		RunE: r.run,
	}

	addRecAreaLookupFlags(cmd, r, nil)

	return cmd
}

// warnLegacyUsage prints the equivalent new command, built from the flags the
// user actually passed. Telling someone their command is deprecated without
// showing the replacement leaves them to work it out; this line can be pasted
// straight into a manifest.
//
// It also reports flags the chosen provider ignores. Those warn rather than
// fail here: the whole point of keeping this surface is not breaking running
// jobs.
func warnLegacyUsage(cmd *cobra.Command, sub string, descs []providers.Descriptor, provider string) error {
	d, ok := providers.LookupIn(descs, provider)
	if !ok {
		// Let RunE produce the real error, which names the usable providers.
		return nil
	}

	logger.Warn("'camply %s --provider %s' is deprecated. Use:\n    %s",
		sub, provider, replacementCommand(cmd, sub, d))

	if !d.SupportsState && cmd.Flags().Changed("state") {
		logger.Warn("--state has no effect on %s; its API has no state filter.", d.DisplayName)
	}
	return nil
}

// replacementCommand renders the equivalent `camply <provider> <sub> ...`.
func replacementCommand(cmd *cobra.Command, sub string, d providers.Descriptor) string {
	parts := []string{"camply", string(d.Key), sub}

	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == "provider" {
			return // the provider is the subcommand now
		}
		if f.Value.Type() == "bool" {
			parts = append(parts, "--"+f.Name)
			return
		}
		parts = append(parts, "--"+f.Name, shellQuote(flagValueForCLI(f)))
	})

	return strings.Join(parts, " ")
}

// flagValueForCLI renders a flag value the way it would be typed. pflag prints
// slices as "[a,b]", which is not valid input.
func flagValueForCLI(f *pflag.Flag) string {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		return strings.Join(sv.GetSlice(), ",")
	}
	return f.Value.String()
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	if strings.ContainsAny(v, " \t'\"$`\\") {
		return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
	}
	return v
}
