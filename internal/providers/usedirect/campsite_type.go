package usedirect

// KnownUnitCategories lists the UnitCategoryName values ReserveCalifornia's
// /rdr/search/filters endpoint returns. These become CampsiteType on results.
//
// camply resolves the live response at search time, so this list is only for
// suggestions and help text; it is not a gate.
var KnownUnitCategories = []string{
	"Camping",
	"Hook Up Camping",
	"Primitive Camping",
	"Remote Camping",
	"Group Camping",
	"Equestrian",
	"Lodging",
	"DailyUse",
}
