# The merge of `origin/main` into `sweep-self-verification`, and the snapshot on it

Covers the merge commit and `02e4404`. The staleness guard correctly fired on **27 watched files**;
all 27 arrived from the other side.

## What came in, and where it was reviewed

`origin/main` moved 19 commits ahead while this branch was working, and the incoming work carries its
own records — `reviews/2026-08-14-d27c5c9/`, `reviews/2026-08-14-d55c7ac/`,
`reviews/2026-08-14-3f4bc9b/`. The bulk of it is the one-quantity-two-implementations audit
(fourteen sites, five diverged), the median-estimator collapse into `internal/stats`, the
`NoXG`/`NoXGC` split, and two CLAUDE.md budget repairs. **Re-reviewing it here would be re-reviewing
a reviewed branch.**

⚠️ Note that **this branch's own first half is already on main** — `c5f2203` merged the cells-schema
work and the `BlendRateK` low-side run. So the 14 commits under review here are only the second
half: the concentration screen, the forensics probe, the finishing measurement, and the xPoints
bound.

## The merge itself

**No conflicts**, which is worth stating rather than assuming, because both sides edited `CLAUDE.md`,
`TODO.md` and three notes. Verified after the fact that this branch's two `CLAUDE.md` claims survived
the merge intact: the `+24 is 2-3 cells` caveat on `FPL_TEAM_FORM` and the `BlendRateK` low-side
sentence are both present exactly once.

`CLAUDE.md` is **69,628 bytes against the 69,632 budget** — 4 bytes of headroom. ⚠️ That is not
comfortable and the next branch to touch it must cut first. It is under only because main's
`13a8222` did a budget repair that this merge inherited.

## The one thing worth checking, and it was checked

Main's incoming work touches `internal/analysis/metrics.go`, `squad.go`, `priorblend.go` and the
median estimator — all on or near the scoring path. This branch's only production change is the
`HoldCaptaincy` per-week fields. **The question is whether the combination moves any model figure,
and it does not**: the snapshot regenerated on the merged tree differs from main's own `6a75a65`
snapshot on **two lines, both the commit stamp**, across 560 figures.

That is a stronger check than either side alone, because it is taken after the interaction.

## Gates

`go build`, `go vet`, `go test ./...` clean on the merged tree. Snapshot regenerated and committed.

## What this branch is still owed

⚠️ **An independent review pass.** Every review record on the second half of this branch is
first-party — no reviewer agents were dispatched, per the session's standing instruction — and three
of the commits are **claims about the model and about the metric the record is measured on**, which
is the category that normally goes to the statistics reviewer *before* the work. Nothing in them
changes shipped behaviour, which is the only reason the ordering is recoverable. **This should not
merge to main as "reviewed" without that pass.**
