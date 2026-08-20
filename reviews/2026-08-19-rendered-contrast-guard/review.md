# The rendered-DOM contrast guard, and the fact that its first build measured nothing

**What was reviewed.** The working tree of `guard-rendered-text-contrast` against `origin/main`
`504c04f`: a new WCAG 1.4.3 contrast suite over the two served pages, a new browser-driving
primitive to support it, four CSS fixes it found, one CSS comment correction, and twelve
regenerated layout goldens.

**What the change is.** `internal/webui` had no contrast test of any kind, and
`typescale_test.go`'s own header records that a rendered one is owed for contrast because
"compositing and inheritance mean a stylesheet cannot answer the question". There is a second
reason, and it is the one this repository was actually caught by: both existing guards read
`assets/static/armband.css` and nothing else, while `landing.html` carries its own `<style>` block
and `app.js` sets styles inline.

So: a headless Chromium renders each screen, a probe injected into the page walks the rendered DOM
and reports — per run of visible text — the colour the glyphs are painted in, the colour actually
composited behind them, and the ratio between the two. Eight screens at two viewports, one of them
a true 390×844 phone. 4.5:1 for text, 3:1 for large text.

| file | what |
|---|---|
| `internal/webui/testdata/contrast_probe.js` | new. The instrument: the in-page walk, the compositor, and the five hard cases decided at the point each is taken |
| `internal/webui/contrast_test.go` | new. The rules, the screens, four guards over the instrument itself |
| `internal/webui/testdata/contrast_stacks.html` | new. Six painting stacks whose composited answer is arithmetic |
| `internal/browsertest/browsertest.go` | `Probe`, `FrameHTML`, `OuterFor`, `minDumpDOMWidth` |
| `internal/webui/visual_test.go` | the test server gained `/probe.js`, `/probe-frame`, `/stacks` and two opt-in page rewrites |
| `internal/webui/assets/static/armband.css` | `--band-hi`; the tab bar, the armband gradient and the scorebug label; `--line`'s comment 1.31 → 1.33 |
| `internal/webui/assets/pages/landing.html` | the same armband gradient value; one literal replaced by the token |
| twelve goldens | regenerated; every diff is one of the four fixes |

## Reviewers

| reviewer | ran | why |
|---|---|---|
| `fpl-code-review` | **yes** | the diff is ~950 lines of new Go and JavaScript that produces a pass/fail verdict. The recorded bug classes it hunts — silent no-ops, comparisons that never ran, one quantity with two implementations — are exactly the exposure of a new instrument |
| `fpl-security-review` | **yes** | not owed by the triage table, dispatched anyway: the change injects a `<script>` into served pages, injects a `<style>` that re-breaks a token, and builds HTML from a query parameter. All three are meant to be test-only and that belief wanted an independent check |
| `fpl-docs-review` | **yes** | the change writes load-bearing figures into code comments, and the paired research note is written record |
| `fpl-stats-review` | **skipped** | no measurement of the model, no constant fitted, no claim about points. The figures here are deterministic properties of a rendered document — a ratio either clears the floor or does not — so there is no threshold to judge and nothing for it to review |
| `fpl-findings-audit` | **skipped** | `AGENTS.md` and `docs/` are unchanged. Its subject is the research record's internal consistency, and nothing in the resident record moved |
| `fpl-run-review` | **skipped** | no live run, no config written, no transfer recommended |
| `fpl-season-maintenance` | **skipped** | none of the four hand-maintained season lists is touched |

⚠️ **A skip is a decision, so all four are named.** The one worth arguing about is
`fpl-stats-review`: the change does introduce constants (4.5, 3.0, 24px, 18.66px, 700, and the
tolerances in three guards). They are quoted from WCAG 2.1 rather than fitted, and the tolerances
are derived from 8-bit rounding, so there is no estimate for a statistical reviewer to judge. Had
any of them been chosen by trying values, that skip would have been wrong.

