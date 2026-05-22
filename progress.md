# Camply Go Rewrite - UseDirect Provider Progress

## Current State

- **Task**: Fix missing `Campsite Occupancy` in the Go rewrite for the `usedirect` (ReserveCalifornia) provider.
- **Root Cause**: The `/rdr/search/grid` endpoint does not provide unit occupancy. The frontend fetches this lazily when clicking "Show More" on a specific unit.
- **Solution Implemented**:
    - Identified the exact endpoint: `GET /rdr/search/details/<UnitId>/startdate/<date>/nights/1/0/0`
    - Added a `unitOccupancy` cache map to the `Provider` struct in `internal/providers/usedirect/usedirect.go`.
    - Added a `fetchUnitOccupancy` method to fetch and parse the `NightlyUnit.MaxOccupancy` and `DayUseUnit.MaxOccupancy` fields lazily.
    - Updated `FindCampsites` to call `fetchUnitOccupancy` and assign the correct `MinOccupancy` and `MaxOccupancy` to the `core.AvailableCampsite` struct.
    - Created mock test data in `internal/providers/usedirect/testdata/details.json`.
    - Updated `usedirect_test.go` to intercept `/search/details` and assert the new occupancy values (2-6).
    - All tests pass (`go test ./internal/providers/usedirect/...`).

## Next Steps

- **Performance Optimization**: Since `fetchUnitOccupancy` is called per-unit sequentially inside `FindCampsites`, it might cause N+1 request delays. We should investigate if we can run these fetches concurrently using Go routines, or if there is a bulk endpoint available.
- **Review Other Providers**: Check if `ReserveCalifornia` or other platforms need similar lazy-loading logic for amenities or rules.
- **Clean up**: Remove hardcoded fallback logic if we prefer strict failures, or keep it as-is for resilience.
