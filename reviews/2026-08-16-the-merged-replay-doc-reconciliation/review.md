# Reconciling `docs/replay.md` after two concurrent correction passes

⚠️ **Findings are named for what they are, not numbered.** An earlier version of this record labelled them `F1`-`F8`, which tells a reader nothing and forces a lookup for every cross-reference. This project's naming rule covers review findings as much as branches and sweep labels.

## What was reviewed

`docs/replay.md` at `5f5869e`, after it absorbed **two independent correction passes made within an
hour of each other by sessions that did not know about one another**, plus my merge resolution.
That is the state most likely to contain a contradiction, and the review was briefed to hunt for
exactly that rather than to re-read the page generically.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-docs-accuracy** | ✅ | `docs/` changed; and the merged state needed adversarial checking |
| others | ❌ skipped | no code changed — the diff is `docs/replay.md` only |

## The finding that justified the whole pass

**The wildcard/bench-boost sequence — the row described something FPL does not permit, and BOTH
passes wrote it wrong independently.** The page said a wildcard *"played in the same week as a bench
boost"*. `playWildcard` (`internal/backtest/simulate.go:2286-2297`) gates on
`cfg.plays(slotBenchBoost, gw+1)` — **the next gameweek** — and the comment three lines above states
it outright: *"FPL allows one chip per gameweek, so a wildcard can never **be** the bench boost week
— it prepares for the one after it."*

**Verified against source myself before applying.** Pass A wrote "in a bench-boost week", pass B
wrote "in the same week as" — so the merge could not catch it, because the two versions agreed on
the error. A reader planning a chip sweep from this row schedules the wildcard one gameweek late and
the arm measures nothing.

⚠️ **Handed over rather than fixed here**: `internal/backtest/replay.go:18` and the
`FPL_WC_IGNORES_BOOST` comment in `internal/snapshot/fingerprint.go` carry the same wrong spelling.
The doc copied it from them. One quantity, two spellings — `fpl-code-review`'s to settle.

## Applied

1. **The 97 MB provenance — the paragraph's explanation contradicted the claim it explained.** It said 97 MB
   "is not even the same statistic as the number the wrapper prints", then glossed 97 as "what the
   wrapped run cost" — which *is* what the wrapper prints, and 97 sits inside the stated 89-142
   band. Worse, **the provenance is recorded nowhere** and this page asserted it in *both*
   directions across two commits without new evidence. Rewritten to say what is provable (145 is the
   sampler's peak under `go test`) and to state plainly that 97's instrument is unrecorded.
2. **Two named three.** The sentence read "mind that two of these are not" and then listed three. Verified: `-blend`, `-blendlo` and
   `-blend-datastate` all record theirs as prose, and no `peak_rss=` line exists in any of the
   three. One word.
3. **The guard rail pinned the wrong figure**, saying "the 145 MB figure above holds
   only until some arm makes it false". By the page's own new paragraph 145 is the `go test`-era
   sampler number; the falsifiable thing is the 89-142 band. Repointed.
4. **The dropped `HOLD`-definition clause — the merge silently lost a true claim.** Pass A's autosubs row noted that `HOLD` is
   *defined* as "with autosubs and the vice-captain fallback applied", so the switch changes **what
   the metric means**, not just what it scores. Pass B's row is better on everything else and I took
   it wholesale; this clause was a real loss. Appended rather than reverting to pass A.
5. **The rotted parser count — a number went stale inside the warning about stale numbers.** My paragraph said "do not read this as
   'the other ten are all of them'", and pass B then added three rows. **Deleted the number** rather
   than re-deriving it, since it will rot again on the next row.
6. **The zero trap is a property of the PARSER, not of one switch.** `envDefaultAbove` defaults
   on any `<= 0` and is read by six switches; naming only `FPL_CS_XGC_FACTOR` implied the others
   were safe. `FPL_BLANK_RUN_MAX=0` — no window — reads as 3. Also moved `FPL_POS_MINUTES_SCALE`
   out of the "deliberate setting" pair, since `MID=0` **is** honoured and its defect is a typo
   defect.
7. **The row-dump memory bucket — two banked figures sat below the band with nowhere to belong.** 69 and 87 MB in
   `2026-08-16-blank-run-position/` sit between the "37" static probe and the "89" sweep floor;
   they are a row-dump diagnostic, not a grid sweep. Named as such so a reader grepping does not
   conclude the band is wrong.
8. **The undated drift warning — the merge lost the date and the method.** Pass B's drift warning said the table agrees
   "as this is written", which a reader cannot resolve. Restored pass A's date and method while
   keeping pass B's better sentence.

## Was taking pass B's side correct in each of the four hunks?

Asked explicitly, because I took their side wholesale on the judgement that their pass was deeper.

| hunk | verdict |
|---|---|
| drift warning | **B right on substance, A right on checkability.** A's date and method were a real loss — restored |
| the two switch rows | **Half right.** B's autosubs row is better and dropped the `HOLD`-definition clause; B's wildcard row is **wrong**, and so was A's |
| the 97 MB paragraph | **B was right to replace my "94 to 130", which was false at both ends** — the reviewer re-derived 89-142 independently. B's replacement then introduced its own contradiction (the 97 MB provenance) and a miscount |
| the memory-cap bullet | **B right, no loss.** |

## Pass B's claims I had taken on trust, now checked

All verified against source by the reviewer: the **89-142 band** (every `peak_rss=` line plus three
prose figures), the **per-repository-path slot keying**, the **`2026-08-16-anti-residual-gate`
self-disagreement** (117 in `console.txt`, 130 in `FINDINGS.md`), and — the sharpest of B's
corrections — that **`FPL_BUDGET_WEIGHT` does act on the shipped bespoke search** while
`FPL_MULTI_SURCHARGE` does not, because `moneyPts`' two call sites sit *after* the unified-transfers
early exit. **One did not survive**: the "97 and 145 are different statistics" claim is only half
supported.

⚠️ **This is why the check was worth running.** I took B's facts on trust *because* their pass
looked more thorough, which is precisely how an error propagates.

## Declined

- **Drafting the switch-lifetime diagram** the reviewer proposed. The gap is real — the page never
  says which mechanism a knob uses (process-start global / per-call getenv / `SimConfig` field /
  `Oracles` field), and that decides whether an arm may vary it safely. But the reviewer explicitly
  declined to draft one blind, because `TestMermaidBlocksAreWellFormed` is not a renderer and a
  green suite is no evidence the block draws. **Recorded as worth doing with a render step**, not
  dropped.
- **Fixing the two Go comments** carrying the same wrong wildcard spelling, and the
  `sweep.go:555` comment that still says `FPL_APPEARANCE_FIT` is "refused rather than
  half-applied" — the doc is now correct where the code comment is not, which is the inverse of the
  usual direction. Both are code changes owed their own review.
- **Plotting the 45 banked `peak_rss` values.** The reviewer's reason is right: one series with no
  comparison behind it, and plotting it invites the argmax reading.

## What could not be checked

- **97 MB's instrument.** Not recorded in the commit that introduced it. The page now says so
  rather than guessing a third time.
- **The statistical figures** in this document (39, 3.9-232, the MDE arithmetic). Out of the docs
  reviewer's remit; `fpl-stats-review`'s.
- **Whether the proposed diagram renders.** Not drafted.