⚠️ **`internal/webui` is not in `ReviewWatchedPaths`.** `TestReviewCoversTheCurrentCode` therefore
would not have required this record at all; it is written because the review gate asks for it, not
because a test did. That gap is worth someone's attention — the entire served client, the only
public surface this project has, is outside the review key — and it is **not** fixed here, because
adding a watched path retroactively demands records for changes already on `main`.

## Findings, ranked by how misleading the current state was

### 1. The compositing walk ran the wrong way, so every run was scored against the page canvas — APPLIED

**Found independently by both `fpl-code-review` and `fpl-docs-review`, within the same hour, each
with its own reproduction. Nothing in the suite found it.**

`worstRatio` seeded the fold at the *bottom* of the painting stack and called
`over(top = accumulator, bottom = layer)`, so the first composite was a transparent accumulator
over the page canvas. The canvas is opaque, the "nothing below an opaque layer shows through"
early exit fired immediately, and the walk returned the canvas colour. Every run on every screen
resolved to `--bg`. `collectBackdrops`, the gradient stop enumeration and the whole of the
inherited-background case were dead code beneath it.

Reproduced before fixing: on `players-desktop`, 945 of 945 runs reported `bg #070b10`; a
three-element page with grey text on an opaque white panel over a black canvas reported a **pass**
at 4.689 for a pairing that renders at 3.95.

`--bg` is the darkest surface in this system and the inks are light, so the error ran in the
**flattering** direction and sixteen screens came back clean. The mirror case is worse and was
latent: `.card.iscap.tcap .shirt .bandc` is `#2A1C00` on a gold gradient and would have read about
1.19:1 for text that renders near 8:1 — no committed fixture renders a triple-captain card, so the
first one that did would have turned the suite red for a pairing that is fine.

**Verified before applying**: I re-read the fold, traced it by hand, and confirmed the early exit
fires on the first step. Fixed by folding from the text outward. Three further defects in the same
resolution came out of the same reviews and are fixed with it:

- **`opacity` was charged to the text twice.** It groups — element and subtree rendered into a
  buffer, composited once at that opacity — so a per-layer multiplier fades the background against
  the parent and then fades the text against the already-faded background. Two different colours,
  not a rounding. Nothing in the shipped design renders it today, which is why it was invisible.
- **A pseudo-element's own background was not in its stack.** `.card.isvc .shirt::after` read
  1.11:1 for a badge that renders at 6.55:1 — a false failure on the one element the probe's own
  comment singles out as its unique capability.
- **The scrim justification was backwards.** "Measuring the undimmed stack can only make the check
  stricter, never laxer" is false: compositing two colours through a common overlay moves both
  toward the overlay and the ratio toward 1. The decision to ignore the scrim stands on "nobody
  reads in that state"; it does not stand on a safety property. The clause is replaced with the
  true statement, because a comment carrying an argument owes what a comment carrying a number
  owes.

### 2. All three existing guards were blind to finding 1 by construction — APPLIED

- The **liveness arm** re-injects `--ink3` at the #64788A that actually shipped and requires
  failures. #64788A fails against `--bg` too (4.32:1), so it caught its 191 runs whether or not
  the resolution worked. Its own comment names the risk it could not see.
- The **two-implementation pin** recomputes the ratio from the pair the probe reports, so a probe
  resolving every background to one wrong colour agrees with itself perfectly.
- `minRunsPerScreen` counts runs; the viewport assertion counts pixels. Nothing asserted anything
  about *which surface* a run resolved to.

Added `TestTheProbeResolvesTheBackgroundThatIsActuallyPainted`: six documents whose composited
answer is arithmetic — opaque panel, painting grandparent, translucent layer, opacity group,
gradient, pseudo-element with its own background — each built so a wrong resolution lands at the
other end of the scale rather than slightly off. **Verified to bite**: re-inverting the fold fails
four of the six. ⚠️ Only four — the opaque-panel case passes under the inverted fold by
coincidence, so a guard built from the obvious case alone would have passed too. The doc comment
on the two-implementation pin now says what it does not cover.

