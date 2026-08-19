# Review — progressive enhancement for the served page

**What was reviewed:** branch `progressive-enhance-the-serve-page`, commits `becd06f`..`d0e1017` (base: `development` at `becd06f`): the watchlist cap moving to `present.Apply` (100 only when unfiltered), the inline enhancement script with sliding rows, the browser-session cookie store with `-persist`, the enhanced 200-page answer, and the README/usage wording — plus the fixes applied in response to the reviews.

**Reviewers run:** `fpl-code-review`, `fpl-security-review` (a shipped script and a new state channel — the triage row for both), `fpl-docs-accuracy` (README and usage text changed). All three ran concurrently over the same range with the range, the known-wrong items, and the null-is-not-settled note in their briefs.

## Findings, ranked by how misleading the current state was

### fpl-code-review — six findings, all applied at `d0e1017`

1. **The enhanced morph discarded the reader's watchlist state.** The enhanced render read the POST action request — no query, no view — so a morph swapped the watch view back to the default list while the address bar still showed the reader's filter; the morphed forms also stamped `ret=/action`, which a plain submission 303s into a 403. → **Applied:** `readerRequest` renders the fresh page from the POSTed `ret`. Pinned by `TestTheEnhancedAnswerReadsTheReadersState`.
2. **The tab badge and the gate sentence counted the whole pool beside a list capped at 100.** → **Applied:** the sentence names its scale — "2 of the 575 players outside your fifteen do" — and the README, the caption and the regenerated screenshot say so.
3. **Ctrl-C printed "error: http: Server closed" and exited 1.** → **Applied:** `http.ErrServerClosed` returns nil from `cmdServe`.
4. **A stalled enhanced POST left the page permanently deaf** — the busy flag had no escape. → **Applied:** the clicked button disables and an `AbortController` aborts at 20s into the catch, which reloads.
5. **The enhanced response path had no Go test.** → **Applied:** `TestTheEnhancedAnswerReadsTheReadersState` pins the state derivation, which is the part a test can reach without a live engine.

### fpl-security-review — no highs; three lows and a note, all lows applied

1. **The page was framable, and a framed page's forms carry a valid token** — the token gate cannot see a clickjacked click. → **Applied:** `X-Frame-Options: DENY` on every render.
2. **The enhanced path trusted "200 = a complete page"** — a template error mid-write would have let the script morph a truncated page. → **Applied:** `render` builds into a buffer and answers 500 on error, so the script's reload path is the self-heal.
3. **The enhanced answer's reader-state loss** — same as code finding 1, same fix.
4. Note, accepted: the cookie's host-scoping quirks (separate stores for `localhost` vs `127.0.0.1`; other loopback services can see and set `fpl_overrides`) are within the design — the cookie is deliberately not secret, codes are bootstrap-validated before use, and the store never touches config.

### fpl-docs-accuracy — five findings, all applied

1. The intro paragraph still claimed the buttons "write standing overrides back to config". → **Applied:** now names the session default and the `-persist` opt-in.
2. "two of a hundred do" no longer matched the page's full-pool note. → **Applied:** "two of the whole pool do".
3. The image caption's "two of 100". → **Applied:** the screenshot was regenerated from the current build and the caption now carries the page's own "two of the 575 players outside the fifteen".
4. The usage tail sentence said serve "only takes -addr". → **Applied:** "-addr and -persist".
5. Optional wording hazard: "the session cookie … removed outright" now collides with the new browser-session cookie. → **Applied:** "FPL's session cookie".

## What was declined

Nothing. All fourteen findings were applied.

## What could not be checked on this harness

- **The browser smoke ran under a broken clock:** the snap chromedriver in this environment kills sessions after ~60 seconds, so each check was one atomic pass — the boot morph was verified (row out, slide-in row in, excluded card, one request, no errors, config byte-identical), but the un-boot path, the persist path and the watchlist-state preservation in a real browser are covered by unit and template tests only.
- **The regenerated `docs/images/watchlist.png` pixels** were not visually inspected — no vision this session. The screenshot was taken after a programmatically verified view switch, and every figure its caption states was read off the same run's rendered page.

The gate's own caution applies: every finding above was verified against the code before applying — the reader-state loss and the ret=/action dead-end by tracing `render`'s inputs, the framing issue by re-reading the token flow with a framed document in mind — and each fix that a test can reach is pinned by one.
