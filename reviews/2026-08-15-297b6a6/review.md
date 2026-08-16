# Review — cutting TODO.md to a titles-only queue, and the priority standing rule

**Commit range reviewed:** `69fe960..297b6a6` (branch `todo-to-<redacted>`).
Two content commits — `6f43c76` made the change, `297b6a6` applied this review.

## What changed

`TODO.md` went from 5,301 lines to open-item **titles only**; its bodies were moved out of
this repository. `CLAUDE.md` gained one bullet at the top of *Standing rules* naming four
classes that take precedence: **security, performance, velocity, then a model or scoring fix**.
No Go source changed in `6f43c76`; `297b6a6` touches one test comment.

The queue was vetted against this tree before the cut. Of 80 open items, **23 were removed**:
most described work already built and tested here, or rested on a premise the record has since
retracted; three restated another item. ⚠️ The finer three-way split quoted in the first draft
is **not reproducible from this checkout**, and the file now says so — the bodies that
justified each removal left in the same commit.

## The invariant came first, and it is the strongest evidence here

Per the gate's own instruction: **what must this change not move?** Any scoring figure or
replayed cell. `6f43c76` touches two Markdown files and no Go source, so nothing *can* have
moved — `git diff --stat` is the whole proof, and it is stronger than any reviewer reading the
diff. `go build`, `go vet` and `go test ./...` all pass, including the 147 s `internal/backtest`
run.

Two guards read `TODO.md` directly and both pass **for the right reason**, which was checked
rather than assumed: no literal in the new file matches any of the twelve `retractedFigures`
entries with or without its context word, so the pass is not vacuous.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-findings-audit** | yes | triage row *"`CLAUDE.md`, `docs/` — the record only"* |
| fpl-code-review | no | no scoring or agent code changed; the only Go edit is a comment |
| fpl-stats-review | no | no measurement made or re-judged. ⚠️ One deferral is owed to it — see below |
| fpl-security-review | no | no change to `internal/fpl`, `internal/agent` or config persistence. The security *prose* was audited by findings-audit and corrected |
| fpl-run-review, fpl-season-maintenance | no | no live run; the four season lists are untouched |

## Findings, ranked by how misleading the state was

**All ten were verified independently before being applied** — the skill's warning that a
report is a set of proposals, not findings, and this project has had a reviewer misattribute a
movement before. Every claim below was re-checked against the source.

### Applied

1. **The `reconstructStarts` closure was wrong on all three of its claims**, and it is the
   entry `CLAUDE.md` points at *by name*, so it resolves a live pointer. It was not
   *replaced*: `reconstructStarts` is still defined (`internal/backtest/reconstructstarts.go:99`)
   and still runs in `Load` (`season.go:737`), with the harvest wired **ahead of** it —
   `startsrepair.go:53` says the repair covers "rows the harvest could not reach". "Byte-identical
   in all 36" dropped `CLAUDE.md:159`'s qualifier that **24 of the 36 are live**, which is this
   record's own *a byte-identical season under an intervention is not a tie* rule, broken while
   restating it. And it credited "the lineups **and minutes** oracles" where `CLAUDE.md:159`
   records `OracleMinutes` moving **none** of 36. Rewritten.
2. **"the stale-item class alone has cost at least four duplicate builds"** — the removed
   `TODO.md` body carried "⚠️ **Do not put a running total on this** … The numbers have been
   written four different ways. **What the class needs is a rule, not a count**". That warning
   was deleted by the same commit that promoted the count into the file read without choosing
   to. The source phrase also counts *times the rule paid* — cases caught **before** building —
   not duplicates shipped. Replaced with the rule.
3. **"a constant … that will never clear a threshold"** is stronger than anything recorded, and
   contradicts `CLAUDE.md` two pages up: the recorded wording is that "unresolved" is the
   **expected** reading, and the vice-captain fix **resolves 12.7** by making the comparison
   sharper. An absolute "never" would license exactly the deprioritisation the bullet's last
   sentence forbids. Replaced in both files.
4. **The security guard was described more broadly than it is.** `TestTheClientHasNoAuthenticatedSurface`
   bans a *superset* of the four tokens named — good — but its package-wide `http.MethodPost`
   scan uses `filepath.Glob("*.go")` relative to its own directory, so it covers **`internal/fpl`
   only**. "Both are blocked, and deliberately" read as a stronger guarantee than the mechanism
   delivers. Scope now stated. `CLAUDE.md`'s rationale also now says the class is **empty by
   construction**, since it justified top priority by a live write path that does not exist.
5. **6.2x is the microbenchmark, and its own commit prefers a smaller number.** `7c065c4`
   ("*The number worth trusting most was not a benchmark*") records **4.6x** on the mixed
   workload against 6.2x on `BenchmarkPolish/pool600`. Both now quoted, with the commit's
   preference named.
