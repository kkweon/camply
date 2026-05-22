# Camply Go Rewrite — Review Findings & TODO

Review of the `recdotgov` and `reservecalifornia` (usedirect) provider migrations, plus
shared `core`. Items are ordered by priority. References use `file:line`.

Recommended order: fix #1 (filter) first (highest impact, self-contained) → add
consolidation test coverage → #2 (`--campsite`) → production hardening (#3, #5, #6).

---

## 🔴 Critical

- [x] **1. `consolidateNights` produces duplicate & over-long records (over-counts everything)** — ✅ fixed
    - `internal/core/filter.go:46-99`
    - Emits a record on _every_ append where `len >= requiredNights`, plus a separate
      single-night record when `requiredNights == 1`. Python emits only contiguous windows
      of _exactly_ `nights` length (`camply/search/base_search.py:730-791`).
    - Verified: 1 campsite available 2 consecutive nights, `--nights 1` → returns **3**
      records (a phantom `06-01..06-03` 2-night block + the two 1-night records) instead of 2.
      For `--nights 2` over a 3-night run, Go yields a phantom 3-night block too.
    - Impact: inflated table counts + duplicate/over-long notifications (flows into
      `cmd/camply/cli/campsites.go:119-137`).
    - Fix: find maximal consecutive runs; for each run emit windows of exactly
      `requiredNights` (`run[i] .. run[i+requiredNights-1]`) with `BookingNights = requiredNights`;
      drop the `requiredNights == 1` special case.
    - Done: rewrote `consolidateNights` to emit sliding windows of exactly `requiredNights`
      (composite `CampsiteID|FacilityID` grouping); added `internal/core/filter_test.go`.
    - Note: both existing tests stayed green — recdotgov asserts on raw `FindCampsites` and
      never calls `Filter.Apply` (`recdotgov_test.go:49-59`); usedirect's `34`
      (`usedirect_test.go:79`) is _insensitive_ to the bug (its 1-night
      `2026-05-22..2026-05-23` window filters out every phantom block either way), not
      "calibrated" to it.

- [ ] **2. `--campsite` flag is silently ignored**
    - `cmd/camply/cli/campsites.go:105` sets `req.Campsites`, but nothing reads it (no
      provider, no filter). Today a `--campsite` search returns the whole campground.
    - PRD.md:25 requires limiting results strictly to those sites. Python resolves
      campsite→facility _and_ post-filters (`camply/.../recdotgov_provider.py:885-891`).
    - Fix: at minimum, drop sites whose `CampsiteID` ∉ `req.Campsites` in `core.Filter`
      when that slice is non-empty.

---

## 🟠 High

- [ ] **3. recdotgov likely WAF/403'd in production + no rate limiting/retry**
    - Static `User-Agent: camply/go-rewrite` (`recdotgov.go:260`, `metadata.go:29`), no
      `Referer` on metadata. Python mimics a browser: rotating Chrome UAs, `STANDARD_HEADERS`,
      `Referer: https://www.recreation.gov/` per request (`recdotgov_provider.py:598-629`).
    - No rate limiting or retry/backoff anywhere; Python uses 3 req/s + exponential backoff +
      1–1.5s sleep between campgrounds. A single 429 currently aborts the run
      (`recdotgov.go:269-271`).

- [ ] **4. usedirect equipment matching is invented and nearly a no-op**
    - Python does no equipment filtering for usedirect — warns and ignores `--equipment`
      (`camply/search/search_usedirect.py:121-124`).
    - Go's heuristic `strings.Contains(lowerUseType, "site")` (`usedirect.go:211`) matches
      almost every real type-group name (`Campsite`, `Tent Site`, `Premium Campsite`,
      `Enroute Site`…) → nearly everything tagged `Tent`, while excluding `Boat In`/`Hike In`/
      `Day Use`. Grid endpoint has no real equipment data.
    - Fix: either drop equipment filtering for this provider (match Python, warn) or document
      it as intentional best-effort — but replace the `"site"` substring with something principled.

