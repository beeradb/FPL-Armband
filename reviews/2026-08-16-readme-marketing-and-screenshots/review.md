# The README landing page, and screenshots of real output

## What was reviewed

Three things, on top of the documentation rewrite recorded in
`reviews/2026-08-16-humanising-the-user-facing-docs/`:

- **`README.md` restructured** so it leads with what the tool does rather than with an accuracy
  table, and so the accuracy figures are interpretable when they do appear.
- **Four screenshots captured** from `armband -html squad.html squad` on live data, three of them
  placed in the page, under `docs/images/`.
- **The project banner added** at the top of the page, replacing the `# FPL Armband` heading.

It also carries a merge of `4d05c25` from `main` — *"Stop the snapshot generator stamping the
branch it ran on"* — which touches `cmd/armband/snapshot.go`, `internal/snapshot/render.go` and adds
`internal/snapshot/branchstamp_test.go`. **That commit is not reviewed here.** It arrived already on
`main` from another session, it is unrelated to this change, and it was merged in only to satisfy
the merge gate's "0 behind" condition. It is named because it is why `internal/snapshot` shows as a
moved tree in this record's key.

## The problem being fixed

The page opened, before it had said what the tool was for, with a table containing `0.427`, `0.330`
and `0.311` under the heading "ranks players within a gameweek". Nothing said what the scale was,
whether 0.427 is good, or what the two comparison rows represented. The owner's words: *"We say a
lot of things about accuracy but we don't contextualize it at all. What is the baseline we are
comparing to? I don't think we need to lead with accuracy at all."*

## Which reviewers ran

| reviewer | status |
|---|---|
| a product-marketing pass | **ran**, twice — once to restructure the page, once to place the images. Proposals only; every factual claim was checked against source before it stayed. |
| my own verification | **ran.** Detailed below, because a marketing pass is exactly where an unsupported claim gets in. |
| fpl-docs-review | **not re-run.** It reviewed the documentation rewrite this sits on top of. This change adds no new claim about the model — see the verification below — so a second full pass had nothing new to read. |
| fpl-findings-audit | **skipped**, as in the companion record: a single reviewer was requested, and this change alters no verdict. |

## What I verified rather than took on trust

**Every number on the page is sourced.** `0.427`, `0.330` and `0.311` appear at
`docs/accuracy.md:118-120`; `0.41`, `2.57` and `1.03` in the same table; the "29% better" figure at
`docs/accuracy.md:3` and `:125`. No number on the README is absent from `docs/`.

**No puffery, checked mechanically.** A scan for `revolutionary|powerful|seamless|unlock|supercharge|game-chang|cutting-edge|best-in-class|effortless|blazing` and for exclamation marks returns nothing. Emoji count is zero.

**No invented benchmark.** The page states the rank-correlation scale — 0 is no better than random,
1 is perfect — and then says in words that football is noisy and a perfect predictor is impossible.
It quotes **no ceiling figure and no state-of-the-art comparison**, because no honest one exists in
this repository. This was the single largest risk in handing the page to a marketing pass and it is
the thing most worth re-checking if the page is edited again.

**One overclaim was caught and corrected.** The draft said the "form column" baseline *is* what FPL
shows in the game. FPL's own Form statistic is points per match over the last 30 days; the benchmark
here is the mean of a player's last five gameweeks. Close cousins, not the same quantity. The page
now says "the same idea as the form figure FPL itself shows" and the table row reads "recent form
(last five gameweeks)".

**The screenshots are real output, and each was looked at.** All four PNGs were opened and inspected
before use — framing, clipping, empty space. They come from a live run against the public FPL API,
not from fixtures or mockups, and the page says so once.

**The honest material survived the marketing pass intact.** The line-ball-with-a-moving-average
caveat, the constants that cannot be shown to be optimal, the football-versus-points distinction,
and both safety claims (no authenticated write path; `review` persists overrides that bind later
free runs) are all still present, and the second is now under its own heading rather than floating
between diagrams.

## What was applied

- Page reordered: pitch → hero screenshot → architecture diagram → quick start → the two things to
  know before a paid run → the public team → accuracy → commands → the rest.
- Accuracy section rewritten to explain the scale and to name the baselines as what a manager
  actually reasons from today.
- `docs/images/squad-eleven.png` near the top; `docs/images/watchlist.png` in the accuracy section,
  where it makes "a transfer is a question about order" concrete; `docs/images/why-this-eleven.png`
  closing the known-limitations list, where the tool printing its own blind spots is the argument.
- The banner replaces the `# FPL Armband` H1. Nothing in the tree referenced that heading.

## What was declined

- **`docs/images/week-by-week.png` was captured and dropped, and the file deleted.** The page has no
  section where planning ahead is the subject, and a fourth screenshot beside three mermaid diagrams
  tips into a gallery. Committing an unreferenced binary is clutter. The framed HTML that produced
  it is kept outside the repository, so it regenerates in seconds if a home appears.
- **Recolouring the logo to the application palette.** Raised because the banner is navy and blue
  while the diagrams and the HTML output are burnt orange and teal, so the page currently carries two
  palettes. The owner's decision was to leave the logo and revisit the HTML palette later. Recorded
  because the mismatch is real and a future palette pass should know it was noticed, not missed.

## What could not be checked on this harness

- **How the page renders on GitHub.** Image paths were verified to exist on disk with the exact
  spelling used, and the mermaid and link guards pass, but nothing here renders Markdown. The
  `<img width="520">` on the banner is standard GitHub-flavoured Markdown and is **asserted rather
  than verified**.
- **Whether the screenshots stay accurate.** They are a point-in-time capture of gameweek 1 of the
  2026/27 season with no `entry_id` configured. They will drift as the season moves — that is
  inherent in showing real output rather than a mockup, and it is the trade that makes them
  credible. No guard watches them.
