# Review — the squad page rebuild

`fplagent squad -html` grew two views behind the roster: **why this eleven** (the standing
overrides binding it, where the model states it is blind, the transfer gate) and a
**watchlist** of the best players outside the fifteen. Two defects in the existing page
were fixed on the way, and the two reviews of that work were applied.

Four commits on `squad-page-rebuild`, off `82fc8e0`:

| | |
|---|---|
| `fcc9918` | give the repo's review agents the private-store tools their briefs already assume |
| `3fd41d7` | the rebuild |
| `09eda4b` | apply the code and security reviews |
| `c1d4ae5` | bank the model half |

**History note.** This work was originally done on `docs-accuracy-agent`, which had drifted
109 commits behind `main`. Rather than merge it, the three substantive commits were
cherry-picked onto a fresh branch off `main` and the two record commits were **regenerated
rather than carried**, for reasons that are the point of both guards: the old snapshot was
diffed against a baseline that predates the per-position xG/xA conversion scale, and the old
review record used the retired sha-in-the-directory-name mechanism and had no `key.csv`. The
old branch also carried a second, independent implementation of
`TestNoTrackedMarkdownCitesAWikilink`, which `main` already has from `c47b86c`; cherry-picking
drops it by construction. `docs-accuracy-agent` is superseded and should be deleted.

## The invariant, which is worth more than either review

The skill's opening rule is to ask what quantity the change must **not** move before
dispatching anyone. Here that is every model figure: the only scoring-path edit adds a
*reported* field and substitutes it into the expression it already multiplied by.

`stats/snapshots/2026-08-15-09eda4b/` reruns 555 rows — calibration drift, the clean-sheet
Poisson fit, the prediction benchmark and its coverage, next-five predictors, the sixty-minute
threshold, defcon bias, team blend, transfer error — against `main`'s own newest snapshot
`2026-08-15-7cbe87f`. **Only `stamp.commit` moved.** Nothing else, to the recorded precision.

That is measured, not argued, and it is measured against the *right* baseline: `main` carries
the per-position conversion scale, so the same claim made from the old branch's base would
have said nothing about the merged tree.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-code-review** | yes | touches `internal/analysis`, `internal/agent`, `internal/config` |
| **fpl-security-review** | yes | `internal/agent`, and the page newly renders FPL-supplied and human-written strings into a file opened in a browser |
| **fpl-stats-review** | **no** | it reviews *claims* — "constant C should be K", "feature F is worth +N". None is made here: no constant moved, no points figure is asserted, and the snapshot above shows no figure moved either |
| **fpl-findings-audit** | **no** | `CLAUDE.md` and `docs/` are untouched; it audits the record, and the record did not change |
| **fpl-run-review** | no | no live run wrote config |
| **fpl-season-maintenance** | no | the four hand-maintained lists are untouched |
| **fpl-docs-accuracy** | no | no `docs/` change — but see "not checked" |

Both ran concurrently and read-only. No replay was running.

## Findings applied

Ranked by how misleading the state was.

1. **The why-card claimed `FixtureLoad` was applied to the score when at the shipped horizon
   it is not.** `Score` multiplies by it only when `FixtureLoadInScore()`, which at the
   weekly-only setting means horizon 1; the page runs at the scoring horizon. Listed beside
   congestion and role — which always apply — a `×2.00 fixtures per gameweek` line told a
   reader a double gameweek was priced into the headline number. `internal/agent` already
   exports `FixtureLoadInScore()` for exactly this question; the card was the third consumer
   and the only one not asking. Now threaded from the caller, both branches tested.
2. **Two defects in the page as it already shipped**, both the same shape — a template
   printing a field whenever it is non-empty, where the neutral value is not empty:
   `htmlPlayer` never set `Opponent`, so the squad table printed **"blank" in the bad colour
   on all fifteen rows**, telling the reader every player he owned had no fixture; and
   `RotationRisk` is never empty, so `.risk` rendered **"nailed" — good news — in the same red
   as an injury on eleven of fifteen rows**. The pitch renderer made the second mistake first
   and `isRisky` fixed it there; this was the second renderer to make it.
3. **Research-target rows bypassed `newCard`** and so carried no badges. Not cosmetic:
   `ResearchTargets` ranges the whole unfiltered pool and its first category selects flagged
   defenders and keepers — exactly where an *excluded* player lands. Saliba rendered there
   unmarked, on the view whose other half exists to explain his absence. Six rows affected in
   the GW1 build.
4. **The week table carried a third hand-rolled badge block** and had already drifted, omitting
   the penalty and availability badges. It now calls the shared template.
