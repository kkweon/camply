package cli

import (
	"fmt"
	"strings"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/providers"
)

// describeSearch echoes the request as one line at the top of the run, spelled
// with the flags that produced it. The log is often all that survives of a
// CronJob run, and a log that does not say what was searched cannot be
// reproduced. Equipment is deliberately absent: it already gets its own
// "Equipment filter:" line.
func describeSearch(req core.SearchRequest) string {
	windows := make([]string, 0, len(req.StartDates))
	for i := range req.StartDates {
		windows = append(windows, fmt.Sprintf("%s → %s",
			req.StartDates[i].Format("2006-01-02"), req.EndDates[i].Format("2006-01-02")))
	}

	parts := []string{strings.Join(windows, "; ")}
	parts = append(parts, fmt.Sprintf("--nights %d", req.Nights))
	if req.WeekendsOnly {
		parts = append(parts, "--weekends")
	}
	if len(req.Campsites) > 0 {
		parts = append(parts, "--campsites "+strings.Join(req.Campsites, ","))
	}
	if len(req.CampsiteTypes) > 0 {
		parts = append(parts, "--"+providers.FlagCampsiteTypes+" "+joinQuoted(req.CampsiteTypes))
	}
	if req.MinVehicleLength > 0 {
		parts = append(parts, fmt.Sprintf("--min-vehicle-length %d", req.MinVehicleLength))
	}
	if req.Shelter != core.ShelterUnknown {
		parts = append(parts, "--"+flagShelter+" "+req.Shelter.String())
	}
	if len(req.Parking) > 0 {
		parts = append(parts, "--"+flagParking+" "+joinParking(req.Parking))
	}
	if len(req.Hookups) > 0 {
		hs := make([]string, 0, len(req.Hookups))
		for _, h := range req.Hookups {
			hs = append(hs, h.String())
		}
		parts = append(parts, "--"+flagHookups+" "+strings.Join(hs, ","))
	}
	return strings.Join(parts, ", ")
}

// describeFunnel narrates what each filter stage left standing, in the order
// the stages run. This line is what tells a reader whether an empty result
// means a full campground or a filter that removed everything — the two used
// to print the same nothing.
//
// Stages the user did not request are omitted: they removed nothing and would
// only pad the line.
func describeFunnel(stats core.FilterStats, req core.SearchRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d sites with %d open nights", stats.RawSites, stats.RawNights)
	if req.Nights > 1 {
		fmt.Fprintf(&b, " → %d stays of %d consecutive nights", stats.Stays, req.Nights)
	}
	if len(req.Campsites) > 0 {
		fmt.Fprintf(&b, " → %d at --campsites %s", stats.AfterCampsites, strings.Join(req.Campsites, ","))
	}
	if req.WeekendsOnly {
		fmt.Fprintf(&b, " → %d starting Fri/Sat", stats.AfterWeekends)
	}
	fmt.Fprintf(&b, " → %d inside the requested dates", stats.AfterWindow)
	if len(req.Equipment) > 0 {
		fmt.Fprintf(&b, " → %d after --%s", stats.AfterEquipment, providers.FlagEquipmentTypes)
	}
	if len(req.CampsiteTypes) > 0 {
		fmt.Fprintf(&b, " → %d after --%s", stats.AfterCampsiteTypes, providers.FlagCampsiteTypes)
	}
	if req.Shelter != core.ShelterUnknown {
		fmt.Fprintf(&b, " → %d after --%s %s", stats.AfterShelter, flagShelter, req.Shelter)
	}
	if len(req.Parking) > 0 {
		fmt.Fprintf(&b, " → %d after --%s %s", stats.AfterParking, flagParking, joinParking(req.Parking))
	}
	if len(req.Hookups) > 0 {
		fmt.Fprintf(&b, " → %d after --%s", stats.AfterHookups, flagHookups)
	}
	return b.String()
}
