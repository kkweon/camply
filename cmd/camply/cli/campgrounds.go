package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/providers"
)

// campgroundsRunner holds one command invocation's flag values. Each command
// constructor allocates its own, so sibling commands cannot share state.
type campgroundsRunner struct {
	registry []providers.Descriptor

	provider    string
	search      string
	state       string
	recArea     string
	campgrounds []string
	campsites   []string
}

func newCampgroundsCmd(descs []providers.Descriptor) *cobra.Command {
	r := &campgroundsRunner{registry: descs}

	cmd := &cobra.Command{
		Use:   "campgrounds",
		Short: "Search for Campgrounds",
		RunE:  r.run,
	}

	cmd.Flags().StringVar(&r.provider, "provider", "RecreationDotGov", "Camping Search Provider")
	cmd.Flags().StringVar(&r.search, "search", "", "Search string")
	cmd.Flags().StringVar(&r.state, "state", "", "State abbreviation")
	cmd.Flags().StringVar(&r.recArea, "rec-area", "", "Recreation Area ID")
	cmd.Flags().StringSliceVar(&r.campgrounds, "campground", []string{}, "Campground ID(s)")
	cmd.Flags().StringSliceVar(&r.campsites, "campsite", []string{}, "Campsite ID(s)")

	return cmd
}

func (r *campgroundsRunner) run(cmd *cobra.Command, _ []string) error {
	req := core.SearchRequest{
		Query:          r.search,
		State:          r.state,
		RecreationArea: r.recArea,
		Campgrounds:    r.campgrounds,
		Campsites:      r.campsites,
	}

	provider, desc, err := providers.NewFrom(r.registry, r.provider)
	if err != nil {
		return err
	}

	fmt.Printf("🏕  Searching %s for Campgrounds...\n", desc.DisplayName)

	facilities, err := provider.FindCampgrounds(context.Background(), req)
	if err != nil {
		return fmt.Errorf("error fetching campgrounds: %w", err)
	}

	printCampgroundsTable(facilities)
	return nil
}

func printCampgroundsTable(facilities []core.CampgroundFacility) {
	if len(facilities) == 0 {
		fmt.Println("❌ No campgrounds found matching your criteria.")
		return
	}

	// Group by Recreation Area Name
	grouped := make(map[string][]core.CampgroundFacility)
	for _, f := range facilities {
		grouped[f.RecreationArea] = append(grouped[f.RecreationArea], f)
	}

	for recAreaName, camps := range grouped {
		// Assuming all camps in the group share the same RecAreaID
		recAreaID := camps[0].RecreationAreaID
		fmt.Printf("🏞  %s - (#%s)\n", recAreaName, recAreaID)
		for _, f := range camps {
			fmt.Printf("    🏕  %s - (#%s)\n", f.FacilityName, f.FacilityID)
		}
	}

	fmt.Printf("\n✅ Found %d matching campground(s)!\n", len(facilities))
}