5. **`Flag()` reported a never-verified override as though it had been checked.** `checkAge`
   falls back to `SetOn`, so "never checked, set 12 days ago" and "checked 12 days ago" both
   rendered `CHECK 12d` — beside a meta row reading `never (12d)`. `NeverChecked` now carries
   the distinction and sorts ahead of merely stale.
6. **The excluded cards dropped "would score" at exactly 0,** the commonest and most
   informative case: an injury exclusion zeroes availability and so zeroes the score. Saliba
   now reads `would score 0.00`.
7. **The watchlist coloured against zero rather than against the gate,** so every positive
   candidate was drawn as an upgrade on a page that refuses a transfer below `min_gain`. Three
   states now, and the caption states how many clear it.
8. Dead branch in `Oldest()`; a hard-coded class string; the override count computed twice for
   one screen; the "due a look" prose stating one of the two conditions `needsCheck`
   implements; a comment describing a bar the watchlist does not draw.
9. **Security:** the budget warning interpolated the FPL client error, which formats as
   `GET /entry/<id>/history/: …` and on the status branch carries 200 bytes of FPL's response
   body, into a page the user hands around. Full error goes to stderr now.
10. **Security:** the self-containment test scanned for `https://` anywhere in the byte stream.
    The page now renders injury news and override reasons, and the live config cites news sites
    by name — a URL in prose would have failed it with "the page reaches outside itself",
    which is false, and the natural response to a red test with a wrong premise is to weaken
    the only guard between this page and an external fetch. It asserts on emitted markup now,
    and the fixture carries a URL in prose.
11. **Security:** the page is written `0600`. It grew from a team sheet into something carrying
    every override reason and FPL's injury notes.

Plus the convention item both reviews raised: `AvailabilityFactor` reaches `agent.playerRow`,
as a **pointer** with `omitempty` rather than a bare float — `omitempty` drops `0.0`, and
`0.0` is the one value the field exists to carry.

## Findings declined

**The watchlist applies no pool filter** (code review #9, correctly marked PLAUSIBLE).
`watchlistFor` ranks raw `AllMetrics()`, skipping only owned and excluded players, while
`Optimize` additionally applies a total-minutes floor and `MinExpectedMinutes` with a
bench-fodder exemption. A candidate below the floor can therefore render as an actionable
transfer neither solver would produce.

Declined **for now**: the only faithful fix needs the optimiser's own eligibility rule, which
is unexported and carries an exemption clause. Re-implementing it here would be one quantity
with two implementations, and a wrong copy would silently *hide* candidates, which is worse
than the symptom. The honest fix is to export the predicate from `internal/analysis` and call
it — a change to that package's surface, not something to fold into a rendering branch
unreviewed.

**Recorded rather than dropped:** the watchlist may list a player the optimiser cannot pick.
Magnitude unmeasured.

## Not checked on this harness

- **Nothing here was measured in points, and nothing claims to be.** No sweep ran and none was
  owed — the change moves no model quantity, which the snapshot demonstrates rather than
  asserts.
- **The page's own prose has no reviewer.** `fpl-docs-accuracy` watches the root README and
  `docs/`; the page now carries several hundred words written in this branch — the corrections
  list, the blind-spot bullets, the watchlist caption — that nothing checks against the code.
  Real gap, not closed here.
- **Four of the eight design sign-off checks are visual** and were settled by screenshots from
  a headless browser against live GW1 data, in all three theme states and at 320px — not by any
  test. A future change can break the dark palette or the narrow reflow with every test green.

## Method note

Neither reviewer's report was taken as a finding; each was verified first, and one did not
survive. The code review asserted the transfer gate is 2.00 pts/gw — that is
`free_transfer_value`. The gate is `min_gain_for_transfer` at **0.40**, and at the shipped
value the `+0.81` candidate it named as a false positive clears correctly and is green. The
underlying point — a source comment citing a figure that does not reproduce at shipped
config — was real, and the comment was fixed.

## Redaction note — 2026-08-15

One cell of the commit table above was edited after this record was filed. Its text named a private
store this repository may not name; it now reads **private-store tools**. What the commit did is
unchanged — `fcc9918` widened six review agents' tool allowlists — and no finding was touched.

This edits a dated attestation, which the standing exemption for already-committed disclosures was
meant to prevent. The user ruled that exemption a grandfather clause over an enumerated set only,
and this file was found afterwards, so it is cleaned at the acknowledged cost of amending a dated
record. The note exists so the amendment is itself attested rather than silent.

⚠️ `fcc9918`'s change has since been undone: those tool names referred to a server that no longer
exists on any machine here, and they were removed as vestigial on `tier-0-queue-sweep`.
