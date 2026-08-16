# `stats/findings/` — the narrative layer, retained when the series leaves

One file per banked sweep, named for the snapshot directory it came from. These were
`stats/snapshots/<dir>/FINDINGS.md` until 2026-08-16.

## Why these stay when the rest of the series goes

The snapshot series is being removed from this repository and its history. **These files are the
exception because they are the only place a retraction NARRATIVE exists inside a checkout.**

`AGENTS.md` is **verdict-only by design**: a stale claim is *deleted* there rather than annotated,
with no before/after accounting. `2026-08-15-gatescaled.md` states the condition that makes that
safe, in its own words:

> a verdict-only resident file only works if the thing it stopped carrying lands somewhere a
> checkout can still reach.

⚠️ **Deleting these would break exactly that condition** — the withdrawn wording would land nowhere a
checkout can reach, and the resident file's terseness would stop being a design and start being a
loss.

## The pre-registrations are here too, and they are the strongest case of all

Five `*-PREREGISTRATION*.md` files, retained on the same argument and then some.

⚠️ **A pre-registration's entire value is that it existed BEFORE the run.** It is the only thing
separating a prediction from a story told afterwards, and this project's standing rule is to
pre-register against a quantity that can actually move. **Delete it and a result that was
pre-registered becomes indistinguishable from one that was not** — which is a strictly worse
position than never having pre-registered, because the claim survives and the evidence for it does
not.

**They also had to come**, independently of that: the findings files retained here **cite them**, so
leaving them behind would have left the retained layer pointing at nothing.

⚠️ **Flattening broke those citations and they were repointed.** Each findings file referred to
`PREREGISTRATION.md` as a bare sibling, which was unambiguous inside a snapshot directory and is not
here. Each now names its own — `2026-08-14-blend.md` cites `2026-08-14-blend-PREREGISTRATION.md`.
**This is the third time in one session that relocating a file broke a relative reference**, after
the two dangling-citation exemptions and the depth fixture. **Assume any move breaks references and
check, rather than assuming it does not.**

## ⚠️ It is twelve files, not one

`internal/snapshot/retracted_test.go` names **one** — `2026-08-15-gatescaled.md` — as holding the
in-place markers for three figures its own guard cannot check, because their context words ("gate",
"transfer", "threshold") are among the commonest in the record.

**Counted 2026-08-16: twelve of the fifteen carry withdrawal or retraction language**, and
`2026-08-15-clean-sheet-2x2.md` carries more of it than the one the test names. **The test's comment
is accurate about its own three figures and is not an inventory.** Recount with
`grep -rciE 'withdraw|retract' stats/findings/` rather than trusting either number.

## The cost, stated

**291 KB against the series' 11.0 MB — about 2.6%.** That is why retaining the whole layer was
preferred to rescuing the single named file: it costs almost nothing, and picking one file requires
being right about which markers matter, which the count above shows is easy to get wrong.

## ⚠️ These are OUTSIDE the retraction guard, deliberately and unchanged

The guard globs `stats/*.md`, which does not recurse, so nothing here is scanned — exactly as
nothing was scanned under `stats/snapshots/`. **That is intentional: these files quote withdrawn
wording verbatim, which is what a retraction narrative is for, and a guard that scanned them would
fire on the very text they exist to preserve.** Do not "fix" the glob to reach this directory.

The names are labels, not pointers: the `<date>-<sha>` came from the snapshot directory, and after
the planned history rewrite those SHAs will name commits that do not exist.
