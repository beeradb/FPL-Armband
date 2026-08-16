# The rename: `fplagent` → `armband`

## What was reviewed

Renaming the module, the binary, the command directory and every prose reference. The product is
**FPL Armband**; the identifier is **`armband`**, chosen by the user from four options because a
bare identifier reads best as both a command and an import path.

**Counted on this tip rather than quoted from the plan**, which is the standing rule here and which
mattered — the plan's figures were 836/429/407 and the tree has moved since:

| | occurrences |
|---|---|
| to change | **430** |
| to leave | **434** |
| total | **864** |

The sub-counts sum to 864 exactly, which is the check. Accounted after the sweep: **428** rewritten
by the sweep, **1** by `go mod edit` (the module line, changed before the sweep ran), **1**
deliberately preserved. 430.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| none dispatched | ⚠️ **Deliberate, and an invariant is the stronger evidence here.** The quantity this must not move is *behaviour*, and the test is the full suite compiling and passing under the new module path — which it does, 15 packages. A reviewer reading 428 identical substitutions would be weaker evidence than the compiler resolving every import |

## ⚠️ The one occurrence that must not change, and why a directory exclusion would have missed it

`internal/capture/capture.go` carries `json:"fplagent_version,omitempty"` — **a persisted schema
key**, written into **228 banked capture manifests** under `data/`. Renaming it either breaks
reading the archive or needs a migration, for no benefit.

⚠️ **The plan's exclusion list is by DIRECTORY — `stats/`, `reviews/`, `data/` — and this key lives
in `internal/`, inside the rename bucket.** A directory-based sweep would have renamed it and the
breakage would have surfaced later, as captures that no longer parse. The sweep protects it by
name, and it is now **the only `fplagent` left outside the record directories**, which is a
one-line check anyone can re-run:

    git grep -n fplagent -- . ':!stats' ':!reviews' ':!data'

## What was left alone, and why

- **`stats/` and `reviews/` (434 occurrences).** Banked findings and dated review records.
  Rewriting them makes a dated artefact attest to a name that did not exist when it was written.
- **`data/captures/*/manifest.json`.** The persisted schema above, in its stored form.

## What was changed that is worth naming

- **Five `User-Agent` headers** now send `armband/1.0` to the FPL API, Understat and the Internet
  Archive. The plan flagged this as a courtesy question rather than a technical one. **Changed**,
  on the ground that the honest thing for a tool to send is its own name — but it is an
  outward-facing change and is recorded here rather than buried in a diff.
- **`.gitignore`'s build-output entries**, `/armband` and `armband-sweep`.
- **The README heading is the PRODUCT name, not the identifier.** The sweep turned it into
  `# armband`; it now reads **FPL Armband**, with one line saying the binary is `armband`, which is
  the question a first-time reader would otherwise have to answer for themselves.

## A claim I wrote and then withdrew before committing

⚠️ **The first version of the README's naming line said a perfect armband is worth "around 210
points a season" and called it the thing the model can prove.** `AGENTS.md` says the opposite in
the same entry: the 210 is **perfect hindsight**, its `t` of 20.4 is *"mechanical and not comparable
with any other t here"*, and **"the entire observed span of captaincy rules is about 28 points a
season — that is what a real captaincy change competes for, not 210."**

Quoting 210 as an achievement would have been falsifiable by anyone who read the docs. **Replaced
with a claim that needs no citation**: the captaincy is the biggest single weekly decision and the
one effect in the record that stands clear of the noise.

## What could not be checked on this harness

- **That nothing outside this repository imports the old module path.** The repo is unpublished, so
  there is nothing to break — which is precisely why this was the moment to do it.
- **Whether the cache directory should have been renamed.** It is `.cache/fpl`, not `.cache/fplagent`,
  so the sweep never touched it and no cache is orphaned. The plan raised a `~/.cache/fplagent` path
  that does not exist in this tree.
- **No detection threshold applies.** Nothing was measured; this is a rename.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l ./internal ./cmd` (empty) and the **full `go test ./...`
across all 15 packages** pass under `armband/...`. The binary builds and `./armband` prints its
usage under the new name. `internal/capture` passes, which is the package the preserved schema key
protects.

## Addendum — the merge from `main`, and what it demonstrates

`main` advanced by ten commits mid-branch (an engine scoring-rules pin), and the merge reintroduced
**six** `fplagent` references: five import paths in new test files and one prose comment. The sweep
was re-run and caught all six.

⚠️ **This is the argument for doing a rename in one shot rather than incrementally.** Every branch in
flight at the moment of a rename carries the old identifier in whatever it adds, and there is no
guard that notices — the compiler does not care which module path a file imports as long as it
resolves, and after the module line changes the *old* path stops resolving, which is the only reason
these surfaced at all.

**The check that generalises**, and it is one line rather than a count that goes stale:

    git grep -n fplagent -- . ':!stats' ':!reviews' ':!data'

**It should return exactly one hit** — `internal/capture/capture.go`'s persisted schema key. Anything
else is an unswept reference. That is a better invariant than any number in this record, and it held
after both sweeps.
