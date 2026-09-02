# Playwright end-to-end suite

Drives a real `armband serve` in a real headless browser. See
[docs/architecture.md](../docs/architecture.md)'s `e2e/` section for what this suite is for, what
it asserts, and how it differs from `internal/webui`'s own Go suite. This file is the "how to run
it" half.

## What this is not

- **No pixel comparison.** `toHaveScreenshot`, `toMatchSnapshot` over images and a committed PNG
  directory are all forbidden here — see `internal/webui/visual_test.go`'s own ruling on why the
  layout goldens are a local-only check. A screenshot, trace and video are captured only as
  **failure artefacts** (`playwright.config.js`'s `use.trace`/`screenshot`/`video`), so a red run
  can be read without reproducing it blind.
- **Not part of `go test ./...`.** It is a separate Node toolchain and a separate CI job
  (`.github/workflows/ci.yml`'s `e2e` job), specifically so a browser download and a live server
  never compete with the Go suite for the same job, and so a Go failure never reads as a
  mysterious e2e timeout or vice versa. Locally it is entirely opt-in.

## Running it locally

```bash
go build -o ./armband ./cmd/armband
cd e2e && npm ci
ARMBAND_E2E_CHROMIUM=/snap/bin/chromium npm test   # on a machine with only the snap chromium
npm test                                            # on a machine that can run `npx playwright install chromium`
```

`ARMBAND_E2E_CHROMIUM` points the suite at an already-installed browser binary — the snap
chromium on a VM with no other install path and no unprivileged sandbox — with `--no-sandbox`
applied ONLY in that case (see `playwright.config.js`'s own comment). Leaving it unset uses
Playwright's own pinned download, which is what CI uses via `npx playwright install --with-deps
chromium`, and is the more representative run wherever it's available: the exact browser build
`e2e/package-lock.json`'s pinned `@playwright/test` names, not whatever happens to be on `PATH`.

`npm test:headed` runs the same suite with a visible browser window (drops automatically to
Playwright's own bundled browser if `ARMBAND_E2E_CHROMIUM` names a headless-only install).
`npm run report` opens the last HTML report.

## What it never does, structurally

The `webServer` block in `playwright.config.js` starts the server itself, from the SAME command
in every environment — `node scripts/serve-fixture.mjs` — rather than a CI step that backgrounds
the binary and a local script that does something else. `reuseExistingServer` is `false` even
locally: `armband serve` mints a random write token per process, and a reused server is one whose
token this harness never captured, which would silently drive every write test against the
unauthenticated shell (see `cmd/armband/webroutes.go`'s `authCookieName` comment).

The server never runs with `-persist`. Every lock, block, and chip placement this suite issues
lives in the `fpl_session` cookie for the life of one browser context and dies with it — nothing
here ever writes `e2e/testdata/config.json` or a team file.

`fixtures.js`'s egress guard aborts and records any request that leaves `baseURL`; every spec's
teardown fails if that list is non-empty. `egress-guard.spec.js` is the positive control proving
the guard still fires — see its own comment for why a plain `fetch()` from the app page cannot be
used to prove it (the page's own Content-Security-Policy blocks that fetch before Playwright's
route interception ever sees it, which is a second, independent guard rather than a broken test).

## What is unreachable under this fixture, and why

The suite is deterministic because every test primes the SAME committed live capture
(`data/captures/<LIVE_CAPTURE>`, named in `e2e/scripts/live-capture.js` and pinned against
`internal/capture.LiveCapture` by `internal/capture/analysisfixture_test.go`'s
`TestEveryFixtureNamesTheLiveCapture`) — GW1 of 2026-27, before that gameweek's deadline. That
buys determinism at the cost of three areas nothing here can reach:

- **Importing a team.** `cmd/armband/importwindow.go`'s gate requires the next gameweek's id to
  be at least 2; this capture's `is_next` names gameweek 1. `smoke-boot.spec.js` asserts the
  *closed* state (the import card hidden) instead of skipping the area silently.
- **The transfer planner**, which needs an imported entry to have rows to plan against.
- **Any past-gameweek result.** `GET /api/results` needs a closed gameweek; under an unimported
  session at GW1 none exists yet. `gameweek-rail.spec.js` notes this at its own call site.

## The wall clock drifts, the capture does not

The capture's own deadline is already in the past relative to whenever this suite runs — it is
not re-captured on a schedule. Two consequences, both load-bearing:

- **No spec may assert a time-derived TEXT value** (a countdown string, a formatted date) — only
  presence/non-emptiness. `smoke-boot.spec.js` and `gameweek-rail.spec.js` both do this
  deliberately; see their own comments.
- **Which gameweek is "current" is a function of the real calendar, not the capture.**
  `buildGameweeks` (`internal/viewmodel/build.go`) computes the open planning horizon from the
  server's wall clock, so `state.gameweeks` names whichever gameweeks have not yet had their
  deadline pass TODAY — not gameweek 1. `smoke-boot.spec.js`'s determinism control checks
  something the capture actually fixes (the club list) rather than the gameweek number for
  exactly this reason.

## The `#optimise` button

Any spec clicking `#optimise` does so as close to the first action in its test as it reasonably
can. It is not permanently disabled after one use — `armband.css`'s `body.saving` rule sets
`pointer-events:none` on every `.btn`/`.card` only while a save is actually in flight — but every
build runs the optimiser (real per-test latency, `workers: 1` in `playwright.config.js`), so
starting from a clean, unmodified session keeps each test's own assertions about what changed
unambiguous.

## Cost

Every `/api/state` build likely runs the optimiser, and the suite runs serially
(`playwright.config.js`'s own comment: the server holds one engine behind one mutex, so parallel
workers would only queue on it). Watch total wall-clock against the CI job's 20-minute budget; if
a future addition makes that tight, narrow which specs run on both `desktop` and `phone` projects
versus `desktop`-only, and say so in a `playwright.config.js` comment rather than deleting
assertions to make the number smaller.
