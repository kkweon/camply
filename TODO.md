# Camply Go Rewrite — Review Findings & TODO

Review of the `recdotgov` and `reservecalifornia` (usedirect) provider migrations, plus
shared `core`. Items are ordered **most-critical-first** within each tier — reorder freely,
there are no numbers to renumber. References use `file:line`.

---

## 🔴 Critical

- [ ] **`--campsite` flag is silently ignored**
    - `cmd/camply/cli/campsites.go:105` sets `req.Campsites`, but nothing reads it (no
      provider, no filter). Today a `--campsite` search returns the whole campground.
    - PRD.md:25 requires limiting results strictly to those sites. Python resolves
      campsite→facility _and_ post-filters (`camply/.../recdotgov_provider.py:885-891`).
    - Fix: at minimum, drop sites whose `CampsiteID` ∉ `req.Campsites` in `core.Filter`
      when that slice is non-empty.

---

## 🟠 High

- [ ] **usedirect sends the full date range in one grid request**
    - Go sends entire `[start, end]` in a single request (`usedirect.go:113-120`). Python
      chunks per month (`StartDate = max(start, today-1)`, `EndDate = last day of month`,
      one request per month per campground — `usedirect.py:316`, `search_usedirect.py:143-161`).
    - If the grid endpoint caps its response window (the per-month chunking implies it does),
      long searches silently return incomplete availability. Fix: match month-chunking.

- [ ] **RIDB pagination has no offset cap or zero-progress guard (infinite-loop/hang risk)** —
      `recdotgov.go:117/199`. Python caps at offset 500 (`recdotgov_provider.py:436`). A
      response with `CurrentCount == 0` and `TotalCount > 0` is an infinite loop (the run never
      terminates); broad listings also make far more requests than Python. Add an offset cap +
      a zero-progress break.

- [ ] **usedirect equipment matching is invented and nearly a no-op**
    - Python does no equipment filtering for usedirect — warns and ignores `--equipment`
      (`camply/search/search_usedirect.py:121-124`).
    - Go's heuristic `strings.Contains(lowerUseType, "site")` (`usedirect.go:211`) matches
      almost every real type-group name (`Campsite`, `Tent Site`, `Premium Campsite`,
      `Enroute Site`…) → nearly everything tagged `Tent`, while excluding `Boat In`/`Hike In`/
      `Day Use`. Grid endpoint has no real equipment data.
    - Fix: either drop equipment filtering for this provider (match Python, warn) or document
      it as intentional best-effort — but replace the `"site"` substring with something principled.

- [ ] **usedirect has no WAF protection (extract shared `internal/httpx`)** — recdotgov now
      rate-limits + retries + sends browser headers via its package-local `getJSON`
      (`recdotgov/http.go`), but usedirect's raw `http` calls have none. Extract that helper
      into a configurable shared `internal/httpx.Client` (per-provider limiter + retry policy + UA pool) and adopt it in usedirect — bundle with the occupancy cleanup below so the
      concurrent fan-out (`usedirect.go:158-180`) gets throttled too.

- [ ] **usedirect occupancy: deviates from Python + redundant double-fetch**
    - Python hardcodes occupancy `(0, 1)`, no per-unit call (`usedirect.py:486`). Go still makes a
      `/rdr/search/details/<unitId>` request per available unit (`usedirect.go:293-334`); the N+1
      is now **concurrent** (sem=5, `usedirect.go:158-180`, commit b596cbc), so latency is
      mitigated but it's still N requests / WAF risk and a behavioral deviation from Python.
    - Remove redundant synchronous call at `usedirect.go:201` (units already pre-fetched
      concurrently at `usedirect.go:158-180`; this call is now a guaranteed cache hit).
    - Document/justify the hardcoded `startdate/2000-01-01` (`usedirect.go:304`) or it may
      return nothing in prod.
    - `time.Now().Truncate(24*time.Hour)` (`usedirect.go:108`) truncates on absolute UTC, so the
      "today-1" floor can land on the wrong wall-clock day in non-UTC zones. Use a date-aware floor.

---

## 🟡 Medium

- [ ] **`refreshMetadata` swallows errors** — `usedirect.go:337-415` ignores HTTP/decode
      errors for places & facilities (and decode errors for filters), returns `nil` even if all
      failed. A WAF block then yields empty rec-area names and `/park/0/<facility>` booking URLs
      (`usedirect.go:195`) instead of surfacing an error.

- [ ] **`consolidateNights` doesn't dedupe raw input** — `internal/core/filter.go:52`.
      Python drops duplicates during consolidation; Go doesn't. A duplicate `(campsite, date)`
      row would break a run via the `diff != 1d` consecutiveness check (and double-count). Low
      risk today (`getSearchMonths` dedupes months upstream), but a provider emitting dup slices
      would silently miss multi-night windows. Add a dedupe + a guarding test.

- [ ] **recdotgov availability uses an allowlist (`status != "Available"`)** —
      `recdotgov.go:329`. Python uses an inverted blocklist `CAMPSITE_UNAVAILABLE_STRINGS`
      (`recdotgov_camps.py:200-203`), treating unrecognized statuses as available. Rarely bites
      (API returns `"Available"`); add a comment or align.