### 3. Four contrast violations in the shipped design — APPLIED, none exempted

Found by the corrected instrument on its first honest run.

| ratio | element | fix | after |
|---|---|---|---|
| 3.40:1 | `.tabbar button[aria-selected]` — the phone's selected tab label | new `--band-hi` token | 6.02:1 |
| 4.34:1 | `.card.iscap .shirt .bandc` — the captain's "C" | gradient stop `#4A6BFF` → `#4361F5` | 4.92:1 |
| 4.34:1 | `.mini.cap .sh .arm` — the same armband on the landing page | same value | 4.92:1 |
| 4.30:1 | `.scorebug .gwlz small` — "NOW" | `opacity` .8 → .9 | 5.02:1 |

The tab bar is the one that matters: `--band` is a fill and a border everywhere else in the file,
the tab bar was its only use as text, and the failing run is the single word on a phone that says
where the reader is. `#6A85FF` was already the file's answer for the band on a dark ground — three
more literals, plus a fourth in `landing.html` — so naming it `--band-hi` and using it gives the
value one spelling across both files.

⚠️ **One of the four is the bound being conservative rather than a defect, and that is recorded
rather than smoothed over.** The gradient rule worst-cases over the stops, so the captain's "C" on
a 16px bar was charged the colour at the very top of the box, which the 11px glyph does not reach;
its true minimum is nearer 5.2. The 9px landing bar is the genuine one. Both were moved by the
same value because it is one hex serving one purpose, and because clearing the floor at every
pixel is a stronger guarantee than any sampling rule. If a gradient ever fires where the text is
nowhere near the worst stop, the answer is band sampling — not an exemption and not a design
change.

### 4. The "no letter or digit" exemption was much too wide — APPLIED

It exempted the sheet close control's only label (`✕`), the price column's heading (`£`), the
transfer arithmetic (`+`, `=`) and a null marker (`—`). Narrowed to an allowlist of three
separator glyphs. All four newly-covered runs pass — they now pass measured rather than by not
being looked at. The core case is kept: `.gatefoot .dotsep` is 1.44:1 and flagging it is precisely
the crying-wolf failure that earns a rule a blanket exemption list.

### 5. `t.Fatalf` from an HTTP handler goroutine — APPLIED

`withProbe`/`withBrokenToken` called `t.Fatalf` from inside the test server's handler. That runs
`runtime.Goexit` on the handler's goroutine, so the request never completes and the browser hangs
— surfacing as "published no probe result", which names neither the cause nor the file. Both now
return an error and the handler answers 500. Trigger is narrow, so this was diagnosis quality
rather than a live break.

### 6. `--line`'s comment stated a ratio the tokens do not have — APPLIED

`/* 1.31 vs --panel */`; recomputed from WCAG relative luminance, `#223039` on `#10171F` is
**1.3311**. No surface in the system yields 1.31, so it was a wrong digit rather than a
mislabelled pairing. `ce0d402`'s message says "lifted from 1.23:1 to 1.31" — the *from* is right,
the old `#1E2A35` is 1.2348, and that message is pushed history and stays wrong.

Rather than fix the digit alone, `TestTheTokenCommentsStateTheRatioTheTokensHave` now recomputes
every ratio stated on a token's own declaration line, in both the `a / b / c` and `N vs --token`
forms, at a tolerance of 0.005 — exactly the rounding error a two-decimal claim is allowed. It
found this one on its first run.

### 7. `--dump-dom` will not render a page narrower than 500px — APPLIED

Asking for 390×844 and reading `window.innerWidth` from inside the page gives **500×757**.
`--screenshot` has no such floor, which is why the committed phone goldens genuinely are 390 wide.
A phone measured at 500px keeps every `max-width:720px` rule, so the page still looks mobile and
nothing announces the difference — the same shape as the wrong finding this area already produced
by rendering a phone at a height no phone has.

