package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/providers/recdotgov"
	"github.com/kkweon/camply/internal/providers/usedirect"
)

var (
	cgSearchStr  string
	cgStateStr   string
	cgRecArea    string
	cgCampground []string
	cgCampsite   []string
)

var campgroundsCmd = &cobra.Command{
	Use:   "campgrounds",
	Short: "Search for Campgrounds",
	RunE: func(cmd *cobra.Command, args []string) error {
		req := core.SearchRequest{
			Query:          cgSearchStr,
			State:          cgStateStr,
			RecreationArea: cgRecArea,
			Campgrounds:    cgCampground,
			Campsites:      cgCampsite,
		}

		var provider interface {
			FindCampgrounds(context.Context, core.SearchRequest) ([]core.CampgroundFacility, error)
		}

		switch providerStr {
		case "RecreationDotGov":
			provider = recdotgov.NewProvider()
		case "ReserveCalifornia":
			provider = usedirect.NewProvider("ReserveCalifornia", "https://california-rdr.prod.cali.rd12.recreation-management.tylerapp.com")
		default:
			return fmt.Errorf("unsupported or missing provider: %s", providerStr)
		}

		fmt.Printf("🏕  Searching %s for Campgrounds...\n", providerStr)

		ctx := context.Background()
		facilities, err := provider.FindCampgrounds(ctx, req)
		if err != nil {
			return fmt.Errorf("error fetching campgrounds: %w", err)
		}

		printCampgroundsTable(facilities)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(campgroundsCmd)

	campgroundsCmd.Flags().StringVar(&providerStr, "provider", "RecreationDotGov", "Camping Search Provider")
	campgroundsCmd.Flags().StringVar(&cgSearchStr, "search", "", "Search string")
	campgroundsCmd.Flags().StringVar(&cgStateStr, "state", "", "State abbreviation")
	campgroundsCmd.Flags().StringVar(&cgRecArea, "rec-area", "", "Recreation Area ID")
	campgroundsCmd.Flags().StringSliceVar(&cgCampground, "campground", []string{}, "Campground ID(s)")
	campgroundsCmd.Flags().StringSliceVar(&cgCampsite, "campsite", []string{}, "Campsite ID(s)")
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