- [ ] **`getSearchMonths` always includes the end-date's month** (`recdotgov.go:293`);
      Python's range is end-exclusive. Harmless to results (window filter drops them) but wastes
      a request per search.

---

## 🟢 Minor / code quality

- [ ] **Test data field mismatch** — `metadata_response.json` uses `parent_name` but the
      struct reads `json:"asset_name"` (`metadata.go:77`) → `Unknown Campground` in test output;
      facility-name hydration is effectively untested.
- [ ] **Provider construction duplicated** across `campsites.go:68-75`,
      `campgrounds.go:38-45`, `recreation_areas.go:32-39` with a repeated hardcoded URL literal.
      Add a `providers.New(name)` factory.
- [ ] **`map[string]interface{}` in notifications** (`notifications.go:84,131`)
      contradicts `GEMINI.md:87`. Use typed payload structs.
- [ ] **Inconsistent logging** — providers and notifications use raw `fmt.Printf`/`fmt.Println`
      (`recdotgov.go:63`, `usedirect.go:100`, `notifications.go:177,198,204`) while the rest of the
      code uses the `internal/logger` package. Route these through `logger` for consistent,
      level-aware output.
- [ ] **`core.Filter` is an empty stateless struct** — could be package-level functions.
- [ ] **Concurrency:** `metadataFetched` and metadata maps mutated without a lock
      (`usedirect.go:337-415`). Fine for current sequential flow; a footgun if a `Provider` is
      reused across goroutines.

---

## ✅ Completed

- [x] **recdotgov WAF/403 hardening — browser headers + rate limiting + retry** — ✅ fixed
    - New shared helper `internal/providers/recdotgov/http.go` (`getJSON`) owns the full
      request lifecycle: rate limit → rotating Chrome UA + `STANDARD_HEADERS`-style headers +
      caller `Referer`/`apikey` → retry → decode → close. All four call sites
      (`getAvailability`, `fetchMetadata`, `FindCampgrounds`, `FindRecreationAreas`) now go
      through it; `fetchMetadata` gained the previously-missing `Referer`.
    - Rate limit: package-global `rate.NewLimiter(3, 3)` (3 req/s, burst 3), mirroring
      Python's `@ratelimit.limits(calls=3, period=1)`.
    - Retry ("Moderate" profile): retry only `429`/`5xx`/network errors; `403` (WAF) and
      `404` are terminal (headers defeat a 403, retrying can't). Full-jitter exponential,
      base 1s, factor 2, per-sleep cap 8s, stop at 5 retries OR 30s — ~12s typical / ~23s
      worst per failing request. Bound deliberately short because long-running searches use
      an external CronJob, not an in-process daemon.
    - Deliberately omits `Accept-Encoding` (Python sets it) to keep Go's transparent gzip
      decompression; documented in `setBrowserHeaders`.
    - Also resolves the Medium "defer inside pagination loops" item (`getJSON` closes each
      body per request). Added `dep golang.org/x/time v0.15.0` and `http_test.go`
      (429-retry, 403-terminal, header assertions).

- [x] **`consolidateNights` produced duplicate & over-long records** — ✅ fixed in `dc0ca74`.
      Rewrote `internal/core/filter.go` to emit sliding windows of exactly `requiredNights`
      with composite `CampsiteID|FacilityID` grouping; added `internal/core/filter_test.go`.
      Follow-up: the `site.BookingNights < req.Nights` guard at `filter.go:20` is now dead
      code (consolidation normalizes `BookingNights`) — safe to drop.

---

## 📋 Scope still to port ("so far")

- [ ] ~~Daemon modes: `--continuous`, `--search-forever`, `--polling-interval`,
      `--search-once`~~ — **not porting.** Long-running/repeated searches are handled by an
      external scheduler (Kubernetes CronJob), not an in-process loop. This is also why the
      recdotgov retry bound is intentionally short (cron re-runs handle repetition).
- [ ] Offline-search state (`--offline-search`, `--offline-search-path`)
- [ ] `--yaml-config`, `--day`, `--equipment-id`
- [ ] Commands: `equipment-types`, `list-campsites`, `configure`, `tui`
- [ ] Notifications beyond Pushover + Telegram (PRD.md lists 9+; `SetupNotifiers` warns and
      skips the rest — `notifications.go:176-178`)

---

## ✅ Test coverage gaps

- [x] Add a `core.Filter.Apply` / `consolidateNights` test that exercises consolidation —
      ✅ `internal/core/filter_test.go`.
- [x] Verify usedirect `34` assertion (`usedirect_test.go:79`) after the consolidation fix —
      ✅ unchanged (insensitive to the bug; see Completed).
- [ ] Add a `consolidateNights` dedupe test (duplicate `(campsite, date)` rows) — covers the
      "doesn't dedupe raw input" item above.
- [ ] Add a recdotgov facility-name hydration assertion once the `asset_name`/`parent_name`
      mismatch is fixed — covers the "Test data field mismatch" item above.
