package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
	"github.com/kkweon/camply/internal/providers"
)

// campgroundsRunner holds one command invocation's flag values. Each command
// constructor allocates its own, so sibling commands cannot share state.
type campgroundsRunner struct {
	registry []providers.Descriptor

	provider    string
	search      string
	state       string
	recAreas    []string
	campgrounds []string
	campsites   []string

	renamedFlags map[string]string
}

func newCampgroundsCmd(descs []providers.Descriptor) *cobra.Command {
	r := &campgroundsRunner{registry: descs}

	cmd := &cobra.Command{
		Use:   "campgrounds",
		Short: "Search for Campgrounds",
		RunE:  r.run,
	}

	f := cmd.Flags()
	f.SetNormalizeFunc(aliasNormalizer(&r.renamedFlags))

	f.StringVar(&r.provider, "provider", "RecreationDotGov", "Camping Search Provider")
	f.StringVar(&r.search, "search", "", "Search string (one value)")
	f.StringVar(&r.state, "state", "", "State abbreviation (one value)")
	f.StringSliceVar(&r.recAreas, "rec-areas", []string{},
		multiHelp("Recreation Area IDs", "rec-areas 2991,1074", "rec-areas 2991", "rec-areas 1074"))
	f.StringSliceVar(&r.campgrounds, "campgrounds", []string{},
		multiHelp("Campground IDs", "campgrounds 232461,234039", "campgrounds 232461", "campgrounds 234039"))
	f.StringSliceVar(&r.campsites, "campsites", []string{},
		multiHelp("Campsite IDs", "campsites 2433,2437", "campsites 2433", "campsites 2437"))

	return cmd
}

func (r *campgroundsRunner) run(cmd *cobra.Command, _ []string) error {
	for _, w := range renamedFlagWarnings(r.renamedFlags) {
		logger.Warn("%s", w)
	}

	provider, desc, err := providers.NewFrom(r.registry, r.provider)
	if err != nil {
		return err
	}

	fmt.Printf("🏕  Searching %s for Campgrounds...\n", desc.DisplayName)

	// core.SearchRequest still carries a single recreation area, so query once
	// per ID and concatenate — the same shape the campsites command uses.
	areas := r.recAreas
	if len(areas) == 0 {
		areas = []string{""}
	}

	var facilities []core.CampgroundFacility
	for _, area := range areas {
		req := core.SearchRequest{
			Query:          r.search,
			State:          r.state,
			RecreationArea: area,
			Campgrounds:    r.campgrounds,
			Campsites:      r.campsites,
		}
		found, err := provider.FindCampgrounds(context.Background(), req)
		if err != nil {
			return fmt.Errorf("error fetching campgrounds: %w", err)
		}
		facilities = append(facilities, found...)
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
