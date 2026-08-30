package recdotgov

import (
	"strings"
	"testing"

	"github.com/kkweon/camply/internal/core"
)

func attrs(pairs ...string) attributes {
	a := attributes{}
	for i := 0; i+1 < len(pairs); i += 2 {
		a[pairs[i]] = pairs[i+1]
	}
	return a
}

// TestClassifyParking pins the rule to the live responses it was derived from.
// Every case is a real campsite, named by ID, because the rule's whole value is
// that it survived contact with the actual API rather than a tidy model of it.
func TestClassifyParking(t *testing.T) {
	tests := []struct {
		name         string
		attributes   attributes
		campsiteType string
		notices      string
		want         core.Parking
	}{
		{
			// The incident. Typed identically to Zephyr Cove's drive-in tent
			// sites, half a mile from the road.
			name:         "zephyr 10300345",
			attributes:   attrs(attrSiteAccess, "Hike-In", attrMaxNumVehicles, "0"),
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.ParkingNone,
		},
		{
			name:         "zephyr 10300323, no vehicle count recorded",
			attributes:   attrs(attrSiteAccess, "Hike-In"),
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.ParkingWalk,
		},
		{
			// The precedence rule. That 1 is trailhead parking; ranking the
			// count above Site Access misclassifies all 54 of Lodgepole's
			// hike-in and walk-in sites as reachable by car.
			name:         "lodgepole 1418, hike-in reporting one vehicle",
			attributes:   attrs(attrSiteAccess, "Hike-In", attrMaxNumVehicles, "1", attrDrivewaySurface, "Paved"),
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.ParkingWalk,
		},
		{
			// The reverse leak: drivable despite a WALK TO type.
			name:         "lodgepole 2524, drive-in typed WALK TO",
			attributes:   attrs(attrSiteAccess, "Drive-In", attrMaxNumVehicles, "1"),
			campsiteType: "WALK TO",
			want:         core.ParkingAtSite,
		},
		{
			// No Site Access at all; only the type gives it away. Its
			// "Max Vehicle Length: 0" is deliberately not consulted.
			name:         "kaspian 86883",
			attributes:   attrs(attrMaxNumVehicles, "1", attrMaxVehicleLength, "0", attrDrivewaySurface, "Paved"),
			campsiteType: "WALK TO",
			want:         core.ParkingWalk,
		},
		{
			// Also reports Max Vehicle Length 0 on 27 sites. Reading that as
			// "no vehicle" would delete them.
			name:         "manzanita, driveway with a zero vehicle length",
			attributes:   attrs(attrMaxNumVehicles, "2", attrMaxVehicleLength, "0", attrDrivewaySurface, "Paved"),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.ParkingAtSite,
		},
		{
			name:         "a vehicle length alone still parks a car at the site",
			attributes:   attrs(attrMaxNumVehicles, "1", attrMaxVehicleLength, "18"),
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.ParkingAtSite,
		},
		{
			// Fallen Leaf 64413: neither attribute present. Kept, not guessed.
			name:         "fallen leaf 64413, no access data at all",
			attributes:   attrs(),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.ParkingUnknown,
		},
		{
			// Zephyr records the half-mile hike only in prose.
			name:         "a notice is enough when no attribute says it",
			attributes:   attrs(attrDrivewaySurface, "Paved"),
			campsiteType: "TENT ONLY NONELECTRIC",
			notices:      "NO VEHICLE ACCESS Hike in distance is .5 miles.",
			want:         core.ParkingNone,
		},
		{
			// Meeks Bay returns "" rather than omitting the attribute.
			name:         "an empty site access string is absence, not a label",
			attributes:   attrs(attrSiteAccess, "  ", attrDrivewaySurface, "Paved"),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.ParkingAtSite,
		},
		{
			// The vocabulary is open. An unrecognised label still means "not
			// Drive-In" rather than decaying to Unknown, which a filter keeps.
			name:         "an unrecognised label is honoured as non-drivable",
			attributes:   attrs(attrSiteAccess, "Ferry-In", attrMaxNumVehicles, "1"),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.ParkingWalk,
		},
		{
			name:         "a recorded hike distance means a walk",
			attributes:   attrs("Hike in Distance", "441 feet", attrDrivewaySurface, "Paved"),
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.ParkingWalk,
		},
		{
			name:         "boat in",
			attributes:   attrs(attrMaxNumVehicles, "1"),
			campsiteType: "BOAT IN",
			want:         core.ParkingNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, basis := classifyParking(tt.attributes, tt.campsiteType, tt.notices)
			if got != tt.want {
				t.Errorf("classifyParking() = %v (basis %q), want %v", got, basis, tt.want)
			}
			if got != core.ParkingUnknown && basis == "" {
				t.Error("a decision must record what decided it")
			}
		})
	}
}

