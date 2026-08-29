package cli

import (
	"strings"

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
func newProviderGroupCmd(d providers.Descriptor, registry []providers.Descriptor) *cobra.Command {
	cmd := &cobra.Command{
		Use:     string(d.Key),
		Aliases: append([]string{d.DisplayName}, d.Aliases...),
		Short:   d.Description,
	}

	cmd.AddCommand(
		newProviderCampsitesCmd(d, registry),
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

// equipmentHelp describes --equipment-types for one provider, listing the
// closed vocabularies in full so the accepted values are visible in --help.
func equipmentHelp(d *providers.Descriptor) string {
	base := "Equipment types a campsite must permit"
	if d == nil {
		return base
	}
	v, ok := providers.LookupVocabulary(*d, providers.FlagEquipmentTypes)
	if !ok {
		return base
	}
	if v.Closed {
		return base + " — one of: " + strings.Join(v.Values, ", ")
	}
	return base + " — varies by campground; " + v.Source
}

// campsiteTypeHelp describes --campsite-types for one provider. This is the
// field that separates drive-in sites from walk-in ones; equipment cannot,
// because a walk-in site still permits a tent.
func campsiteTypeHelp(d *providers.Descriptor) string {
	base := "Campsite types to accept — this is what separates drive-in from walk-in sites"
	if d == nil {
		return base
	}
	v, ok := providers.LookupVocabulary(*d, providers.FlagCampsiteTypes)
	if !ok {
		return base
	}
	return base + "; " + v.Source
}
