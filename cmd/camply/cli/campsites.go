package cli

import (
	"context"
	"fmt"
	"sort"

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

	// desc is set when the command already belongs to a provider
	// (camply recdotgov campsites). It is nil on the deprecated top-level
	// command, which resolves the provider from --provider instead.
	desc *providers.Descriptor

	provider       string
	campgrounds    []string
	recAreas       []string
	campsites      []string
	dateRanges     []string
	startDate      singleValue
	endDate        singleValue
	equipmentTypes []string
	campsiteTypes  []string
	minVehicleLen  int
	maxEquipLength int
	allowPartial   bool
	excludeNoVeh   bool
	notifications  []string
	nights         int
	weekendsOnly   bool

	// renamedFlags records the deprecated singular spellings the user typed.
	renamedFlags map[string]string
}

// newProviderCampsitesCmd builds `camply <provider> campsites`, bound to one
// provider so a flag that means nothing for it is never offered.
func newProviderCampsitesCmd(d providers.Descriptor, registry []providers.Descriptor) *cobra.Command {
	// The registry travels with the command even though the provider is fixed:
	// an error still needs every provider's vocabulary to say where a rejected
	// value does belong.
	r := &campsitesRunner{desc: &d, registry: registry}

	cmd := &cobra.Command{
		Use:   "campsites",
		Short: "Find available campsites on " + d.DisplayName,
		RunE:  r.run,
	}

	addSearchFlags(cmd, r, &d)

	return cmd
}

// addSearchFlags registers the campsite-search flags on cmd. A nil descriptor
// means the deprecated top-level command, which must accept the union of every
// provider's flags plus --provider.
func addSearchFlags(cmd *cobra.Command, r *campsitesRunner, d *providers.Descriptor) {
	f := cmd.Flags()
	f.SetNormalizeFunc(aliasNormalizer(&r.renamedFlags))

	r.startDate = singleValue{name: "start-date", hint: multiWindowHint}
	r.endDate = singleValue{name: "end-date", hint: multiWindowHint}

	if d == nil {
		f.StringVar(&r.provider, "provider", "RecreationDotGov", "Camping Search Provider")
	}
	f.StringSliceVar(&r.campgrounds, "campgrounds", []string{},
		multiHelp("Campground IDs", "campgrounds 232461,234039", "campgrounds 232461", "campgrounds 234039"))
	f.StringSliceVar(&r.recAreas, "rec-areas", []string{},
		multiHelp(recAreaDesc(d), "rec-areas 2991,1074", "rec-areas 2991", "rec-areas 1074"))
	f.StringSliceVar(&r.campsites, "campsites", []string{},
		multiHelp("Campsite IDs", "campsites 2433,2437", "campsites 2433", "campsites 2437"))
	f.StringSliceVar(&r.dateRanges, "date-ranges", []string{},
		multiHelp("Search windows as START:END", "date-ranges 2026-09-04:2026-09-07",
			"date-ranges 2026-09-04:2026-09-07", "date-ranges 2026-10-01:2026-10-05"))
	f.Var(&r.startDate, "start-date", "Start of a single search window, YYYY-MM-DD (one value)")
	f.Var(&r.endDate, "end-date", "End of a single search window, YYYY-MM-DD (one value)")
	f.StringSliceVar(&r.equipmentTypes, "equipment-types", []string{},
		multiHelp(equipmentHelp(d), "equipment-types Tent,'Small Tent'",
			"equipment-types Tent", "equipment-types 'Small Tent'"))
	f.StringSliceVar(&r.campsiteTypes, "campsite-types", []string{},
		multiHelp(campsiteTypeHelp(d), "campsite-types 'STANDARD NONELECTRIC'",
			"campsite-types 'STANDARD NONELECTRIC'", "campsite-types 'TENT ONLY NONELECTRIC'"))
	f.IntVar(&r.maxEquipLength, "max-equipment-length", 0,
		"Only sites that fit equipment this long, in feet (one value)")
	// Only where the provider reports a vehicle length per site.
	if d == nil || d.SupportsVehicleLength {
		f.IntVar(&r.minVehicleLen, "min-vehicle-length", 0,
			"Only sites that fit a vehicle at least this long, in feet (one value)")
	}
	// Named for what it does, not for what remains. "--vehicle-access=drive-in"
	// would read as a promise that every result is drive-in, but sites the
	// provider reports nothing about are deliberately kept, and a flag whose
	// name oversells its guarantee is how the incident happened in the first
	// place.
	f.BoolVar(&r.excludeNoVeh, flagExcludeNoVehicleAccess, false,
		"Drop sites proven unreachable by car (walk-in, hike-in, boat-in). "+
			"Sites with no access data are not excluded; results always flag them ⚠️")
	f.BoolVar(&r.allowPartial, "allow-partial-match", false,
		"Continue when an equipment filter matches nothing at some campgrounds, instead of failing")
	f.StringSliceVar(&r.notifications, "notifications", []string{},
		multiHelp("Notification providers", "notifications pushover,telegram",
			"notifications pushover", "notifications telegram"))
	f.IntVar(&r.nights, "nights", 1, "Minimum number of consecutive nights (one value)")
	f.BoolVar(&r.weekendsOnly, "weekends", false, "Only search for weekend availabilities")
}