// TestClassifyPermits pins that permitted_equipment outranks the type token,
// which is the reverse of how the token reads.
func TestClassifyPermits(t *testing.T) {
	eq := func(names ...string) []core.Equipment {
		var out []core.Equipment
		for _, n := range names {
			out = append(out, core.Equipment{EquipmentName: n})
		}
		return out
	}

	tests := []struct {
		name         string
		equipment    []core.Equipment
		campsiteType string
		want         core.Permitted
	}{
		{
			// 98% of STANDARD sites. "Standard" is not a kind of camping, it is
			// a site that takes either.
			name:      "standard takes a tent and an RV",
			equipment: eq("RV", "Tent", "Trailer"), campsiteType: "STANDARD NONELECTRIC",
			want: core.PermitsTent | core.PermitsRV,
		},
		{
			// 50 of 196 sites named TENT ONLY. The equipment is the authority.
			name:      "a TENT ONLY site that also permits RVs",
			equipment: eq("Car", "Pop up", "Small Tent"), campsiteType: "TENT ONLY NONELECTRIC",
			want: core.PermitsTent | core.PermitsRV,
		},
		{
			name:      "tent only, and the equipment agrees",
			equipment: eq("Small Tent", "Tent"), campsiteType: "TENT ONLY NONELECTRIC",
			want: core.PermitsTent,
		},
		{
			name:      "the backticked spelling is still a tent",
			equipment: eq("Large Tent Over 9X12`"), campsiteType: "WALK TO",
			want: core.PermitsTent,
		},
		{
			// A cabin is not something you bring, so 80% of CABIN sites list no
			// equipment at all. That silence is expected, not missing data.
			name:      "a cabin with no equipment",
			equipment: nil, campsiteType: "CABIN ELECTRIC",
			want: core.PermitsCabin,
		},
		{
			name:      "the type is the fallback when equipment is absent",
			equipment: nil, campsiteType: "STANDARD NONELECTRIC",
			want: core.PermitsTent | core.PermitsRV,
		},
		{
			name:      "nothing known",
			equipment: nil, campsiteType: "MANAGEMENT",
			want: core.PermittedUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := classifyPermits(tt.equipment, tt.campsiteType); got != tt.want {
				t.Errorf("classifyPermits() = %b, want %b", got, tt.want)
			}
		})
	}
}

// TestClassifyHookups pins that the axes are read from separate evidence and
// never derived from one another.
func TestClassifyHookups(t *testing.T) {
	// 61 sites are typed NONELECTRIC and still have a water hookup: the token
	// speaks only to electricity.
	h := classifyHookups(attrs("Water Hookup", "Yes"), "STANDARD NONELECTRIC")
	if h.Electric != core.TriNo {
		t.Errorf("Electric = %v, want TriNo", h.Electric)
	}
	if h.Water != core.TriYes {
		t.Errorf("Water = %v, want TriYes", h.Water)
	}
	if h.Sewer != core.TriUnknown {
		t.Errorf("Sewer = %v, want TriUnknown — absence is not a no", h.Sewer)
	}

	// 23 of 137 ELECTRIC sites carry no electricity attribute, so the token is
	// the better source where both exist.
	if got := classifyHookups(attrs(), "RV ELECTRIC"); got.Electric != core.TriYes {
		t.Errorf("Electric = %v, want TriYes from the token alone", got.Electric)
	}

	// An explicit No is a real answer, unlike an absent attribute.
	if got := classifyHookups(attrs("Water Hookup", "No"), "RV ELECTRIC"); got.Water != core.TriNo {
		t.Errorf("Water = %v, want TriNo", got.Water)
	}
}

