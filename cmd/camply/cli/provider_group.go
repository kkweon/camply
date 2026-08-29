package cli

import (
	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/providers"
)

// newProviderGroupCmd builds `camply <provider>` with the searches that
// provider supports underneath it.
//
// Binding commands to a provider is the point of the split: a flag that means
// nothing for the chosen provider is never registered, so it cannot be passed
// by mistake. A ReserveCalifornia equipment name reaching RecreationDotGov is
// what silently discarded 423 of 667 campsites.
func newProviderGroupCmd(d providers.Descriptor) *cobra.Command {
	cmd := &cobra.Command{
		Use:     string(d.Key),
		Aliases: append([]string{d.DisplayName}, d.Aliases...),
		Short:   d.Description,
	}

	cmd.AddCommand(
		newProviderCampsitesCmd(d),
		newProviderCampgroundsCmd(d),
		newProviderRecreationAreasCmd(d),
	)

	return cmd
}

// recAreaDesc describes --rec-areas for one provider. The two providers use
// disjoint ID namespaces, so a RIDB ID passed to ReserveCalifornia matches
// nothing at all — the help text has to say which one is meant.
func recAreaDesc(d *providers.Descriptor) string {
	if d == nil || d.RecAreaIDHelp == "" {
		return "Recreation Area IDs"
	}
	return "Recreation Area IDs — " + d.RecAreaIDHelp
}