Fixed with an iframe, which has its own layout viewport, plus an assertion that the page laid out
at exactly the size asked for. Without that assertion the whole phone half of the suite would have
been running at 500px and passing.

### 8. Security: no finding — RECORDED

`go list -deps ./cmd/armband` contains neither `armband/internal/browsertest` nor `testing`; the
production router is an exact-path switch that gained nothing; `servePage` sets `X-Frame-Options:
DENY` and a CSP without `unsafe-inline`, so the frame trick structurally cannot reach it. The
`?regress=` parameter is a lookup key into a fixed map, never a value. `http.ServeFile` is given a
path built from two compile-time constants and the request path is an exact mux match. The only
shipped change is the CSS. One theoretical note — `FrameHTML` did not constrain the URL scheme —
is closed anyway with an http(s) check, not because it was reachable but because a handler should
refuse input it has no use for.

## What was declined, and why

- **Fixing `.mini .arm`'s 7px on the landing page.** It is one line and it is a real violation of
  the floor `armband.css` states at the top of itself. Declined here because **this change does
  not guard type size**, and a fix with no guard is the pattern that put the defect back last
  time; the two scans still read one file, and extending them fires on a second selector
  (`.lockup .tagline`, 15px → 14px on mobile) that needs its own decision. Its *contrast* is
  covered by this change. Recorded as still open, with the instruction to extend the scan and the
  fix together.
- **Adding `internal/webui` to `ReviewWatchedPaths`.** See above: it would retroactively demand
  records for changes already on `main`. Flagged, not fixed.
- **Band-sampling a gradient at the text's own position.** The worst-case-over-stops bound fired
  once where the glyph does not reach the worst stop. Declined for now because the geometry —
  gradient angle, stop positions, projecting a rect onto the gradient line — is a second mechanism
  that can be silently wrong, and the alternative cost was one hex. Recorded as the correct answer
  if it happens again.
- **Narrowing the separator allowlist further, or by container.** The reviewer offered "never
  exempt inside a `button`/`a`/`th`" as an alternative to the glyph allowlist. The allowlist is
  narrower and needs no second concept.
- **Putting the 500px trap in `AGENTS.md`.** It is recorded at the point of use and pinned by an
  assertion that fails loudly, which is the standard that file sets. The resident record has a
  size budget and this does not need it.
- **`landing-mobile.png` from the first `-update` run.** It came back rewritten on a run whose
  change could not have touched it — exactly the trap `visual_test.go`'s flag comment warns about
  — and was reverted. The twelve goldens that ship are each explained by one of the four CSS
  fixes; the bounding box of every diff was checked against the element that moved.

## What could not be checked on this harness

- **The leak scan's shipped scanner cannot run locally.** `LEAKSCAN_PATTERN` is a CI secret and
  `scripts/leakscan` correctly refuses to report a pass without it. All three channels were
  scanned by hand — diff body, commit message, branch name — and **a hand scan is weaker than the
  scanner**: it checks the patterns a person thought of. The `pre-receive` hook remains the real
  gate.
- **Whether the four violations are the complete set.** The suite covers what two committed state
  fixtures produce. A screen neither reaches is unmeasured, and the triple-captain card is a live
  example of one.
- **Timing.** The sixteen cases took 12-19s on a quiet machine and about 60s under load. Any
  affordability claim needs the machine state beside it.
- **Whether the corrected gradient bound is right at the pixel.** The glyph-versus-stop reasoning
  above is geometry on paper, not sampled pixels.

## Verification

Every proposal above was checked against the code before applying, and two were checked by
reintroducing the defect: the inverted fold, to confirm the new resolution guard fails on it, and
the `--ink3` injection, which still catches its runs. `go build ./... && go vet ./... && go test
./...` is green; `gofmt -l ./internal ./cmd` is empty.
