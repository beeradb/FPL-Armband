# Scoping the citation guard after the v1 reset

## What was reviewed

`TestNoTrackedMarkdownCitesAMissingFile` failed with **252 offenders** the moment the history became
a single root commit. Five live citations were repaired; the rest were brought into the scope the
project's other two guards already had.

## Why 252 appeared without a character changing

The guard accepts a citation if its path **is, or ever was, tracked** — resolved by walking
`git log --name-only`. With a thousand commits behind it, that covered nearly everything.

⚠️ **With one root commit, "ever tracked" collapses to "tracked now."** So 252 citations became
offenders in a single step, and **not one of them changed**. That is the clearest possible
demonstration that they were never being checked on their merits — only shielded by the length of
the history. It was the problem recorded as unsolved in the externalisation design, arriving exactly
where it was predicted to.

## The split, which is the whole of the fix

| where | count | disposition |
|---|---|---|
| `reviews/` | 225 | **excluded** — dated attestations |
| `stats/findings/`, `stats/snapshots/` | 22 | **excluded** — dated records and commitments |
| `docs/`, `AGENTS.md` | 5 | ⚠️ **REPAIRED, not excluded** |

**The exclusions are this project's own doctrine applied consistently, not an exemption invented to
make a test pass.** `TestNoLivePointerCitesTheRecordByPath` and
`TestRetractedFiguresAreNotQuotedAsCurrent` both already skip `reviews/`, for the reason
`notes_test.go` gives: a review record is a dated attestation about a named commit, and *"rewriting
the path inside it would make it attest to a location that did not exist."* A findings file, a
pre-registration and a banked snapshot are the same kind of object.

**The alternative was editing 247 dated records so they point at files that exist today, which is
precisely what that doctrine forbids.**

⚠️ **What is NOT excluded is every live surface** — `AGENTS.md`, `README.md`, `docs/`, `.claude/`
and the Go sources. Those are read forward, so a stale pointer in one is a *premise* rather than a
dated claim.

## The five that were repaired, and they were my own breakage

All five cited a bare `FINDINGS.md`, which stopped existing when the findings layer was relocated
earlier in this session. **The reset did not break them; my move did, and the reset revealed it.**
Each now names the real location, and two were factually wrong beyond the path — `AGENTS.md` and
`docs/README.md` both said `stats/snapshots/` holds the cells and the findings, which has not been
true since they were retained into `stats/cells/` and `stats/findings/`.

## Three exemptions removed, none of them repaired

They named `reviews/` files that are now out of scope, so each described nothing. **Their reasoning
is preserved in a comment where the entries were**, because it was never about the paths: one
recorded a dated record resting a gate decision on a document outside this repository — with the
lesson that a claim sourced that way may not be made in the first place, which is a rule for the
next record rather than a repair to that one. Another covered a citation *whose point was that it
does not resolve*, where repairing it would have deleted the finding.

## What could not be checked

- **Whether the 247 excluded citations are individually sound.** They are now unchecked by
  construction. The judgement is that a dated record's pointers are historical claims, not that each
  one is correct.
- **No detection threshold applies.** Counts of citations.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l` (empty) and the full `go test ./...` pass on the v1
baseline.