6. **Two item titles carried premises this same cut had killed.** "…parked here only because
   the file has ~230 bytes of headroom" — the budget went to 72 KB and headroom is ~1.2 KB, and
   *this commit spent 1,345 bytes adding a standing rule while an item below said standing rules
   were unaffordable*. And "six to convert below" pointed at a list that no longer exists, five
   of whose six were actioned by the cut. Both rewritten. **This is the titles-only risk
   arriving immediately**: a stale figure in a title has no body beside it to mark it withdrawn.
7. **`retracted_test.go`'s scope comment described a file that no longer exists** — "97 KB, most
   of it completed items". Updated, and it now records *why the guard matters more* at 9 KB of
   titles, not less.
8. **The three-way removal split was not reproducible.** Total verified (80 → 57 = 23, and
   11+3+9 = 23), but at least twelve removals carry an explicit retraction marker against a
   quoted "nine"; the categories genuinely overlap. Softened to the total plus the honest
   statement that the finer split cannot be checked from here.
9. **The recovered-team-news item was the only calendar-gated item in the file** and was dropped
   six days before its date. Its removal is defensible — the premise was retracted, since team
   news is now "measured and too small" at **+15 held against a threshold of 51** rather than
   unmeasurable — but the file gave no way to tell that from an oversight. Added to *Closed, but
   cited by name*, with the fence's own "should happen" comment preserved.

### Declined

- **Nothing was declined outright.** The one judgement deferred is finding 9's *substance*:
  whether the +15 team-news bound should be re-adjudicated is `fpl-stats-review`'s call, not
  this pass's. Recorded so the next pass does not re-raise it as a documentation defect — it is
  a measurement question, and the item is now visible again rather than silently gone.

### Verified sound, recorded so the next pass does not re-derive it

- **`1031 MB → 97 MB`, and "no slower"** — correct figures, correct direction, correctly
  attributed to the `go test` driver rather than the replay. `reviews/2026-08-11-89618ec/review.md:33`
  records 6:17 against 8:39 on differently-loaded hosts and concludes that removing the driver
  *did not cost* time rather than that it saved 2:22 — the new text does not overclaim it.
- **"GC-bound, not compute-bound"** — sourced to `7c065c4`: a CPU profile "almost entirely
  `runtime.scanObject`, madvise and scheduler wait", 6.73 M allocations → 852, bit-exactness
  pinned by the differential harness.
- **"11 to 34 points a season"** — the canonical range, used consistently across `CLAUDE.md:126`,
  `docs/replay.md`, `docs/accuracy.md` and `stats/README.md`. Only the "never" tail was wrong.
- **"A1 to A5 and B1 to B6 closed, only C1 open"** — verified item by item in the old file.
- **"Three more copies of the paired difference — closed, debt list empty"** — verified:
  `internal/snapshot/inference_test.go:481` reads `knownCopies := map[string][]string{}`, with
  the staleness branch still armed.
- **The retraction history is intact.** The cut removed none of it: `min_gain` 0.7 → 0.4, the
  unified-search tie, the 265-point argmax protection re-measured at −40, and the fixture-weight
  retraction all survive unchanged in *Closed lines*.
- **CLAUDE.md byte budget** — 72,856 B against 73,728 (`internal/snapshot/notes_test.go:305`).

## What could not be checked on this harness

- **Whether the 23 removals were individually correct** is checkable only by re-reading each
  item's body, and the bodies are no longer in this checkout. What *is* checkable, and was
  checked, is the code claim behind each removal — every "already built" verdict cites a
  file:line in this tree.
- **Whether the priority ordering is the right one** is not a measurable claim. It is a
  judgement, and the rule states its own caveat rather than leaving it to be rediscovered:
  precedence is not worth.
- **The velocity class has no measured payoff.** "A recurring manual step is paid every time" is
  mechanism, not measurement, and this record's standing scepticism about mechanism arguments
  applies to it as much as to a scoring term. It is *unmeasured* rather than unmeasurable — the
  cost of a duplicate build is countable if anyone chooses to count it.

## Redaction note — 2026-08-16

The branch name in the commit-range line above was edited after this record was filed. It
named a private store this repository may not name; it now reads `todo-to-<redacted>`.
⚠️ **It is shown as redacted rather than replaced with a plausible name**, because a branch
name is an identifier: substituting a different one would make this record attest to a
branch that never existed. The commit range is untouched and is the identifier that matters.

⚠️ **Cleaned rather than exempted.** The standing exemption for already-committed
disclosures is a grandfather clause over an enumerated set; this was found afterwards. The
cost — amending a dated attestation — is acknowledged, which is why this note exists rather
than the edit being silent. **No finding was altered.**
