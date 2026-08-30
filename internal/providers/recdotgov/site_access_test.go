package recdotgov

import (
	"encoding/json"
	"testing"

	"github.com/kkweon/camply/internal/core"
)

func intPtr(n int) *int { return &n }

// TestClassifySiteAccess pins the classifier to the live responses it was
// derived from. Every case is a real campsite, named by ID, because the rule's
// whole value is that it survives contact with the actual API rather than a
// tidy model of it.
func TestClassifySiteAccess(t *testing.T) {
	tests := []struct {
		name         string
		siteAccess   string
		maxVehicles  *int
		campsiteType string
		want         core.SiteAccess
	}{
		{
			// The incident. Typed identically to Zephyr Cove's drive-in tent
			// sites, half a mile from the road.
			name:         "zephyr 10300345 hike-in typed TENT ONLY NONELECTRIC",
			siteAccess:   "Hike-In",
			maxVehicles:  intPtr(0),
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.SiteAccessHikeIn,
		},
		{
			name:         "zephyr 10300323 hike-in with no vehicle count",
			siteAccess:   "Hike-In",
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.SiteAccessHikeIn,
		},
		{
			// Site Access must outrank the vehicle count: the 1 is trailhead
			// parking. Ranking them the other way misclassifies all 54 of
			// Lodgepole's hike-in and walk-in sites as drive-in.
			name:         "lodgepole 1418 hike-in reporting one vehicle",
			siteAccess:   "Hike-In",
			maxVehicles:  intPtr(1),
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.SiteAccessHikeIn,
		},
		{
			name:         "lodgepole 2455 walk-in typed TENT ONLY NONELECTRIC",
			siteAccess:   "Walk-In",
			maxVehicles:  intPtr(1),
			campsiteType: "TENT ONLY NONELECTRIC",
			want:         core.SiteAccessWalkIn,
		},
		{
			// Site Access also rescues the reverse error: these are drivable
			// despite a WALK TO type, and campsite_type alone hides them.
			name:         "lodgepole drive-in typed WALK TO",
			siteAccess:   "Drive-In",
			maxVehicles:  intPtr(1),
			campsiteType: "WALK TO",
			want:         core.SiteAccessDriveIn,
		},
		{
			// No Site Access at all; only the type gives it away. Its
			// "Max Vehicle Length: 0" is deliberately not consulted.
			name:         "kaspian 86883 WALK TO with no site access attribute",
			maxVehicles:  intPtr(1),
			campsiteType: "WALK TO",
			want:         core.SiteAccessWalkIn,
		},
		{
			// Also reports Max Vehicle Length 0 on 27 sites. Reading that
			// attribute would delete these.
			name:         "manzanita drive-in inferred from count plus type",
			maxVehicles:  intPtr(2),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.SiteAccessDriveIn,
		},
		{
			name:         "nevada beach drive-in inferred from count plus type",
			maxVehicles:  intPtr(1),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.SiteAccessDriveIn,
		},
		{
			// Fallen Leaf 64413: neither attribute present. Kept, not guessed.
			name:         "fallen leaf 64413 no access data at all",
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.SiteAccessUnknown,
		},
		{
			name:         "zero vehicles is conclusive without a site access label",
			maxVehicles:  intPtr(0),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.SiteAccessNoVehicle,
		},
		{
			// Meeks Bay returns "" rather than omitting the attribute.
			name:         "empty site access string is absence not a label",
			siteAccess:   "  ",
			maxVehicles:  intPtr(2),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.SiteAccessDriveIn,
		},
		{
			// The vocabulary is open. An unrecognised label still means "not
			// Drive-In" and must not decay into Unknown, which the filter keeps.
			name:         "unknown site access label is honoured as non-drivable",
			siteAccess:   "Ferry-In",
			maxVehicles:  intPtr(1),
			campsiteType: "STANDARD NONELECTRIC",
			want:         core.SiteAccessNoVehicle,
		},
		{
			name:         "a vehicle count alone proves nothing",
			maxVehicles:  intPtr(2),
			campsiteType: "SOMETHING RECREATION.GOV INVENTED",
			want:         core.SiteAccessUnknown,
		},
		{
			name:         "boat in type",
			campsiteType: "BOAT IN",
			maxVehicles:  intPtr(1),
			want:         core.SiteAccessBoatIn,
		},
		{
			name:         "group hike to type",
			campsiteType: "GROUP HIKE TO",
			maxVehicles:  intPtr(1),
			want:         core.SiteAccessHikeIn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySiteAccess(tt.siteAccess, tt.maxVehicles, tt.campsiteType)
			if got != tt.want {
				t.Errorf("classifySiteAccess(%q, %v, %q) = %v, want %v",
					tt.siteAccess, tt.maxVehicles, tt.campsiteType, got, tt.want)
			}
		})
	}
}

// TestAccessAttributes parses the real attribute shape, and pins the one
// attribute that must be ignored.
func TestAccessAttributes(t *testing.T) {
	// Trimmed from the live api/search/campsites response for Kaspian 86883,
	// which reports Max Vehicle Length 0 while being parked at by one car.
	const payload = `{
	  "campsite_id": "86883",
	  "attributes": [
	    {"attribute_name": "Max Vehicle Length", "attribute_value": "0"},
	    {"attribute_name": "Max Num of Vehicles", "attribute_value": "1"},
	    {"attribute_name": "Shade", "attribute_value": "Full"}
	  ]
	}`

	var item campsiteSearchItem
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	siteAccess, maxVehicles := item.accessAttributes()
	if siteAccess != "" {
		t.Errorf("siteAccess = %q, want empty", siteAccess)
	}
	if maxVehicles == nil || *maxVehicles != 1 {
		t.Fatalf("maxVehicles = %v, want 1 (Max Vehicle Length must not be read)", maxVehicles)
	}
}

func TestAccessAttributesAbsentIsNilNotZero(t *testing.T) {
	var item campsiteSearchItem
	if err := json.Unmarshal([]byte(`{"campsite_id":"64413","attributes":[]}`), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// nil, not 0: a missing count must not be readable as "parks no vehicles".
	if _, maxVehicles := item.accessAttributes(); maxVehicles != nil {
		t.Errorf("maxVehicles = %v, want nil", maxVehicles)
	}
}

// TestCampsiteTypePartition keeps the fallback sets exhaustive, so a campsite
// type added to KnownCampsiteTypes cannot be silently left unconsidered.
func TestCampsiteTypePartition(t *testing.T) {
	for _, name := range KnownCampsiteTypes {
		var found int
		if _, ok := noVehicleCampsiteTypes[name]; ok {
			found++
		}
		if driveInCampsiteTypes[name] {
			found++
		}
		if campsiteTypeUnclassified[name] {
			found++
		}
		if found != 1 {
			t.Errorf("campsite type %q appears in %d of the three access sets, want exactly 1", name, found)
		}
	}
}
