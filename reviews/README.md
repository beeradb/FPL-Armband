# reviews/

Dated review attestations — a review record saying it checked a given file is a claim about
what was true *then*, at the commit named in the directory. It is never rewritten to match a
later state, the same rule `stats/snapshots/`'s and `stats/cells/`'s own READMEs give for the
same reason: the pointer moves, the record of the pointer does not.

## The 79 SHA-keyed directories do not resolve, and that is expected

79 of the directories here are named `<date>-<sha>`, the SHA being the commit reviewed. **Most
or all of these SHAs no longer resolve on this repository's published history.** The 2026-08-16
history reset (`git rev-list --max-parents=0 HEAD` returns exactly one commit, `61bf00a`) means
any review dated before that point attests to a commit from a discarded pre-v1 history — check
with `gh api repos/beeradb/FPL-Armband/commits/<sha>` before assuming one resolves; a 422 is
not evidence the review is wrong, only that its subject predates the current history.

**Do not rename these directories to match a new history, and do not delete them for failing to
resolve.** Either would assert a provenance that was never true, or destroy a record of a
verdict someone reached. The directory name is a label recording what was reviewed, not a live
pointer a reader can follow — `stats/cells/README.md` states the identical rule for the
`<date>-<sha>` cells directories it carries, and `internal/snapshot/notes_test.go` and
`wikilink_test.go` exclude this whole tree from the citation-resolution guards that check every
other surface, on exactly this doctrine.

⚠️ **This note was owed since the reset and is being written 2026-08-22**, alongside removing
the accuracy snapshot series from the tree (see `stats/snapshots/`'s own note on that). The gap
was flagged in the vault design record for the reset and never closed until this session found
it while re-scoping that work — recorded so a future reader does not have to rediscover it.
