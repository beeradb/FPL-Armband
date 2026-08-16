# The second rebase, and the branch's work restated after main moved under it

Covers the rebase onto `1495bc6` and the two commits that follow it. **This record does not replace
`reviews/2026-08-14-cfe257f/review.md`** — that is the substantive review of the xPoints instrument
and the gate arm, written against the same content, and it survives the rebase with its commit
renamed. This one exists because the gate diffs committed history and that history was rewritten.

⚠️ **First-party.** No new code was written for the rebase; what changed is where documents live and
which commits they sit on. The reviewed work is unchanged and its review stands.

## What main did, and why the branch had to move rather than merge

Five commits landed while this branch was out, one of which — `2bf6018` — **moved the research
record and the design documents out of the repository**. `docs/` is now reference only: eight files,
each describing something that ships. The nine research notes and five design documents are no
longer reachable from a checkout, and 34 links into them were deleted rather than repointed.

That collides with this branch head-on, because most of its commits edit `docs/notes/`. Every one
produced a modify/delete conflict. **All resolved by accepting main's deletion**, which is the
policy and not a convenience: the findings live in the private store, and `CLAUDE.md` keeps a
verdict line per finding — which is the thing that actually stops an idea being rebuilt.

`docs/decision-scoring-design.md` was new here and so did not conflict. It would have landed as a
ninth file describing something that does *not* ship, so it was moved to the private store whole and
delinked from `TODO.md`. **`docs/` is back to eight.**

## ⚠️ Two of my own tools were wrong in ways that destroy work

Recorded because both are silent, and both are the shape this project keeps paying for.

- **`.git` is a FILE in a worktree.** A script that checks `.git/rebase-merge` to decide whether a
  rebase is finished reports "finished" on the first iteration of a rebase with 29 commits left. It
  did, and for several minutes I believed 37 commits had collapsed to 7. Ask git for the path
  (`git rev-parse --git-path rebase-merge`) rather than assuming the layout.
- **`git rebase --skip` DROPS a commit.** Using it as a general fallback for "continue failed"
  silently loses work — and it did: two scripts came back as *deleted by us* because the commit that
  created them had been skipped. Caught by checking what survived rather than by any error.

**Both fixed, then the rebase was aborted and restarted from the recorded pre-rebase SHA** rather
than repaired forward. A rebase repaired forward from an unknown-good state cannot be verified; one
restarted from a known SHA can. All 36 commits present afterwards, working tree clean.

⚠️ **A third failure worth naming: "continue failed" is not a failure.** `git rebase --continue`
exits non-zero when it stops at the *next* commit's conflict, which is the normal path. Treating it
as fatal aborted a rebase that was working.

## What survived, checked rather than assumed

`internal/analysis/xpoints.go`, `internal/backtest/gatexpoints_diag_test.go`,
`stats/xpoints_permove.py`, `stats/gate_recovered_fraction.py`, the banked cells and inference at
`stats/snapshots/2026-08-14-gatexpoints/`, and `reviews/2026-08-14-cfe257f/`. The three conflicts
that were *not* modify/delete — two in `CLAUDE.md`, two in `TODO.md` — were resolved one at a time
with the decision stated per block, and every surviving pointer into `docs/notes/` was rewritten to
bold text rather than left dangling.

## The result this branch carries, unchanged by the move

**A transfer gate judged on realised underlying is worth +85.3 a season** (+2.246 pts/gw, 36 cells,
six seasons, `POLICY`, CR2 SE 0.471, df 5, t 4.76, Holm 0.0050) against this comparison's own
threshold of 46.0. The recovered fraction against a gate perfect on realised points is **0.645,
Fieller 95% CI [0.426, 0.835]** — it **rejects equivalence** and rejects 0.89, and **cannot reject**
the pre-registered 50%.

## Gates

`go build`, `go vet`, `go test ./...` clean at this commit. Snapshot regenerated at `8f17202` and
the three the rebase orphaned removed, per main's own precedent at `6a75a65`: figures differ from
the predecessor by the commit stamp alone, `constants.csv` byte-identical. `origin/main` is an
ancestor, so **`git merge --ff-only` is available**.

**Nothing shipped changed** on this branch. No scoring constant, config field or objective term
moved, across either rebase.

## Redaction note — 2026-08-16

Two phrases above were edited after this record was filed. Both named a private store this
repository may not name; they now read **the private store**. The conflict resolutions and
the file counts they describe are unchanged.

⚠️ **Cleaned rather than exempted.** The standing exemption for already-committed
disclosures is a grandfather clause over an enumerated set; this was found afterwards. The
cost — amending a dated attestation — is acknowledged, which is why this note exists rather
than the edit being silent. **No finding was altered.**
