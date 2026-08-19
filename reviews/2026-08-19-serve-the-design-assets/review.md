# Review — serving the fplarmband.com design, and the JSON contract under it

**Range reviewed:** `c3ee6ff..HEAD` on `serve-the-design-assets`, eleven commits plus a
merge of `origin/main`.

**What it is.** `armband serve` stops rendering a Go-templated squad page and hosts a
client application instead: a landing page at `/`, the planner at `/app`, the design system
under `/assets/`, and the numbers over `GET /api/state`. The application is the design
delivered at `fplarmband.com/design-assets`, vendored into `internal/webui` and compiled
into the binary. The contract it reads is `internal/viewmodel` — plain Go structs with no
transport in them, so the Wails desktop build and the website can bind to the same shape
rather than each restating it.

`squad -html` and `transfers -html` are retired and say where the page went; `brief -html`
is unchanged, because it goes through a Markdown converter rather than the page template.
`present.Render` survives with no product caller, deliberately and with the reason recorded
in the source.

Nine screenshots at 1440px and 390px are committed as visual-regression goldens, driven by
headless chromium against a committed GW1 capture with the clock, the token and the
timezone pinned.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| `fpl-security-review` | yes | a new HTTP surface with four routes, a cookie, and a client that renders untrusted strings by `innerHTML` |
| `fpl-code-review` | yes | the change's central claim is that the client computes no model quantity, which is exactly the class this repository has paid for most often |
| `fpl-docs-review` | yes | `README.md` changed, and a design note was written to the research store |
| `fpl-stats-review` | no | nothing in `internal/analysis` moved and no measurement was made or quoted. The one edit under `internal/backtest` adds an entry to `TestEveryScoringEngineGetsRecency`'s known-exception map, which is a process fact rather than a statistical one — and it is asserted rather than asserted-in-prose, by `TestTheFixtureMatchesWhatProductionBuildsAtGW1` |
| `fpl-findings-audit` | no | no verdict in `AGENTS.md` was added, moved or leaned on |
| `fpl-run-review` | no | no live run wrote config |

**A reviewer's report is a set of proposals.** Every finding below was reproduced before it
was acted on, and two of them turned out to be wrong in the reviewer's favour rather than
mine — recorded as declined, with the reason.

## Findings, ranked by how misleading the state was

### 1. The client was still scoring players — APPLIED

`app.js`'s `xpFor` multiplied `FixtureAdjXP90`, which is already averaged over each
fixture's difficulty, by a hand-rolled ladder `1 + (3 − fdr) × 0.055` that exists nowhere in
Go. It dropped `AvailabilityFactor`, whose most important value is 0; dropped `Congestion`,
`RoleFactor` and `FixtureLoad`; and used expected minutes where the model uses a reliability
figure. It drove the score bug, the captain's arithmetic, the formation comparison and the
armband picker, while `squad.xi_score` and `squad.expected` arrived in the contract and were
read by nothing.

This is the finding that matters most, because the branch's premise was that it could not
happen. Three smaller instances had been found and removed by hand before review; the
largest one survived that sweep.

**Applied**: the projection is the model's. A per-player, per-gameweek figure is not in the
contract, and inventing one client-side is how this started, so the rail's numbers no longer
vary by week until the contract carries one.

**And a test that could see it**: `TestThePageHeadlineIsTheModelsNumber` renders the page in
a browser and compares its printed projection against `squad.expected`;
`TestEveryCardShowsTheModelsProjection` does the same per card. Nothing else could —
the API tests passed because the API was right, and the goldens passed because they were
generated from the same wrong arithmetic.

### 2. Fixtures were indexed by absolute gameweek into "the next N fixtures" — APPLIED

`hydrate` discarded `Fixture.Gameweek` and every consumer read `FIX[club][S.gw-1]`. Index
equals gameweek at GW1 and nowhere else. At GW2 a card claimed GW2 and drew GW3's opponent;
past the window every strip rendered blank and switching gameweeks changed no number.

The test capture is pinned to GW1, which is the one point in the season where the bug is
invisible — the recorded shape of "code that runs, produces plausible output, and measures
nothing".

**Applied**: the gameweek is carried through and every read asks for one by number. A blank
week is now a stated answer rather than a silent fall-back to a different week's fixture.

### 3. Three unescaped sinks, one on first paint — APPLIED

`renderShapes` joined player names into a sentence, the swap bar read `byId(...).n`, and the
market echoed the reader's own query. All three interpolated into `innerHTML`.

