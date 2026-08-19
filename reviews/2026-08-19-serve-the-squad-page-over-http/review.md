# Review — serve the squad page over HTTP

**What was reviewed:** branch `serve-the-squad-page-over-http`, commits `22c8903`..`6df15aa` (base: `development` at `342530c`): the `armband serve` command, the lock/boot write actions, the flat 100-player watchlist with server-side filter/sort/pagination, the shared squad-page pipeline, and the README updates — plus the fixes applied in response to the reviews.

**Reviewers run:** `fpl-code-review`, `fpl-security-review` (config persistence gained a write path — the triage row for both), `fpl-docs-accuracy` (README and docs/images changed). All three were dispatched concurrently over the same range with the range, the known-wrong items, and the null-is-not-settled note in their briefs.

## Findings, ranked by how misleading the current state was

### fpl-code-review

1. **The lock button lied under a minutes override.** `pageOverrides` filled each element's single badge slot from three lists, and the minutes list ran last — so a locked player who also carried a minutes correction bound the MINUTES override to his card, rendering an OFF lock button on a player the optimiser is forced to keep. Clicking it re-wrote the lock with the page's canned reason and cleared MustStart. The same overwrite cost an excluded player his EXCL badge, which the watchlist's skip set reads. → **Applied:** `player` gained a `bind` flag; first writer wins, lock and exclude before minutes. Pinned by `TestALockedPlayerKeepsHisLockBadgeUnderAMinutesOverride`.
2. **Plain `armband squad` ran the whole page pipeline and threw it away.** The refactor moved the page assembly out of the `htmlPath` guard, so the terminal command gained the owned-squad fetches, a full transfer search and pool-wide passes it never had. → **Applied:** `buildSquadPage` takes `wantPage`; `cmdSquad` passes `htmlPath != ""`. Not pinned by a test — exercising it needs a live engine, and the gate is one boolean; recorded rather than asserted.
3. **The legend said "ranked by the model's score" above a price-sorted table**, and the static export has no control to recover the score order. → **Applied:** legend reworded to "chosen by score, ordered by price/the column you sort on". The unreachable empty-filter message on the static page was made conditional at the same time.

### fpl-security-review

1. **DNS rebinding could defeat the token gate** — the listener bound loopback but never checked `r.Host`, so a rebound browser reads the page same-origin. → **Applied:** `loopbackHost(r.Host)` gates `ServeHTTP`. Pinned by `TestServeAnswersByLoopbackHostOnly`.
2. **The `ret` open-redirect check missed backslashes** (`/\evil.com` parses as an authority). → **Applied:** `safeRetPath` rejects `\` and control characters. Pinned in `TestTheActionsWriteAndLiftOverridesByPermanentCode`.
3. **Unbounded POST body parsed under the mutex before the token check.** → **Applied:** `http.MaxBytesReader` at 64 KB before anything else. Pinned by `TestTheActionClampsTheBodyBeforeTheTokenCheck`.
4. **A stale form could persist a phantom override** — a code the bootstrap does not contain saved as a nameless exclusion. → **Applied:** the handler refuses codes that do not resolve against the bootstrap. Pinned in `TestTheActionsWriteAndLiftOverridesByPermanentCode`.

### fpl-docs-accuracy

1. "sortable by any column" — the fixture strip is not sortable. → **Applied:** "every column but the fixture strip".
2. "every player carries a lock and a boot" — only the fifteen do. → **Applied:** "every player in the fifteen".
3. The "two of a hundred do" figure was undated. → **Applied:** "On this run's data, two of a hundred do", and both image captions now name the 2026-08-19 run.
4. The free-commands mermaid node did not list `serve`. → **Applied**.
5. The two README screenshots depicted different data states. → **Applied:** `docs/images/squad-eleven.png` was regenerated from the same live run as the new `watchlist.png`, and its caption figures (3-5-2, 46.5 XI pts/gw, B.Fernandes captain, £100.0m spent, £0.0m left, GW1, two days before the deadline) were taken from that run's own page.

## What was declined

Nothing. All ten findings above were applied.

## What could not be checked on this harness

- **The pixel content of the two regenerated PNGs.** This session has no vision, so the screenshots were produced by headless chromium over a page whose DOM and figures were verified programmatically (view visibility, row counts, the gate sentence) — the caption claims were cross-checked against the rendered HTML, but nobody looked at the image.
- **The smoke test's full suite numbers** were gathered against the live FPL API on 2026-08-19; the README's "two of a hundred" figure is live-data-dependent by design and will drift.

The gate's own caution applies: a reviewer's report is a set of proposals. Each finding above was verified against the code before applying — finding 1 by rendering the page, finding 2 by tracing `buildTransferBoard`, the security findings by tracing the request paths — and the fixes are pinned by tests where a test can reach them.