- [ ] **5. usedirect occupancy: deviates from Python + redundant double-fetch**
    - Python hardcodes occupancy `(0, 1)`, no per-unit call (`usedirect.py:486`). Go adds a
      `/rdr/search/details/<unitId>` request per available unit (`usedirect.go:293-334`) — N+1
      request volume / WAF risk.
    - Remove redundant synchronous call at `usedirect.go:201` (units already pre-fetched
      concurrently at `usedirect.go:171-180`; cache always hits).
    - Document/justify the hardcoded `startdate/2000-01-01` (`usedirect.go:304`) or it may
      return nothing in prod.

- [ ] **6. usedirect sends the full date range in one grid request**
    - Go sends entire `[start, end]` in a single request (`usedirect.go:113-120`). Python
      chunks per month (`StartDate = max(start, today-1)`, `EndDate = last day of month`,
      one request per month per campground — `usedirect.py:316`, `search_usedirect.py:143-161`).
    - If the grid endpoint caps its response window (the per-month chunking implies it does),
      long searches silently return incomplete availability. Fix: match month-chunking.

---

## 🟡 Medium

- [ ] **7. `defer` inside pagination loops** — `recdotgov.go:136` and `:218` defer
      `resp.Body.Close()` inside `for start < total`; bodies stay open until the function
      returns. Close per-iteration (or use a body-reading helper).

- [ ] **8. `refreshMetadata` swallows errors** — `usedirect.go:337-415` ignores HTTP/decode
      errors for places & facilities (and decode errors for filters), returns `nil` even if all
      failed. A WAF block then yields empty rec-area names and `/park/0/<facility>` booking URLs
      (`usedirect.go:195`) instead of surfacing an error.

- [ ] **9. RIDB pagination has no offset cap or zero-progress guard** —
      `recdotgov.go:117/199`. Python caps at offset 500 (`recdotgov_provider.py:436`). A
      response with `CurrentCount == 0` and `TotalCount > 0` is an infinite loop; broad listings
      make far more requests than Python.

- [ ] **10. recdotgov availability uses an allowlist (`status != "Available"`)** —
      `recdotgov.go:329`. Python uses an inverted blocklist `CAMPSITE_UNAVAILABLE_STRINGS`
      (`recdotgov_camps.py:200-203`), treating unrecognized statuses as available. Rarely bites
      (API returns `"Available"`); add a comment or align.

- [ ] **11. `getSearchMonths` always includes the end-date's month** (`recdotgov.go:293`);
      Python's range is end-exclusive. Harmless to results (window filter drops them) but wastes
      a request per search.

---

## 🟢 Minor / code quality

- [ ] **12. Test data field mismatch** — `metadata_response.json` uses `parent_name` but the
      struct reads `json:"asset_name"` (`metadata.go:77`) → `Unknown Campground` in test output;
      facility-name hydration is effectively untested.
- [ ] **13. Provider construction duplicated** across `campsites.go:68-75`,
      `campgrounds.go:38-45`, `recreation_areas.go:32-39` with a repeated hardcoded URL literal.
      Add a `providers.New(name)` factory.
- [ ] **14. `map[string]interface{}` in notifications** (`notifications.go:84,131`)
      contradicts `GEMINI.md:87`. Use typed payload structs.
- [ ] **15. `core.Filter` is an empty stateless struct** — could be package-level functions.
- [ ] **16. Concurrency:** `metadataFetched` and metadata maps mutated without a lock
      (`usedirect.go:337-415`). Fine for current sequential flow; a footgun if a `Provider` is
      reused across goroutines.

---

## 📋 Scope still to port ("so far")

- [ ] Daemon modes: `--continuous`, `--search-forever`, `--polling-interval`, `--search-once`
- [ ] Offline-search state (`--offline-search`, `--offline-search-path`)
- [ ] `--yaml-config`, `--day`, `--equipment-id`
- [ ] Commands: `equipment-types`, `list-campsites`, `configure`, `tui`
- [ ] Notifications beyond Pushover + Telegram (PRD.md lists 9+; `SetupNotifiers` warns and
      skips the rest — `notifications.go:176-178`)

---

## ✅ Test coverage gaps

- [x] Add a `core.Filter.Apply` / `consolidateNights` test that exercises consolidation —
      ✅ `internal/core/filter_test.go`.
- [x] Verify usedirect `34` assertion (`usedirect_test.go:79`) after fixing #1 — ✅ unchanged
      (insensitive to the bug; see #1).
