# Repointing the findings citations

## What was reviewed

Eight citations to `stats/snapshots/<dir>/FINDINGS.md` repointed to `stats/findings/<dir>.md`.

⚠️ **This is a regression I introduced.** Retaining the findings layer moved those files and did not
repoint the things citing them, so the tree carried eight pointers that resolved only through git
history. Fixed here rather than left for the reset to publish.

## Which reviewers ran

| reviewer | why |
|---|---|
| none | a mechanical path substitution with an exact pattern, verified by the full suite and by a grep that returns no live pointer to the old location |

## The one that was not a citation

⚠️ **`internal/snapshot/wikilink_test.go` carried a DEPTH FIXTURE, not a citation, and the blanket
substitution inverted its expected result.** The case asserts that `../../docs/model.md` from a file
**three segments deep** lands one level short. Rewritten to a two-segment path it reaches the
repository root and resolves, so the test failed with `want false, got true`.

Reverted to a three-segment path and commented as load-bearing. **The path does not need to exist —
it is a depth fixture.**

⚠️ **This is the second time in one session that a relocation moved a file's depth and changed what a
relative link means**, the first being the two dangling-citation exemptions the findings move
repaired by accident. **The lesson is the same and now recorded twice: a relative link's correctness
is a property of depth, not content.**

## What was left alone

- **`reviews/`** — dated attestations, cited by their original path deliberately.
- **`stats/snapshots/`** — going away wholesale.
- **`stats/findings/README.md`** — its one mention of the old path is prose explaining the move.

## What could not be checked

- **Whether any citation outside the repository points at the old location.** Not visible from here.
- **No detection threshold applies.** A path substitution.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l` (empty), full `go test ./...` pass. `git grep` for the
old `FINDINGS.md` path outside `reviews/` and `stats/snapshots/` returns only the depth fixture and
one line of explanatory prose.