func (r *campsitesRunner) run(cmd *cobra.Command, _ []string) error {
	{
		logger.Camply("camply, the campsite finder ⛺️")

		for _, w := range renamedFlagWarnings(r.renamedFlags) {
			logger.Warn("%s", w)
		}

		// 1. Parse Dates
		windows, err := parseDateWindows(r.dateRanges, r.startDate.value, r.endDate.value)
		if err != nil {
			return err
		}
		parsedStarts, parsedEnds := splitWindows(windows)

		// 2. Equipment
		provider, desc, err := r.resolveProvider()
		if err != nil {
			return err
		}
		if err := validateEquipmentLength(r.equipmentTypes, r.maxEquipLength); err != nil {
			return err
		}
		reg := r.crossProviderRegistry()
		if err := validateVocabularyValues(providers.FlagEquipmentTypes, r.equipmentTypes, desc, reg); err != nil {
			return err
		}
		if err := validateVocabularyValues(providers.FlagCampsiteTypes, r.campsiteTypes, desc, reg); err != nil {
			return err
		}
		parsedEquipment := buildEquipmentFilter(r.equipmentTypes, r.maxEquipLength)
		if d := describeEquipmentFilter(parsedEquipment); d != "" {
			logger.Info("Equipment filter: %s", d)
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
			StartDates:    parsedStarts,
			EndDates:      parsedEnds,
			Nights:        r.nights,
			WeekendsOnly:  r.weekendsOnly,
			Campgrounds:   r.campgrounds,
			Campsites:     r.campsites,
			CampsiteTypes: r.campsiteTypes,
			Equipment:     parsedEquipment,

			MinVehicleLength:       r.minVehicleLen,
			ExcludeNoVehicleAccess: r.excludeNoVeh,
		}

		logger.Info("Searching across %d campgrounds", len(r.campgrounds))

		// 6. Fetch Raw Campsites
		rawCampsites, err := provider.FindCampsites(ctx, req)
		if err != nil {
			return fmt.Errorf("error fetching campsites: %w", err)
		}

		// 6b. Measure the equipment filter before it silently removes anything.
		if err := reportEquipmentCoverage(
			core.AnalyzeEquipment(rawCampsites, parsedEquipment),
			parsedEquipment, desc, r.crossProviderRegistry(), r.allowPartial,
		); err != nil {
			return err
		}

		// 6c. Measure vehicle access before the filter acts on it, for the same
		// reason as the equipment coverage above: a filter that removes sites
		// without saying so is indistinguishable from a campground being full.
		if err := reportSiteAccessCoverage(
			core.AnalyzeSiteAccess(rawCampsites), r.excludeNoVeh, r.allowPartial,
		); err != nil {
			return err
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

		locations := make([]string, 0, len(groupedByLocation))
		for loc := range groupedByLocation {
			locations = append(locations, loc)
		}
		sort.Strings(locations)

		for _, locStr := range locations {
			sitesForLoc := groupedByLocation[locStr]
			logger.Info("\t%s: ⛺ %d sites", locStr, len(sitesForLoc))
			for _, c := range sitesForLoc {
				nightsStr := "night"
				if c.BookingNights > 1 {
					nightsStr = "nights"
				}
				// Same markers the notification title carries, so the
				// terminal and the phone never tell different stories.
				if prefix := c.WarningPrefix(); prefix != "" {
					logger.Info("\t\t🔗 %s (%d %s) %s", c.BookingURL, c.BookingNights, nightsStr, prefix)
				} else {
					logger.Info("\t\t🔗 %s (%d %s)", c.BookingURL, c.BookingNights, nightsStr)
				}
			}
		}
	}
}

// resolveProvider returns the provider this command is bound to, or resolves
// --provider on the deprecated top-level command.
func (r *campsitesRunner) resolveProvider() (providers.Provider, providers.Descriptor, error) {
	if r.desc != nil {
		return r.desc.New(), *r.desc, nil
	}
	return providers.NewFrom(r.registry, r.provider)
}

// crossProviderRegistry is consulted to say where a rejected value does belong.
func (r *campsitesRunner) crossProviderRegistry() []providers.Descriptor {
	if len(r.registry) > 0 {
		return r.registry
	}
	// Only reachable if a command was built without one; the real registry is
	// the correct answer there.
	return providers.Descriptors()
}