The lesson is in how they were missed: the first sweep looked for interpolations that *looked
like* names — `${p.n}`, `${o.who}` — so it found every site where a name appeared under its
own name. The rule has to be about the **sink**: every `${...}` in a template that becomes
`innerHTML`.

### 4. The escaping test was worth much less than it looked — APPLIED

Its needle `<img src=x onerror` could never match, because `--dump-dom` re-serialises what
the parser built and quotes the attributes. And it renamed one arbitrary player, so which
panels it exercised was luck; the player it happened to pick is in the eleven, and
`renderShapes` only ever lists players *outside* it.

**Applied**: every player is renamed, all four panels are walked, and the assertion matches
the three things that together identify a live element — a real `<`, a span with no real
`>`, and a real quote before the payload. Each of the three obvious weaker patterns fires on
correct output, and the reasoning for all four attempts is recorded in the test, because
each looks right until it is tried. Verified by re-breaking a sink and watching it fire.

### 5. The transfer gate, the bank, and the override filter — APPLIED

The gate was a literal `0.4` in six places while `market.gate` and `clears_gate` were sent
and unread — the same defect this page once had against zero, one constant to the right. The
bank was `100 − spend()`, which asserts the opening allowance as a fact and is right for one
week a season. The override filter's kinds were `min`/`excl`/`team` against a server sending
`lock`/`exclude`/`minutes`/`club`, so every filter matched nothing and rendered "the model is
running unaided here", which reads as a fact about the configuration rather than a broken
control.

### 6. Two panels asserted things nothing computed — APPLIED

The Brief tab was prose written by hand against one GW1 build — a captain, a chip plan, and
the one thing that would change it — presented as this week's verdict and pinned as correct
by a golden. The Players tab claimed "Free transfers: unlimited, until the GW1 deadline" as
a literal.

**Applied**: the Brief panel says the verdict is not wired through and points at
`armband brief`; the free-transfer cell shows the policy's actual banking limit. The
material for a real verdict *is* computed on every request and dropped by `Build` — recorded
below as owed.

### 7. Two harness defects — APPLIED

The goldens were only reproducible in the timezone that generated them: the rail formats
deadlines with `toLocaleDateString`, and a run under `TZ=Pacific/Auckland` moved 491 pixels
with a worst channel delta of 104. `TZ=UTC` is now pinned, and verified under two foreign
zones.

The size-mismatch sentinel travelled as a differing-pixel count of 1, which failed correctly
against a zero-tolerance caller and was swallowed the moment a noise floor of 32 was added.
It has its own field now — a sentinel that travels as a magnitude gets compared against a
threshold sooner or later.

### 8. No Content-Security-Policy, and the landing gate reached nothing — APPLIED

Both reviewers asked for a policy, and the script had been lifted out of the document
specifically to make `script-src 'self'` possible. It was first declined for this change,
because the landing page still carried two inline `onsubmit` handlers so the header could
not be applied uniformly — and then that turned out to be the same defect as something the
user had asked for and this work had not delivered. `POST /gate` exists, validates, sets its
cookie and redirects; **no shipped page could reach it.** Both forms were
`onsubmit="event.preventDefault()"` with no action, no method and no `name` on the input, so
the reader saw "check your inbox" and nothing was sent. A server-side test of an endpoint
says nothing about whether anything calls it.

**Applied**: the handlers moved into `landing.js`, which posts to `/gate` and has a pending
and a failure state as well as the success one it shipped with. With nothing inline left,
both documents carry `default-src 'self'; script-src 'self'` with no `'unsafe-inline'`,
`connect-src 'self'`, `frame-ancestors 'none'` and `base-uri 'none'`.

`style-src` does allow inline, and that is a judgement rather than an oversight: the landing
page carries a large inline `<style>`, and a stylesheet buys a defacement where a script
buys a same-origin read of the reader's squad.

Two tests keep it satisfiable — one asserting the policy has no `'unsafe-inline'` in
`script-src`, one scanning both documents for inline handlers and script blocks, because the
natural way to add a control to a page is `onclick=` and the failure mode is a control that
silently stops working in a browser while every test still passes.

The asset route also gained `nosniff` and stopped answering directory listings.

### 9. `transfers -html` refused after the work, and not at all on two outcomes — APPLIED

`cmdSquad` refuses before the optimiser runs and says why; `cmdTransfers` checked last, and
two of its four outcomes return early, so on a week the policy banked or found nothing the
command exited 0, wrote no file and said nothing. That is the failure the refusal exists to
prevent.

### 10. Three of this change's own documents said things the code contradicts — APPLIED

The documentation review found the most uncomfortable class: claims written by the author of
the change, about the change, that the change does not support.

