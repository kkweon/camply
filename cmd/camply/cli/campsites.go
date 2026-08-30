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
	"github.com/kkweon/camply/internal/providers/recdotgov"
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
	shelter        string
	parking        []string
	hookups        []string
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
	f.StringVar(&r.shelter, flagShelter, "",
		"What you are camping in: "+joinKeys(shelterValues)+". One value — it is a choice, "+
			"not a list, and it also decides what --hookups water means (one value)")
	f.StringSliceVar(&r.parking, flagParking, nil,
		multiHelp("How close a car gets: "+joinKeys(parkingValues)+
			". Sites the provider reports no parking for are kept and flagged ⚠️",
			"parking at-site,walk", "parking at-site", "parking walk"))
	f.StringSliceVar(&r.hookups, flagHookups, nil,
		multiHelp("Utilities the site must have: "+joinKeys(hookupValues)+
			". Additive — naming two requires both",
			"hookups electric,water", "hookups electric", "hookups water"))
	// Superseded by --parking. Kept working because the CronJobs run
	// ghcr.io/kkweon/camply:latest with imagePullPolicy Always, so a release
	// reaches them before anyone can edit a manifest.
	f.BoolVar(&r.excludeNoVeh, flagExcludeNoVehicleAccess, false,
		"Deprecated: use --"+flagParking+" at-site. Drops sites that need a walk from the car")
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
		shelter, err := parseShelter(r.shelter)
		if err != nil {
			return err
		}
		parking, err := parseParking(r.parking)
		if err != nil {
			return err
		}
		hookups, err := parseHookups(r.hookups)
		if err != nil {
			return err
		}
		// Behaviour-preserving: the old flag dropped both walk-in and
		// no-access sites and kept unknowns, which is exactly --parking at-site.
		// Mapping it to at-site,walk would silently start including Kaspian and
		// Lodgepole's 54 walk-in sites, and that is the user's call to make,
		// not an upgrade's.
		if r.excludeNoVeh {
			logger.Warn("--%s is deprecated; --%s at-site does the same thing.",
				flagExcludeNoVehicleAccess, flagParking)
			if len(parking) == 0 {
				parking = []core.Parking{core.ParkingAtSite}
			}
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

			MinVehicleLength: r.minVehicleLen,
			Shelter:          shelter,
			Parking:          parking,
			Hookups:          hookups,
		}

		logger.Info("Searching across %d campgrounds", len(r.campgrounds))

		// 6. Fetch Raw Campsites
		rawCampsites, err := provider.FindCampsites(ctx, req)
		if err != nil {
			return fmt.Errorf("error fetching campsites: %w", err)
		}

		// 6a. Anything the adapter could not understand, by value. This is the
		// difference between "the provider said nothing" (normal) and "the
		// provider said something new" (a bug report), which are otherwise the
		// same Unknown.
		for _, line := range recdotgov.TakeDrift() {
			logger.Warn("%s", line)
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
		if err := reportParkingCoverage(
			core.AnalyzeParking(rawCampsites), parking, r.allowPartial,
		); err != nil {
			return err
		}

		reportHookupCoverage(core.AnalyzeHookups(rawCampsites, hookups, shelter), hookups)

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

func printTable(bookings []core.Availability) {
	if len(bookings) == 0 {
		logger.Info("0 New Campsites Found.")
		return
	}

	logger.Info("Found %d Campsites", len(bookings))

	// Group by booking date
	groupedByDate := make(map[string][]core.Availability)
	for _, b := range bookings {
		dateStr := b.Start.Format("Mon, January 02")
		groupedByDate[dateStr] = append(groupedByDate[dateStr], b)
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
		groupedByLocation := make(map[string][]core.Availability)
		for _, b := range sitesForDate {
			locStr := fmt.Sprintf("⛰️  %s  🏕  %s", b.Site.Facility.RecreationArea, b.Site.Facility.Name)
			groupedByLocation[locStr] = append(groupedByLocation[locStr], b)
		}

		locations := make([]string, 0, len(groupedByLocation))
		for loc := range groupedByLocation {
			locations = append(locations, loc)
		}
		sort.Strings(locations)

		for _, locStr := range locations {
			sitesForLoc := groupedByLocation[locStr]
			logger.Info("\t%s: ⛺ %d sites", locStr, len(sitesForLoc))
			for _, b := range sitesForLoc {
				nightsStr := "night"
				if b.Nights > 1 {
					nightsStr = "nights"
				}
				// Same markers the notification title carries, so the
				// terminal and the phone never tell different stories.
				if prefix := b.WarningPrefix(); prefix != "" {
					logger.Info("\t\t🔗 %s (%d %s) %s", b.Site.BookingURL, b.Nights, nightsStr, prefix)
				} else {
					logger.Info("\t\t🔗 %s (%d %s)", b.Site.BookingURL, b.Nights, nightsStr)
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
