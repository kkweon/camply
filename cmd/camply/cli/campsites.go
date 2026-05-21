package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/config"
	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
	notifications_pkg "github.com/kkweon/camply/internal/notifications"
	"github.com/kkweon/camply/internal/providers/recdotgov"
	"github.com/kkweon/camply/internal/providers/usedirect"
)

var (
	providerStr   string
	campgrounds   []string
	recAreas      []string
	campsites     []string
	startDates    []string
	endDates      []string
	equipment     []string
	notifications []string
	nights        int
	weekendsOnly  bool
)

var campsitesCmd = &cobra.Command{
	Use:   "campsites",
	Short: "Find available campsites",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.Camply("camply, the campsite finder ⛺️")
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

		// 2. Parse Equipment
		parsedEquipment, err := parseEquipment(equipment)
		if err != nil {
			return fmt.Errorf("invalid equipment format: %w", err)
		}

		// Load global configurations for notifications
		appConfig, _ := config.Load()

		// 3. Select Provider
		var provider interface {
			FindCampsites(context.Context, core.SearchRequest) ([]core.AvailableCampsite, error)
			FindCampgrounds(context.Context, core.SearchRequest) ([]core.CampgroundFacility, error)
		}

		switch providerStr {
		case "RecreationDotGov":
			provider = recdotgov.NewProvider()
		case "ReserveCalifornia":
			provider = usedirect.NewProvider("ReserveCalifornia", "https://california-rdr.prod.cali.rd12.recreation-management.tylerapp.com")
		default:
			return fmt.Errorf("unsupported or missing provider in Go rewrite: %s", providerStr)
		}

		ctx := context.Background()

		// 4. Resolve Recreation Areas to Campgrounds
		if len(recAreas) > 0 {
			logger.Info("Resolving %d recreation area(s) to campgrounds...", len(recAreas))
			for _, recArea := range recAreas {
				req := core.SearchRequest{RecreationArea: recArea}
				facilities, err := provider.FindCampgrounds(ctx, req)
				if err != nil {
					return fmt.Errorf("failed to fetch campgrounds for rec-area %s: %w", recArea, err)
				}
				for _, f := range facilities {
					campgrounds = append(campgrounds, f.FacilityID)
				}
			}
		}

		if len(campgrounds) == 0 {
			return fmt.Errorf("no campgrounds specified. Please provide --campground or --rec-area")
		}

		// 5. Build Request
		req := core.SearchRequest{
			StartDates:   parsedStarts,
			EndDates:     parsedEnds,
			Nights:       nights,
			WeekendsOnly: weekendsOnly,
			Campgrounds:  campgrounds,
			Campsites:    campsites,
			Equipment:    parsedEquipment,
		}

		logger.Info("Searching across %d campgrounds", len(campgrounds))

		// 6. Fetch Raw Campsites
		rawCampsites, err := provider.FindCampsites(ctx, req)
		if err != nil {
			return fmt.Errorf("error fetching campsites: %w", err)
		}

		// 7. Apply Fast Native Filter (Consolidates consecutive nights & filters bounds, weekends, equipment, campsites)
		filter := core.Filter{}
		filteredCampsites := filter.Apply(rawCampsites, req)

		// 8. Print Results
		printTable(filteredCampsites)

		// 9. Send Notifications
		if len(filteredCampsites) > 0 && len(notifications) > 0 {
			logger.Info("Dispatching notifications via %v...", notifications)
			notifiers, err := notifications_pkg.SetupNotifiers(notifications, appConfig)
			if err != nil {
				logger.Error("Error setting up notifications: %v", err)
			} else {
				for _, n := range notifiers {
					if err := n.SendCampsites(filteredCampsites); err != nil {
						logger.Error("Failed to send notification: %v", err)
					}
				}
			}
		}

		logger.Camply("Exiting camply 👋")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(campsitesCmd)

	campsitesCmd.Flags().StringVar(&providerStr, "provider", "RecreationDotGov", "Camping Search Provider")
	campsitesCmd.Flags().StringSliceVar(&campgrounds, "campground", []string{}, "Campground ID(s)")
	campsitesCmd.Flags().StringSliceVar(&recAreas, "rec-area", []string{}, "Recreation Area ID(s)")
	campsitesCmd.Flags().StringSliceVar(&campsites, "campsite", []string{}, "Campsite ID(s)")
	campsitesCmd.Flags().StringSliceVar(&startDates, "start-date", []string{}, "Start of Search window (YYYY-MM-DD)")
	campsitesCmd.Flags().StringSliceVar(&endDates, "end-date", []string{}, "End of Search window (YYYY-MM-DD)")
	campsitesCmd.Flags().StringSliceVar(&equipment, "equipment", []string{}, "Equipment filter in format 'Name,Length' (e.g. 'RV,25' or 'Tent')")
	campsitesCmd.Flags().StringSliceVar(&notifications, "notifications", []string{}, "Notification providers to use")
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

func parseEquipment(eqs []string) ([]core.Equipment, error) {
	var parsed []core.Equipment
	for _, e := range eqs {
		parts := strings.Split(e, ",")
		name := strings.TrimSpace(parts[0])
		maxLength := 0
		if len(parts) > 1 {
			lengthStr := strings.TrimSpace(parts[1])
			l, err := strconv.Atoi(lengthStr)
			if err != nil {
				return nil, fmt.Errorf("invalid length '%s' for equipment '%s'", lengthStr, name)
			}
			maxLength = l
		}
		parsed = append(parsed, core.Equipment{
			EquipmentName: name,
			MaxLength:     maxLength,
		})
	}
	return parsed, nil
}

func printTable(campsites []core.AvailableCampsite) {
	if len(campsites) == 0 {
		logger.Info("0 New Campsites Found.")
		return
	}

	logger.Info("Found %d Campsites", len(campsites))

	// Group by booking date
	groupedByDate := make(map[string][]core.AvailableCampsite)
	for _, c := range campsites {
		dateStr := c.BookingDate.Format("Mon, January 02")
		groupedByDate[dateStr] = append(groupedByDate[dateStr], c)
	}

	// Sort dates for printing
	var dates []string
	for k := range groupedByDate {
		dates = append(dates, k)
	}
	sort.Strings(dates)

	for _, dateStr := range dates {
		sitesForDate := groupedByDate[dateStr]
		logger.Info("📅 %s 🏕  %d sites", dateStr, len(sitesForDate))

		// Group by Location (Rec Area + Facility)
		groupedByLocation := make(map[string][]core.AvailableCampsite)
		for _, c := range sitesForDate {
			locStr := fmt.Sprintf("%s  🏕  %s", c.RecreationArea, c.FacilityName)
			groupedByLocation[locStr] = append(groupedByLocation[locStr], c)
		}

		for locStr, sitesForLoc := range groupedByLocation {
			logger.Info("\t⛰️  %s: ⛺ %d sites", locStr, len(sitesForLoc))
			for _, c := range sitesForLoc {
				nightsStr := "night"
				if c.BookingNights > 1 {
					nightsStr = "nights"
				}
				logger.Info("\t\t🔗 %s (%d %s)", c.BookingURL, c.BookingNights, nightsStr)
			}
		}
	}
}