`README.md`'s `serve` row said the token gates the write actions and overrides live in a
session cookie. **Nothing in the shipped client posts anywhere** — a lock changes the page
in front of the reader and a reload discards it, so neither store is reachable. The row now
says that, in the user's own vocabulary: changes are meant to be *saved to the session*, and
`-persist` puts them in `config.json` instead; "written" implies the file, and only
`-persist` writes one.

`internal/viewmodel`'s `Override.Session` comment said it "drives whether the delete button
is live". Nothing reads it: the delete control renders on every override including
config-sourced ones, and removes a row from a JavaScript array while the model goes on
applying it. `Session`'s own comment said the client "is told which store is live and says
so" — it is told and does not say so.

`internal/webui`'s package comment said every number the app displays arrives as JSON. Four
surfaces still contradict it, and the comment now names all four rather than restating the
intention.

Also applied: `cmdServe`'s doc still described the page `squad -html` writes; two comment
blocks were orphaned when their functions were deleted, one still explaining an
enhanced-response path that happens nowhere; `internal/present`'s package comment still
advertised the self-contained file; `WatchPageSize` and `WatchCap` described a served page
and a static export that no longer exist, and bind nothing on the product path;
`docs/architecture.md` omitted both new packages; and `AGENTS.md`'s standing rule cited
`TestTheClientHasNoAuthenticatedSurface` as guarding the security class when it is scoped to
`internal/fpl` and says nothing about `serve`'s inbound listener — which this change added a
route to.

Three labels misstated what a number is — "xPts this GW", "projected GW n", "Ranked by
projected points for GW n" — about a figure that is an average over the horizon. The rail
lets the reader change gameweek, the figure does not move, and the label promised it would.

And the player sheet's panel headed "How the number is built" printed points-per-90 then
`× minutes/90` and a total: the exact expression removed from `xpFor` for being wrong, and
one that does not produce the total under it. It shows the model's inputs as inputs now, and
the model's own figure as the answer. Reproducing the real expression there would be the
same mistake again.

## Declined, with reasons

**`present.Render` and the helpers behind it were not deleted.** The approved plan said to
remove it. It is what `internal/present`'s page tests drive, and those tests are the only
coverage `pageTmpl` has outside the replay views, so removing the entry point deletes the
coverage with it. Triaging seven hundred lines of tests did not belong in the change that
moved the product onto a different renderer. `present.HTML` is gone; `Render` survives with
the reason in the source.

**The self-XSS in the market search box** (`${S.q}`) was escaped anyway, though the reviewer
ranked it theoretical and was right to: `S.q` is only ever set from the reader's own
keystrokes and is never read from the URL. Escaped because an unescaped sink is a trap for
whoever makes the search shareable.

## What could not be checked here

**The container image build.** No runtime is installed, so the `COPY . .` → `go build` path
could not be exercised. `.dockerignore` excludes `*.html` and two embedded assets are HTML;
they survive only because Docker's `*` does not cross a `/`. A test asserts that property
against Docker's own matching rules instead, verified by adding `**/*.html` and watching it
fire — but the image itself is unbuilt here and CI is the first thing that will run it.

**Any gameweek but the first.** The fixture is pinned to the committed GW1 capture, which is
what makes it deterministic and what made finding 2 invisible. The fix is a code fact, not a
measured one; a mid-season capture would test it and none is committed.

**Whether the design is faithful.** The 28 screenshots shipped with the handoff are a
conformance reference for a human to read, not a gate — they were rendered from mock data
over `file://` with CDN fonts, so a machine comparison against them would need a tolerance
loose enough to catch nothing.

## Owed

- The landing page's inline `<style>` block, which is why `style-src` still allows inline.
- **Three client surfaces still compute**: the formations rail picks and totals its own
  eleven on a plain sum where `analysis.bestFormation` maximises sum + captain + vice and
  honours locks; the two captain fallbacks apply their own rule; `Per £m` re-divides a ratio
  the model carries as `ValueScore`. Named in `internal/webui`'s package comment as owed
  rather than left to be rediscovered.
- The README's hero screenshot is of the retired renderer. Its attribution now says so; the
  shot itself wants retaking from the new application.
- `present.Apply`, `WatchPageSize` and `WatchCap` bind nothing on the product path.
- The write path is unreachable from the application: `/action`, the token, and the session
  override store are live server code with no client caller. That is stage two.
- `/api/state` runs `bestPlanForOwnedSquad` on every request and drops the result, along
  with the brief and the analysis. It is the most expensive discarded stage in the build.
- Per-player, per-gameweek projections, without which the rail cannot move honestly.
