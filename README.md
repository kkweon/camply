<div align="center">
<a href="https://github.com/kkweon/camply">
  <img src="https://raw.githubusercontent.com/kkweon/camply/develop/.github/assets/camply.svg"
    width="400" height="400" alt="camply">
</a>
</div>

> [!NOTE]
> **Fork Notice:** This is an independently maintained fork of the original [juftin/camply](https://github.com/juftin/camply) project, rewritten in Go. It ships a standalone binary and multi-architecture Docker images (including ARM64/Raspberry Pi) to GHCR. PyPI releases are not supported.
>
> The Python implementation this fork started from was removed once the Go rewrite took over. It is still readable at the [`python-final`](https://github.com/kkweon/camply/tree/python-final) tag.

**`camply`**, the campsite finder ⛺️, is a tool to help you book a campsite online. Finding
reservations at sold out campgrounds can be tough. That's where camply comes in. It searches
the APIs of booking services like [recreation.gov](https://recreation.gov) for cancellations
and availabilities — once a campsite becomes available, camply sends you a notification to
book your spot.

---

---

<p align="center">
  <a href="https://github.com/kkweon/camply/blob/develop/LICENSE"><img src="https://img.shields.io/github/license/kkweon/camply?color=blue&label=License" alt="GitHub License"></a>
  <a href="https://github.com/kkweon/camply/actions/workflows/ci.yaml?query=branch%3Adevelop"><img src="https://github.com/kkweon/camply/actions/workflows/ci.yaml/badge.svg?branch=develop" alt="CI Status"></a>
  <a href="https://go.dev"><img src="https://img.shields.io/github/go-mod/go-version/kkweon/camply?logo=go&label=Go" alt="Go Version"></a>
  <a href="https://github.com/go-task/task"><img src="https://img.shields.io/badge/task---?message=task&logo=task&color=teal&labelColor=grey" alt="task"></a>
  <a href="https://github.com/pre-commit/pre-commit"><img src="https://img.shields.io/badge/pre--commit-enabled-lightgreen?logo=pre-commit" alt="pre-commit"></a>
  <a href="https://github.com/semantic-release/semantic-release"><img src="https://img.shields.io/badge/%20%20%F0%9F%93%A6%F0%9F%9A%80-semantic--release-e10079.svg" alt="semantic-release"></a>
  <a href="https://gitmoji.dev"><img src="https://img.shields.io/badge/gitmoji-%20😜%20😍-FFDD67.svg" alt="Gitmoji"></a>
</p>

## Installing

Download a binary from the [latest release](https://github.com/kkweon/camply/releases/latest) —
`linux` and `darwin`, `amd64` and `arm64` are published, with a `checksums.txt` alongside them:

```commandline
curl -sSfL -o camply \
  https://github.com/kkweon/camply/releases/latest/download/camply_<version>_linux_amd64
chmod +x camply && sudo mv camply /usr/local/bin/
```

Or build it yourself:

```commandline
go install github.com/kkweon/camply/cmd/camply@latest
```

Or use the Docker image — see [Docker Usage](#docker-usage).

## Usage

The provider comes first, then the command. Every flag a command offers is one that
provider's API can actually act on, so `--state` exists on `recdotgov` and not on
`reservecalifornia`.

Search for a recreation area (recreation areas contain campgrounds):

```commandline
camply recdotgov recreation-areas --search "Glacier National Park"
```

Search for campgrounds (campgrounds contain campsites):

```commandline
camply recdotgov campgrounds --search "Fire Lookout Towers" --state CA
```

Search for available campsites and get a Pushover notification for each one found:

```commandline
camply recdotgov campsites \
    --rec-areas 2725 \
    --date-ranges 2026-07-10:2026-07-18 \
    --nights 3 \
    --notifications pushover
```

`camply <provider> <command> --help` lists everything a provider accepts.

### Flag conventions

- **Multi-value flags are plural** and take either a comma-separated list or repetition:
  `--campgrounds 232461,234039` is the same as `--campgrounds 232461 --campgrounds 234039`.
- **Date windows are one value**, not a start/end pair: `--date-ranges 2026-09-04:2026-09-07`.
  Repeat it to search several windows at once. `--start-date`/`--end-date` still work for a
  single window.
- **A filter that matches nothing is an error**, not a silent empty result. Pass
  `--allow-partial-match` to continue when an equipment filter matches at some campgrounds
  but not others.
- **Missing data is surfaced, never acted on.** No filter excludes a site because the
  provider reported nothing about it — the site reaches the results flagged ⚠️ so you can
  check it, and the run says how many it flagged.

Notable filters: `--shelter`, `--parking`, `--hookups`, `--campsites` (watch individual
sites by ID), `--equipment-types`, `--max-equipment-length`, `--nights`, `--weekends`,
and — for ReserveCalifornia — `--min-vehicle-length`.

### The three axes

A booking API's `campsite_type` is four orthogonal facts in one string slot, and each
campground's operator picks a different one to put there. Zephyr Cove spends it on
shelter, so all 47 of its hike-in sites are typed `TENT ONLY NONELECTRIC` — identical to
its drive-in tent sites. Lodgepole spends it on access, so 3 of its drive-in tent sites
are typed `WALK TO` and vanish from a tent search. camply reads the underlying fields
instead and filters on three independent axes:

```bash
camply recdotgov campsites --shelter tent --parking at-site,walk \
  --campgrounds 232461 --date-ranges 2026-09-04:2026-09-07 --nights 2
```

- **`--shelter tent|rv|cabin`** — one value, because it is a choice rather than a list.
  It matches against what the site _accepts_, so a site taking both a tent and an RV
  answers yes to either camper. Someone bringing both runs the search twice.
- **`--parking at-site,walk,none`** — how close a car gets. Three levels, not a
  drive-in/not flag, because that flag put Kaspian's 30-foot stroll and Zephyr Cove's
  half-mile haul in the same bucket.
- **`--hookups electric,water,sewer`** — additive, so naming two requires both. `water`
  means a pipe at the site for an RV and a shared tap for a tent; the alert always names
  which one it found.

### Missing data is never acted on

No filter excludes a site because the provider reported nothing about it. The site reaches
the results flagged ⚠️, and the run says how many it flagged and at which campgrounds.
Every result also reports its access, what it accepts and its hookups whether or not you
filtered on them — that part needs no flag, because the incident that prompted all of this
was an alert that simply said nothing.

Meeks Bay is why it matters: it reports no equipment for 17 of its 88 campsites, and
treating that silence as "does not match" threw away 242 nights of real availability from
a single September search.

`--exclude-no-vehicle-access` still works and is equivalent to `--parking at-site`.

## Providers

Run **`camply providers`** to list them:

| Provider            | Service                                                                       | Status              |
| ------------------- | ----------------------------------------------------------------------------- | ------------------- |
| `recdotgov`         | [Recreation.gov](https://recreation.gov) (US Federal)                         | available           |
| `reservecalifornia` | [ReserveCalifornia.com](https://reservecalifornia.com) (CA State Parks)       | available           |
| `goingtocamp`       | [GoingToCamp](https://goingtocamp.com) (Canada & US)                          | not implemented yet |
| `yellowstone`       | [Yellowstone National Park Lodges](https://yellowstonenationalparklodges.com) | not implemented yet |

Each provider name has aliases — `recdotgov` also answers to `RecreationDotGov`,
`recreation-dot-gov`, and `recgov`.

## Configuration

camply reads `~/.camply`, a `KEY=value` file. Every key can also be supplied through the
environment, which is how it is configured under Kubernetes:

```shell
# Pushover
PUSHOVER_PUSH_USER=your_pushover_user_key
PUSHOVER_PUSH_TOKEN=your_pushover_app_token

# Telegram
TELEGRAM_BOT_TOKEN=your_telegram_bot_token
TELEGRAM_CHAT_ID=your_telegram_chat_id
```

`PUSHOVER_PUSH_TOKEN` is optional — without it camply falls back to its own registered
Pushover application. Check your setup with:

```commandline
camply test-notifications --notifications pushover
```

## Docker Usage

`camply` publishes a multi-architecture image to `ghcr.io/kkweon/camply:latest`, with the
CLI as its entrypoint. Mount your `~/.camply` to pick up notification credentials:

```bash
docker run --rm -it \
  -v ~/.camply:/root/.camply \
  ghcr.io/kkweon/camply:latest recdotgov campsites \
    --campgrounds 232450 \
    --date-ranges 2026-06-01:2026-06-14 \
    --notifications pushover
```

camply searches once and exits — there is no daemon mode. To poll continuously, run it on a
schedule (a Kubernetes `CronJob`, a systemd timer, or plain `cron`).

## Contributing

Development uses [Task](https://taskfile.dev):

```commandline
task install    # install the pre-commit hooks
task test       # go test ./...
task lint       # golangci-lint, gofumpt, goimports, prettier
task run -- recdotgov campsites --help
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org); releases are cut
automatically by semantic-release from the commit history.

`PRD.md` records the CLI contract and the places the Go CLI deliberately diverges from the
original Python one. `TODO.md` tracks the remaining work.

<br/>

Recreation data provided by [**Recreation.gov**](https://ridb.recreation.gov/)

---

---

<br/>