// TestAttributeAliasesDoNotOverreach keeps the substring trap shut: matching
// "Water" loosely would turn 127 lakefront sites into water hookups.
func TestAttributeAliasesDoNotOverreach(t *testing.T) {
	a := attrs(
		"Proximity to Water", "Lakefront",
		"Drinking Water", "Drinking Water",
		"Water Spigot", "Water Spigot",
	)
	if got := a.yesNo(waterHookupAttrs); got != core.TriUnknown {
		t.Errorf("a lakefront site with a spigot reports a water hookup: %v", got)
	}
	if got := a.yesNo(sharedWaterAttrs); got != core.TriYes {
		t.Errorf("a spigot should register as shared water, got %v", got)
	}

	// One concept, several spellings.
	for _, spelling := range []string{"Water Hookup", "Water Hookups"} {
		if got := attrs(spelling, "Yes").yesNo(waterHookupAttrs); got != core.TriYes {
			t.Errorf("%q not recognised as a water hookup", spelling)
		}
	}
	// Values are written inconsistently even within one campground.
	for _, raw := range []string{"441 feet", "30 Feet", "20"} {
		if got := attrs("Hike in Distance", raw).firstNumber(hikeDistanceAttrs); got == nil {
			t.Errorf("hike distance %q not parsed", raw)
		}
	}
}

// TestDriftIsReportedByValue is what keeps the adapter from decaying quietly.
//
// A vocabulary that grows on recreation.gov's side turns into Unknown one
// campsite at a time, and Unknown is the safe answer nobody investigates. The
// report has to name the value, because a count says something is wrong while
// the value says what to add.
func TestDriftIsReportedByValue(t *testing.T) {
	TakeDrift() // clear anything an earlier test left behind

	// Recognised values are not drift, however unusual.
	classifyParking(attrs(attrSiteAccess, "Drive-In"), "STANDARD NONELECTRIC", "")
	// Nor is silence: a provider that said nothing has not said anything new.
	classifyParking(attrs(), "STANDARD NONELECTRIC", "")
	if got := TakeDrift(); len(got) != 0 {
		t.Fatalf("recognised values and silence are not drift, got %v", got)
	}

	classifyParking(attrs(attrSiteAccess, "Ferry-In"), "STANDARD NONELECTRIC", "")
	classifyParking(attrs(attrSiteAccess, "Ferry-In"), "STANDARD NONELECTRIC", "")
	classifyParking(attrs(attrSiteAccess, "Tram-In"), "STANDARD NONELECTRIC", "")

	report := TakeDrift()
	if len(report) != 1 {
		t.Fatalf("want one line, got %v", report)
	}
	for _, want := range []string{"Ferry-In", "Tram-In", "Site Access", "3 campsites", "adapter.go"} {
		if !strings.Contains(report[0], want) {
			t.Errorf("report is missing %q:\n%s", want, report[0])
		}
	}

	// Taking it clears it, so a second search does not re-report the first.
	if got := TakeDrift(); len(got) != 0 {
		t.Errorf("drift should be cleared once reported, got %v", got)
	}
}

// A label camply does not know is still honoured as non-drivable rather than
// decaying to Unknown, which a filter keeps.
func TestUnmappedLabelIsStillNonDrivable(t *testing.T) {
	got, _ := classifyParking(attrs(attrSiteAccess, "Ferry-In"), "STANDARD NONELECTRIC", "")
	TakeDrift()
	if got != core.ParkingWalk {
		t.Errorf("classifyParking() = %v, want ParkingWalk", got)
	}
}
