# Retaining the pre-registrations

## What was reviewed

Five `PREREGISTRATION*.md` files moved from `stats/snapshots/<dir>/` to `stats/findings/`, ahead of
the series' removal, plus the sibling citations that move broke.

## Which reviewers ran

| reviewer | why |
|---|---|
| none | a file move plus a mechanical citation repoint, verified by the full suite and by grep. The judgement — *whether* they should be retained — is argued below rather than delegated |

## Why they had to be retained

⚠️ **A pre-registration's entire value is that it existed BEFORE the run.** It is the only thing
separating a prediction from a story told afterwards. **Deleting one leaves a strictly worse position
than never having pre-registered**, because the claim survives and the evidence for it does not — and
this project's standing rule is to pre-register against a quantity that can actually move.

**They also had to come for a second, independent reason**: the findings files retained earlier
**cite them**. Leaving them would have left the retained layer pointing at nothing.

## What broke, and it is the third instance of one lesson

⚠️ Each findings file referred to `PREREGISTRATION.md` as a **bare sibling** — unambiguous inside a
snapshot directory, meaningless once both are flattened into one. Repointed so each names its own.

**This is the third time in one session a relocation broke a relative reference**, and the three
failed differently, which is the point:

1. The findings move **repaired** two dangling-citation exemptions by accident, killing them.
2. The citation repoint **inverted a depth fixture** in `wikilink_test.go`, which asserted a
   two-level prefix from a three-deep file lands short.
3. This move broke **sibling** references, which depend on co-location rather than depth.

**Depth, history and co-location are three different ways a reference can depend on where a file
sits, and a move can break any of them with no edit to the reference itself.** Written into
`stats/findings/README.md` as: assume a move breaks references and check.

## What could not be checked

- **Whether the four findings files that cite a pre-registration are the only citers.** The grep
  covered tracked files; `reviews/` records that cite the old paths are dated attestations and are
  deliberately untouched.
- **No detection threshold applies.** A file move.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l` (empty) and the full `go test ./...` pass. Every
findings file that cited a bare `PREREGISTRATION.md` now names the qualified file, checked by grep.
