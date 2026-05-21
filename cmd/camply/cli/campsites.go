package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/providers/recdotgov"
)

var (
	providerStr  string
	campgrounds  []string
	startDates   []string
	endDates     []string
	nights       int
	weekendsOnly bool
)

var campsitesCmd = &cobra.Command{
	Use:   "campsites",
	Short: "Find available campsites",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Parse Dates
		parsedStarts, err := parseDates(startDates)
		if err != nil {
			return fmt.Errorf("invalid start date: %w", err)
		}
		parsedEnds, err := parseDates(endDates)
		if err != nil {
			return fmt.Errorf("invalid end date: %w", err)
		}

		if len(parsedStarts) != len(parsedEnds) {
			return fmt.Errorf("number of start dates must match number of end dates")
		}

		// 2. Build Request
		req := core.SearchRequest{
			StartDates:   parsedStarts,
			EndDates:     parsedEnds,
			Nights:       nights,
			WeekendsOnly: weekendsOnly,
			Campgrounds:  campgrounds,
		}

		// 3. Select Provider
		var provider interface {
			FindCampsites(context.Context, core.SearchRequest) ([]core.AvailableCampsite, error)
		}

		switch providerStr {
		case "RecreationDotGov":
			provider = recdotgov.NewProvider()
		default:
			return fmt.Errorf("unsupported or missing provider in Go rewrite: %s", providerStr)
		}

		fmt.Printf("🏕  Searching %s for %d campground(s)...\n", providerStr, len(campgrounds))

		// 4. Fetch Raw Campsites
		ctx := context.Background()
		rawCampsites, err := provider.FindCampsites(ctx, req)
		if err != nil {
			return fmt.Errorf("error fetching campsites: %w", err)
		}

		// 5. Apply Fast Native Filter (Consolidates consecutive nights & filters bounds)
		filter := core.Filter{}
		filteredCampsites := filter.Apply(rawCampsites, req)

		// 6. Print Results
		printTable(filteredCampsites)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(campsitesCmd)

	campsitesCmd.Flags().StringVar(&providerStr, "provider", "RecreationDotGov", "Camping Search Provider")
	campsitesCmd.Flags().StringSliceVar(&campgrounds, "campground", []string{}, "Campground ID(s)")
	campsitesCmd.Flags().StringSliceVar(&startDates, "start-date", []string{}, "Start of Search window (YYYY-MM-DD)")
	campsitesCmd.Flags().StringSliceVar(&endDates, "end-date", []string{}, "End of Search window (YYYY-MM-DD)")
	campsitesCmd.Flags().IntVar(&nights, "nights", 1, "Minimum number of consecutive nights")
	campsitesCmd.Flags().BoolVar(&weekendsOnly, "weekends", false, "Only search for weekend availabilities")
}

func parseDates(dates []string) ([]time.Time, error) {
	var parsed []time.Time
	for _, d := range dates {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, t)
	}
	return parsed, nil
}

func printTable(campsites []core.AvailableCampsite) {
	if len(campsites) == 0 {
		fmt.Println("❌ No available campsites found matching your criteria.")
		return
	}

	for _, c := range campsites {
		fmt.Printf("🏕  %s - (#%s)\n", c.FacilityName, c.FacilityID)
		fmt.Printf("    ⛺️ %s - (#%s)\n", c.CampsiteSiteName, c.CampsiteID)
		fmt.Printf("        🔗 %s (%d nights)\n", c.BookingURL, c.BookingNights)
	}

	fmt.Printf("\n✅ Found %d matching campsite(s)!\n", len(campsites))
}
