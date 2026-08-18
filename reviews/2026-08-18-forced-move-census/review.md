# Review: the forced-move census

**What was reviewed:** `TestDiagForcedMoveCensus` (archive rows only, no replay,
no cells, no threshold) and its finding `stats/findings/2026-08-18-forced-move-census.md`,
in three passes as the measurement grew: the initial count + finding
(`c7d45b1`), the cost-half extension after the user's first note (`6cdda24`), and
the appearance-shortfall channel after the user's second note (`efc348b`). Commit
range `origin/development..bab2e3a` on branch
`count-availability-breaks-in-congested-weeks`.

**Reviewers:**

- **fpl-stats-review — ran twice.** First on `c7d45b1`+`6cdda24`: re-ran the
  diagnostic and independently re-derived every figure in Python over the same
  CSVs (all reproduced exactly); verified the counting logic edge by edge
  (double-leg summing, was_home attribution, the 2025-26 duplicate rows, phantom
  matches, blanks inside the before-window, season-end truncation, the Van Dijk
  ACL case); reported the prose findings below. Then on the `efc348b` delta:
  verified the legs60 accumulation, the superset relationship, and reported the
  valuation and loop-bound findings below.
- **fpl-findings-audit — skipped, by triage.** No `AGENTS.md`/`CLAUDE.md` edit;
  results live in the research record's notes and the finding per the user's
  standing direction.
- **fpl-code-review — skipped, by triage.** A DIAG-gated archive diagnostic and a
  finding; no shipped path changed (the diagnostic is skipped unless `DIAG=1`).

**Findings, ranked by how misleading the current state was:**

1. The title and intro carried the pre-extension "no mass" verdict and
   contradicted the finding's own cost half.
2. The appearance valuation charged 2 points per sub-60 leg while describing it
   as "the second appearance point" (worth 1) — overstated levels by ~35-40%;
   the ratio was robust (conservative), the levels were not.
3. The shortfall loop inherited the break loop's `i+1` bound and silently dropped
   the final played week of every sequence (1,326 single weeks, 0 doubles).
4. The premium-bracket interpretation asserted placement ("not where the
   multiplier prices it") on counts with no per-bracket exposure.
5. Minor: "about 3 SE" for a 3.4 figure; the ppm explanation presented a
   contributor as the explanation; the sibling nailedness range (0.66-0.82)
   spanned seasons outside the census's six; two doc-comment nits.

**What was applied:** all of them, verified against the banked output before
editing. 1: title/intro rewritten to the two-half picture, then to the
three-channel picture. 2: switched to the exact FPL valuation (2 for a leg never
entered, 1 for a 1-59-minute leg) — the headline moved 2.07 → 2.09, in the
direction that makes the asserted-constant comparison stronger. 3: loop bound
fixed to `i < len(seq)` with a comment naming why the break loop differs. 4:
bracket interpretation softened to "consistent with". 5: all applied, including
the six-season sibling range (0.72-0.82).

**What was declined:** nothing. The reviewer's "bound or exact" choice on the
valuation was resolved in favour of exact, which is the stronger disclosure.

**What could not be checked on this harness:** the net value of the channels is
deliberately not a valuation — the burden is an upper bound, the appearance
valuation is exact but covers appearance points only, and no points claim is made
or owed. The proxy's named misses (injury-then-sale, GW1-GW3) stand as recorded.
