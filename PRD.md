# Camply CLI Product Requirements Document (PRD)

This document records the exact behavior, commands, and flags of the original Python `camply` CLI, and the places where the Go CLI deliberately departs from it.

The Python implementation itself has been removed — it is readable at the `python-final` tag (`git show python-final:camply/cli.py`). That makes this document the only written record of the contract the Go CLI inherited, so it stays: where Go does not match, the difference below should say why.

## Global Options

These options apply to almost all commands.

- `--provider [TEXT]`: Camping Search Provider. Defaults to 'RecreationDotGov'. Valid options are typically listed by the `providers` command.
- `--debug/--no-debug`: Enable extra debugging output (verbose logging).

## Go CLI Command Structure

The Go rewrite deliberately diverges from the Python CLI here. This is a
documented decision, not drift.

Python takes `--provider` on a single `campsites` command. The Go CLI puts the
provider first:

```
camply recdotgov campsites --campgrounds 232461 --date-ranges 2026-09-04:2026-09-07
camply reservecalifornia campsites --campgrounds 453 --equipment-types Vehicle
```

**Why.** `--provider` on one command makes every flag look applicable to every
provider, and they are not. A ReserveCalifornia equipment name passed to
RecreationDotGov matched nothing at four of six campgrounds and the job reported
"0 New Campsites Found" for weeks, indistinguishable from a full campground.
With the provider as the subcommand, a flag its API cannot act on is never
registered — `--state` exists on `recdotgov` and not on `reservecalifornia`,
because UseDirect has no state filter.

Other deliberate differences from the Python CLI:

- **Multi-value flags are plural** (`--campgrounds`, `--rec-areas`,
  `--campsite-types`) and accept both comma-separated and repeated forms.
- **`--date-ranges START:END`** replaces the `--start-date`/`--end-date` pair.
  Those were parallel slices zipped by position, so a mismatched order passed
  validation and silently searched the wrong windows. `--start-date` and
  `--end-date` remain for a single window and reject a second occurrence.
- **`--equipment-types` and `--max-equipment-length`** replace the `Name,Length`
  syntax, which never worked: the flag is a slice, so the comma split first and
  the length arrived as a second equipment name.
- **`--campsite-types`** has no Python equivalent. It filters on the
  `campsite_type` field, which is what separates drive-in sites from walk-in
  ones; equipment cannot, because a `WALK TO` site still permits a tent.
- **Values are validated against per-provider vocabularies**, and a filter that
  drops a whole campground is an error rather than an empty result.

The Python-facing flag names remain accepted as aliases with a deprecation
warning, as do the top-level commands.

## Core Commands

### 1. `campsites`

Finds available campsites with custom search criteria. This is the primary workhorse command.

**Arguments:**

- `--start-date [YYYY-MM-DD]`: **(Required)** Start of the search window. Can be passed multiple times to define multiple discrete search windows.
- `--end-date [YYYY-MM-DD]`: **(Required)** End of the search window. Must be paired with `--start-date` and passed the same number of times.
- `--rec-area [TEXT]`: Search by Recreation Area ID. Can be passed multiple times (e.g. `--rec-area 1 --rec-area 2`). Automatically resolves the recreation area down to its constituent campgrounds and searches them all.
- `--campground [TEXT]`: Search by Campground ID. Can be passed multiple times.
- _Note: At least one `--rec-area` OR `--campground` must be provided._
- `--campsite [TEXT]`: Add individual Campsites by ID. Limits search results strictly to these specific sites within the selected campgrounds. Can be passed multiple times.
- `--nights [INTEGER]`: Minimum number of consecutive nights. Defaults to 1.
- `--weekends`: Only search for weekend bookings (Friday/Saturday nights).
- `--day [TEXT]`: Day(s) of the Week to search (e.g. `Monday`). Can be passed multiple times.
- `--equipment [TEXT...]`: Search for campsites compatible with camping equipment. Format: `Name,Length` (or just `Name`). Length `0` means no filter. Names are case-insensitive (`Tent`, `RV`, `Trailer`). Can be passed multiple times.
- `--equipment-id [TEXT]`: (GoingToCamp specific) Filter by raw equipment ID.
- `--notifications [TEXT]`: Triggers the notification pipeline. Options: `pushover`, `email`, `ntfy`, `apprise`, `pushbullet`, `slack`, `telegram`, `twilio`, `webhook`, `silent`. Can be passed multiple times to trigger multiple channels.

**Daemon/Continuous Searching Arguments:**

- `--continuous`: Continuously check for availability. Quits once at least one campsite is found.
- `--search-forever`: Same as continuous, but does not quit after finding a site. Instead, it runs forever (but won't notify about the identical campsite/date combo twice).
- `--polling-interval [INTEGER]`: Wait time between continuous checks (in minutes). Defaults to 10 (minimum 5).
- `--notify-first-try`: In continuous mode, if >5 sites are found immediately, normally it suppresses spam by only sending 5. This flag forces sending all of them.
- `--search-once`: Used for Cron jobs. Runs a continuous-style check exactly once (saving state via offline search) so you get notifications without running a long-lived process.

**Offline State Arguments:**

- `--offline-search`: Saves results to disk. Next time it runs, it won't send duplicate notifications for sites it already found.
- `--offline-search-path [FILE]`: Path to the offline JSON/pickle file. Defaults to `camply_campsites.json`.

**Configuration:**

- `--yaml-config` / `--yml-config [FILE]`: Pass a YAML file containing all the parameters instead of using CLI flags.

### 2. `campgrounds`

Searches for Campgrounds and lists their IDs and metadata.

**Arguments:**

- `--search [TEXT]`: Search for Campgrounds by name/string.
- `--state [TEXT]`: Filter by US state code (e.g. `CA`, `CO`).
- `--rec-area [TEXT]`: Filter campgrounds strictly to those inside this Recreation Area ID. Can be passed multiple times.
- `--campground [TEXT]`: Lookup specific campgrounds by ID. Can be passed multiple times.
- `--campsite [TEXT]`: Lookup campgrounds that contain these specific Campsite IDs.

### 3. `recreation-areas`

Searches for Recreation Areas (like National Parks or Forests) and lists their IDs.

**Arguments:**

- `--search [TEXT]`: Search for Recreation Areas by name/string.
- `--state [TEXT]`: Filter by US state code.

### 4. `test-notifications`

Validates the user's `~/.camply` configuration by sending a dummy payload to the requested services.

**Arguments:**

- `--notifications [TEXT]`: **(Required)** The notification provider to test (e.g., `pushover`, `telegram`). Can be passed multiple times.

### 5. `providers`

Prints a simple list of the supported backend APIs (e.g. `RecreationDotGov`, `GoingToCamp`, `Yellowstone`, `ReserveCalifornia`).

### 6. `equipment-types`

Prints a list of supported equipment for a specific provider/area (primarily used for GoingToCamp which uses strict IDs instead of strings).

**Arguments:**

- `--rec-area [TEXT]`: The Recreation Area to query equipment for.

### 7. `list-campsites`

Lists all raw campsite IDs that exist within a given campground or recreation area.

**Arguments:**

- `--rec-area [TEXT]`: Recreation Area ID.
- `--campground [TEXT]`: Campground ID.

### 8. `configure`

Opens an interactive terminal prompt to build a `.camply` configuration file in the user's home directory. Prompts for API keys (Pushover, Telegram, Twilio, etc.).

### 9. `tui`

Opens the Textual Terminal User Interface (a visual dashboard). _(Note: Likely out of scope for the initial Go rewrite, as rebuilding a TUI in Go is a distinct project)._
