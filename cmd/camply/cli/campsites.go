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
	"github.com/kkweon/camply/internal/providers"
)

// campsitesRunner holds one command invocation's flag values. Allocating it in
// the constructor keeps sibling commands from sharing state.
type campsitesRunner struct {
	registry []providers.Descriptor

	provider      string
	campgrounds   []string
	recAreas      []string
	campsites     []string
	startDates    []string
	endDates      []string
	equipment     []string
	notifications []string
	nights        int
	weekendsOnly  bool
}

func newCampsitesCmd(descs []providers.Descriptor) *cobra.Command {
	r := &campsitesRunner{registry: descs}

	cmd := &cobra.Command{
		Use:   "campsites",
		Short: "Find available campsites",
		RunE:  r.run,
	}

	cmd.Flags().StringVar(&r.provider, "provider", "RecreationDotGov", "Camping Search Provider")
	cmd.Flags().StringSliceVar(&r.campgrounds, "campground", []string{}, "Campground ID(s)")
	cmd.Flags().StringSliceVar(&r.recAreas, "rec-area", []string{}, "Recreation Area ID(s)")
	cmd.Flags().StringSliceVar(&r.campsites, "campsite", []string{}, "Campsite ID(s)")
	cmd.Flags().StringSliceVar(&r.startDates, "start-date", []string{}, "Start of Search window (YYYY-MM-DD)")
	cmd.Flags().StringSliceVar(&r.endDates, "end-date", []string{}, "End of Search window (YYYY-MM-DD)")
	cmd.Flags().StringSliceVar(&r.equipment, "equipment", []string{}, "Equipment filter in format 'Name,Length' (e.g. 'RV,25' or 'Tent')")
	cmd.Flags().StringSliceVar(&r.notifications, "notifications", []string{}, "Notification providers to use")
	cmd.Flags().IntVar(&r.nights, "nights", 1, "Minimum number of consecutive nights")
	cmd.Flags().BoolVar(&r.weekendsOnly, "weekends", false, "Only search for weekend availabilities")

	return cmd
}

func (r *campsitesRunner) run(cmd *cobra.Command, _ []string) error {
	{
		logger.Camply("camply, the campsite finder ⛺️")
		// 1. Parse Dates
		parsedStarts, err := parseDates(r.startDates)
		if err != nil {
			return fmt.Errorf("invalid start date: %w", err)
		}
		parsedEnds, err := parseDates(r.endDates)
		if err != nil {
			return fmt.Errorf("invalid end date: %w", err)
		}

		if len(parsedStarts) != len(parsedEnds) {
			return fmt.Errorf("number of start dates must match number of end dates")
		}

		// 2. Parse Equipment
		parsedEquipment, err := parseEquipment(r.equipment)
		if err != nil {
			return fmt.Errorf("invalid equipment format: %w", err)
		}

		// Load global configurations for notifications
		appConfig, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// Set the notifiers up before searching. Doing this lazily meant a
		// broken notification config stayed invisible until the one run that
		// actually had something to report — exactly the run you cannot
		// afford to lose.
		var notifiers []notifications_pkg.Notifier
		if len(r.notifications) > 0 {
			notifiers, err = notifications_pkg.SetupNotifiers(r.notifications, appConfig)
			if err != nil {
				return fmt.Errorf("failed to set up notifications: %w", err)
			}
		}

		// 3. Select Provider
		provider, _, err := providers.NewFrom(r.registry, r.provider)
		if err != nil {
			return err
		}

		ctx := context.Background()

		// 4. Resolve Recreation Areas to Campgrounds
		if len(r.recAreas) > 0 {
			logger.Info("Resolving %d recreation area(s) to campgrounds...", len(r.recAreas))
			for _, recArea := range r.recAreas {
				req := core.SearchRequest{RecreationArea: recArea}
				facilities, err := provider.FindCampgrounds(ctx, req)
				if err != nil {
					return fmt.Errorf("failed to fetch campgrounds for rec-area %s: %w", recArea, err)
				}
				for _, f := range facilities {
					r.campgrounds = append(r.campgrounds, f.FacilityID)
				}
			}
		}

		if len(r.campgrounds) == 0 {
			return fmt.Errorf("no campgrounds specified. Please provide --campground or --rec-area")
		}

		// 5. Build Request
		req := core.SearchRequest{
			StartDates:   parsedStarts,
			EndDates:     parsedEnds,
			Nights:       r.nights,
			WeekendsOnly: r.weekendsOnly,
			Campgrounds:  r.campgrounds,
			Campsites:    r.campsites,
			Equipment:    parsedEquipment,
		}

		logger.Info("Searching across %d campgrounds", len(r.campgrounds))

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
		if len(filteredCampsites) > 0 && len(notifiers) > 0 {
			logger.Info("Dispatching notifications via %v...", r.notifications)
			for _, n := range notifiers {
				if err := n.SendCampsites(filteredCampsites); err != nil {
					return fmt.Errorf("failed to send notification: %w", err)
				}
			}
		}

		logger.Camply("Exiting camply 👋")
		return nil
	}
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
			locStr := fmt.Sprintf("⛰️  %s  🏕  %s", c.RecreationArea, c.FacilityName)
			groupedByLocation[locStr] = append(groupedByLocation[locStr], c)
		}

		for locStr, sitesForLoc := range groupedByLocation {
			logger.Info("\t%s: ⛺ %d sites", locStr, len(sitesForLoc))
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
