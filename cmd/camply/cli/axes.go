package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kkweon/camply/internal/core"
)

// The three axes as flag values. These vocabularies are camply's own and closed,
// so unlike a provider's they can be checked before a single API call.

const (
	flagShelter                = "shelter"
	flagParking                = "parking"
	flagHookups                = "hookups"
	flagExcludeNoVehicleAccess = "exclude-no-vehicle-access"
)

var shelterValues = map[string]core.Shelter{
	"tent":  core.ShelterTent,
	"rv":    core.ShelterRV,
	"cabin": core.ShelterCabin,
}

var parkingValues = map[string]core.Parking{
	"at-site": core.ParkingAtSite,
	"walk":    core.ParkingWalk,
	"none":    core.ParkingNone,
}

var hookupValues = map[string]core.Hookup{
	"electric": core.HookupElectric,
	"water":    core.HookupWater,
	"sewer":    core.HookupSewer,
}

// parseShelter accepts exactly one value.
//
// It is a choice, not a filter list: a camper decides they are going tent
// camping, or RV camping, or cabin camping, and that decision also settles what
// --hookups water means. Accepting a list would re-merge the choice with the
// site's permitted set, which is the confusion the domain model exists to
// prevent. Someone bringing both an RV and a tent runs the search twice.
func parseShelter(raw string) (core.Shelter, error) {
	if strings.TrimSpace(raw) == "" {
		return core.ShelterUnknown, nil
	}
	if strings.Contains(raw, ",") {
		return core.ShelterUnknown, fmt.Errorf(
			"--%s takes one value, not a list: you are going tent camping, or RV camping, "+
				"or cabin camping. Run the search once per kind.\n  Valid values: %s",
			flagShelter, joinKeys(shelterValues))
	}
	s, ok := shelterValues[core.NormalizeValue(raw)]
	if !ok {
		return core.ShelterUnknown, fmt.Errorf("--%s %q is not a kind of camping.\n  Valid values: %s",
			flagShelter, raw, joinKeys(shelterValues))
	}
	return s, nil
}

func parseParking(raw []string) ([]core.Parking, error) {
	var out []core.Parking
	for _, v := range raw {
		p, ok := parkingValues[core.NormalizeValue(v)]
		if !ok {
			return nil, fmt.Errorf("--%s %q is not a parking level.\n  Valid values: %s",
				flagParking, v, joinKeys(parkingValues))
		}
		out = append(out, p)
	}
	return out, nil
}

func parseHookups(raw []string) ([]core.Hookup, error) {
	var out []core.Hookup
	for _, v := range raw {
		h, ok := hookupValues[core.NormalizeValue(v)]
		if !ok {
			return nil, fmt.Errorf("--%s %q is not a hookup.\n  Valid values: %s",
				flagHookups, v, joinKeys(hookupValues))
		}
		out = append(out, h)
	}
	return out, nil
}

func joinKeys[T any](m map[string]T) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
