package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// wikilinkPattern matches `[[target]]` where the brackets do not directly follow an
// identifier.
//
// ⚠️ The leading `(^|[^\w])` is not tidiness, it is the difference between a guard
// and a nuisance. **R indexes a list as `blocks[[b]]`**, and the first version of
// this test — which also scanned code spans — flagged
// `reviews/2026-08-14-dbe8251/review.md:35` on exactly that, in a sentence about
// `schedule_screen.R`. A guard whose first run reports a false positive in a
// committed review record is one nobody will keep.
var wikilinkPattern = regexp.MustCompile(`(^|[^\w])(\[\[[^\]\n]{1,200}\]\])`)

// TestNoTrackedMarkdownCitesAWikilink fails when a tracked Markdown file cites a
// document by `[[name]]` rather than by a path in this repository.
//
// # Why this is a defect and not a style preference
//
// A `[[name]]` in tracked Markdown is a pointer to something **outside the
// checkout**. Three things follow, and the third is the one that bites:
//
//   - **It cannot be resolved by a reader.** Every other reference in this
//     repository is a path, a commit, or an identifier that `git grep` finds. A
//     bare name in double brackets is checkable only by whoever wrote it.
//   - **It cannot be checked by any guard.** `TestRetractedFiguresAreNotQuotedAsCurrent`
//     scans this repository's own Markdown; a claim behind a `[[name]]` is outside
//     its reach, so a retracted figure can be cited through one and nothing fires.
//   - **It leaks the existence and the shape of an external store into a shared
//     artefact.** The reference names something the reader cannot see, which is at
//     best useless to them and at worst discloses a private index by enumerating it.
//
// # Why it is a test rather than a review item
//
// Because a review missed it, twice, and this record's own conclusion is that
// invariants beat reviewers decisively — see the note on
// TestReviewCoversTheCurrentCode.
//
// The instance that motivated this: `.claude/skills/review-gate/SKILL.md` carried a
// `[[…]]` reference that a docs review had already flagged and a later branch had
// already written a guard for — on a branch that was never merged. The commit that
// keyed the staleness guards on content then **edited that same file, 33 lines, and
// shipped the reference to `main` untouched**, because nothing in the tree checked.
// A guard that lives on an unmerged branch is not a guard.
//
// # Scope
//
// Every tracked `.md` file, including `.claude/`, which is where the miss happened
// and which is exactly as public as the rest of the repository.
//
// ⚠️ **Code is exempt, and the first version of this test was wrong to say it was
// not.** That version reasoned that "a fenced example teaches the syntax as
// acceptable". Its first run flagged `blocks[[b]]` — R list indexing, inside
// backticks, in a committed review record discussing `schedule_screen.R`. The
// reasoning was backwards: `[[…]]` is ordinary syntax in at least one language this
// project writes, so scanning code does not catch more leaks, it manufactures false
// ones. Fenced blocks and inline spans are blanked before scanning, and the
// not-preceded-by-an-identifier rule above catches the same case a second way.
func TestNoTrackedMarkdownCitesAWikilink(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	// Tracked files only. An untracked scratch file is nobody's business, and
	// walking the working tree would scan build output and dependencies.
	out, err := output(root, "git", "ls-files", "-z", "--", "*.md")
	if err != nil {
		t.Fatalf("listing tracked markdown: %v", err)
	}

	var offenders []string
	for _, rel := range strings.Split(out, "\x00") {
		if rel = strings.TrimSpace(rel); rel == "" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		eachUnfencedLine(string(body), func(n int, line string) {
			for _, m := range wikilinkPattern.FindAllStringSubmatch(blankCodeSpans(line), -1) {
				offenders = append(offenders, rel+":"+itoa(n)+"  "+m[2])
			}
		})
	}
	if len(offenders) == 0 {
		return
	}

	t.Errorf("%d tracked Markdown line(s) cite a document by [[name]] rather than by "+
		"a path in this repository:\n  %s\n\n"+
		"Replace each with something a reader of THIS checkout can resolve: a path, a "+
		"commit, or the name of the standing rule in AGENTS.md that carries the point. "+
		"If the reference has no in-repository equivalent, state the claim instead of "+
		"pointing at it — a pointer nobody can follow is worse than a sentence.\n\n"+
		"This is a test because two reviews missed the last one and a third wrote a "+
		"guard for it on a branch that never merged.",
		len(offenders), strings.Join(offenders, "\n  "))
}

// citedMarkdownLink matches the destination of a Markdown link, the `dest` of
// `[text](dest)`. Destinations only — the link text is prose and is not a pointer.
var citedMarkdownLink = regexp.MustCompile(`\[[^\]\n]*\]\(([^)\s]+)\)`)

// citationPath matches a token that is a path and NOTHING else.
//
// This is the whole false-positive defence, and it is deliberately unforgiving. A
// citation is a bare path standing alone; anything else that happens to end in
// `.md` is prose, a command, a glob or a template. **Four of the five forms below
// were observed in this tree on the guard's first run**, and a looser classifier
// would have reported each as a violation:
//
//   - a shell command — `git log --all -- docs/TODO.md` — fails on the spaces
//   - a template placeholder — `reviews/<dir>/review.md` — fails on the angle
//     brackets, which is the right answer whether or not `<dir>` is ever filled in
//   - a glob — `docs/*.md` — fails on the asterisk, because a pattern is a
//     statement about a set and this guard resolves individuals
//   - a bare `.md`, the extension discussed as a string, rejected on its empty
//     stem below
//   - a URL — `https://example/foo.md` — fails on the scheme's colon. ⚠️ **Not
//     observed here**: the tree holds three `http(s)` URLs and none names a `.md`,
//     so this one is pinned against arrival rather than against a sighting
//
// The leading class admits a dot so `.claude/agents/…` resolves, and an optional
// leading slash so a repo-root-relative citation is *classified* rather than
// silently skipped — the first version anchored on the character class alone, so
// `[the model](/docs/never-existed.md)` was not a citation at all and the guard
// passed it in silence while `resolvesInHistory` carried unreachable code for
// exactly that spelling. The empty-stem case both of those admit is rejected
// separately below, because a regexp that excluded it would also exclude every
// dotfile path.
var citationPath = regexp.MustCompile(`^/?[A-Za-z0-9_.][A-Za-z0-9_./-]*$`)

// danglingCitationExemption is one deliberate decision to leave a non-resolving
// citation exactly as written.
//
// Keyed by file, line and a digest of the cited token — never by the token itself.
// Two reasons, and the second is the load-bearing one. A file-and-line key alone
// would silently transfer to whatever citation later occupied that line, which is
// an allowlist granting cover it was never read as granting — the failure the
// `allowedGridLabels` comment in `internal/backtest` describes for a match that is
// too broad. And a `why` that quoted the offending text would put the unresolvable
// pointer into a second file, which is the defect this guard exists to stop,
// arriving through the escape hatch.
type danglingCitationExemption struct {
	file string
	line int
	// digest is the first 8 hex characters of the SHA-256 of the cited token,
	// after any `#fragment` or `?query` is stripped. Reported by the failure
	// message, so adding an entry never requires anyone to retype the citation.
	//
	// ⚠️ The key has one deliberate width: two citations of the SAME path on the
	// SAME line share a digest, so one entry silences both and the second is
	// invisible. Adding an occurrence ordinal would close it, at the cost of a key
	// that shifts when an earlier citation on the line is edited. The two entries
	// below that share a digest are on different lines, which is the ordinary case.
	digest string
	why    string
}

// allowedDanglingCitations enumerates every citation this repository has decided
// not to repair, each with the reason.
//
// **Every entry here is in `reviews/` or `stats/snapshots/`, and that is not a
// category exemption** — there is no rule that a path under those trees passes.
// Each line is listed, and a further one appearing under either tree tomorrow
// fails until somebody writes down why it should not.
//
// The shared reason is the one `TestNoLivePointerCitesTheRecordByPath` sets out at
// length for excluding those trees from its own scan: each file is a **dated
// attestation about a named commit**. A record saying it checked a given file is a
// claim about what was true then, and repointing the citation would make it attest
// to a location it never named. The pointer moves; the record of the pointer does
// not. What is different here is that the decision is per line rather than per
// tree, so the list shrinks as history allows rather than standing open forever.
//
// ⚠️ Three of these records are keyed by SHA and two by change name, because
// `guards-keyed-on-content` is the branch that retired SHA keying. Both spellings
// name their commits in the body, so the attestation property holds either way.
//
// **How long this list is, is reported by the test and asserted in no prose,
// including here** — the same rule `allowedGridLabels` states, for the same
// reason: an uncounted quantity gets written several different ways, and the
// exemption count is the only reviewable summary of the escape hatch. ⚠️ The
// `182 → 5` figures elsewhere in this file are **not** that count. They are a
// dated measurement of the resolver at `e8b931f`; if this list later differs from
// five, the measurement still stands and nothing there needs editing.
var allowedDanglingCitations = []danglingCitationExemption{
	// ⚠️ Two exemptions were REMOVED here on 2026-08-16, and the reason is worth
	// keeping because it is not "the citation was repaired by anyone".
	//
	// Both covered a two-level relative link to the scoring-model note, twice, in
	// the banked findings for 2026-08-11-0104d9d. It was one directory level SHORT
	// of its target on the day it was banked: from `stats/snapshots/<dir>/FINDINGS.md`
	// a `../../` prefix reached `stats/`, so it named a path under `stats/` that
	// never existed at any commit.
	//
	// Retaining the findings layer moved that file to `stats/findings/<dir>.md`, one
	// directory shallower — so the same prefix now reaches the repository root and
	// the link resolves against the note's real former home, which existed until
	// 2bf6018 and which `resolvesInHistory` therefore accepts. The move repaired the
	// citation as a side effect of the depth change, and the exemptions stopped
	// describing anything.
	//
	// (The path is described rather than written here: the retired-location scan in
	// notes_test.go bans that literal, and it caught this very comment.)
	//
	// The lesson for the next relocation: a relative link's correctness is a
	// property of the file's DEPTH, not of its content, so moving a file can repair
	// or break one without any edit. Re-run this guard after any move.

	// ⚠️ Three exemptions for `reviews/` files were REMOVED here on 2026-08-16,
	// and none of them was repaired — the scope moved out from under them.
	//
	// This guard now skips `reviews/` entirely, on the doctrine the other two
	// guards already applied: a review record is a dated attestation, and a
	// citation inside one is a claim about what was true then. So an exemption
	// naming a review record describes nothing, and the guard says so rather than
	// letting a grant sit there reading as though it still covers this tree.
	//
	// Their reasoning is not lost, because it was never about the paths. One
	// recorded that a dated record rested a gate decision on a document outside
	// this repository, so a reader cannot check what the gate was decided on —
	// and that a claim sourced that way may not be made in the first place, which
	// is a rule for the next record rather than a repair to that one. Another
	// covered a citation whose POINT was that it does not resolve, where repairing
	// it would have deleted the finding.
}

// TestNoTrackedMarkdownCitesAMissingFile fails when tracked Markdown cites a `.md`
// file that this checkout cannot produce.
//
// # It is the sibling guard's principle, generalised
//
// `TestNoTrackedMarkdownCitesAWikilink` above rejects one *syntax* on the ground
// that a `[[name]]` is a pointer a reader of this checkout cannot follow.
// `TestNoLivePointerCitesTheRecordByPath` rejects eight *literal spellings* on the
// same ground — one directory in three forms, and five document names. Between them
// sits the general case: a citation that looks exactly like an ordinary
// in-repository path, resolves to nothing, and is therefore indistinguishable from a
// working reference until somebody tries to follow it. Enumerating names cannot
// reach it, because the next one has a name nobody has written down yet.
//
// So this guard **carries no vocabulary at all**. It has no list of things not to
// cite; it has a resolver. That is deliberate and it is the reason this form could
// be guarded when others could not — a test that spelled the strings it forbids
// would be a larger disclosure than the set it polices, and it would also duplicate
// an enumeration this repository deliberately does not hold.
//
// # What counts as a citation
//
// Two forms, both of which a reader treats as "go and look at this":
//
//   - a Markdown link destination, `[text](path.md)`
//   - an inline code span holding a path and nothing else, “ `path.md` “
//
// The second is not optional decoration: it is the form the instance that
// motivated this guard actually uses, and it is the form **602 of the 686
// citations** in this tree use, against 84 link destinations. **Which means this guard reads code spans while
// the wikilink guard above blanks them** — the opposite treatment of the same
// text, for opposite reasons. `[[…]]` inside backticks is R indexing a list, so
// scanning code there manufactures false positives; a path inside backticks is a
// citation, so *not* scanning code there misses nearly all of them. That is why
// these are two functions and not one scan with two patterns.
//
// A `.md` filename in plain prose is **not** a citation and is not checked. The
// remedy for an unfollowable pointer is often to name the thing in prose instead of
// pathing to it, and a guard that then fired on the remedy would be telling authors
// to do something it punishes.
//
// # How a citation is resolved, and why it asks history
//
// The naive resolver — "is this a file at HEAD" — reports **182 violations in this
// tree**, all of them under `reviews/` (169) and `stats/snapshots/` (13), and an
// allowlist that long is a guard nobody keeps. Each cites a path that history shows
// was tracked at some commit and has since been deleted or moved. ⚠️ History
// establishes that the path once existed, **not** that it existed when the record
// was written — no path resolver can reach that, and the reading below does not
// need it.
//
// That is the discriminator, and it is pure structure: a citation resolves if its
// path is, **or ever was**, tracked on this branch. `git log --name-only` over this
// branch's history — 863 commits at `e8b931f`, quoted as the order of the work
// rather than as a fact that stays true — costs 62 ms and takes the count **from
// 182 to 5**. Both figures are measured with the classifier below rather than with
// the looser prototype that preceded it, which said 185 and 6. A reader who cannot
// open the file today can still run `git show <commit>:<path>`, which is following
// the pointer; a path that appears nowhere in this repository's history cannot be
// followed by any means available here, which is the defect.
//
// The set is the union of HEAD's tracked files with every path any commit touched.
// The union is there because `--name-only` reports nothing for a merge commit, so
// the history half alone would miss a file that arrived through a merge
// resolution. ⚠️ That closes the gap only for such a file **still at HEAD**: one
// introduced by a merge resolution *and later deleted* is in neither half. There
// are 28 merges on this branch, so the residue is real rather than theoretical —
// unobserved today, and it would surface as a false positive rather than as a miss.
// `HEAD` rather than `--all`, because `--all` sees whatever branches happen to
// exist in this clone and would make the verdict depend on local refs rather than
// on the commit under test.
//
// A bare basename with no directory resolves against basenames anywhere in that
// set, because a bare name is a *name* and a reader who has it can find it:
// `git ls-files` for one still at HEAD, `git log --diff-filter=A --name-only` for
// one that is not.
//
// # What it deliberately cannot see
//
// Stated because a guard whose reach is assumed is worse than one whose reach is
// known. A **renamed** file resolves under both names forever, since history holds
// both — this guard answers "can a reader find this", not "is this the current
// spelling". A **prose** mention is out of scope by the paragraph above. A citation
// **assembled across two spans** reads as two innocent tokens. And a file that
// exists but no longer contains what is claimed of it is a question no path
// resolver can ask.
//
// Four more, found by review rather than by design, and the first is **not
// hypothetical**:
//
//   - **Only `.md` targets are resolved.** `docs/architecture.md` cites a `.go`
//     filename this repository has never tracked on any ref, in a live reference
//     document, and this guard passes it. Widening the extension is the obvious
//     next move and is not made here, because a `.go` citation resolves against a
//     different population — identifiers, SDK files, paths in other repositories —
//     and would need its own false-positive study rather than a one-character edit.
//   - **Only tracked `.md` files are scanned as citing surfaces.** Go comments and
//     R scripts cite paths constantly and are outside this scan; the sibling reaches
//     them only for its own enumerated names.
//   - **A citation inside a fenced block is skipped by design**, and 0 of them fail
//     to resolve today, so the exemption costs nothing yet.
//   - **A file with an odd number of ``` markers inverts the fence state for its
//     remainder**, which reads as a pass. Measured across all 192 tracked Markdown
//     files: 0 such files, and fences hide 0.9% of lines. The fence reader is shared
//     with the guard above, so this limit is one implementation rather than two —
//     but it does not track fence length or `~~~`, and a file that needed either
//     would be silently half-scanned in both guards at once.
//
// A cross-line code span has the same shape and is worth its own note: the span
// reader is line-local, and the tree holds 28 non-fenced lines with an odd backtick
// count, all of them continuations. On the line after one, code and prose swap
// roles. No `.md` path is on any of the 28 today.
func TestNoTrackedMarkdownCitesAMissingFile(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	// This guard needs the whole history, and it asks git directly whether it has
	// it. A truncated history is an INABILITY to run the check, so it skips.
	//
	// # Two earlier versions of this precondition, and why both were wrong
	//
	// The first floored the UNION of HEAD and history. `git ls-files` supplies
	// ~1,500 paths here on its own, so that floor is satisfied before `git log` is
	// consulted and could never fail. A second version moved the floor onto the
	// history half alone, which fixes that — and rests on a claim about shallow
	// clones that is FALSE, and was never measured.
	//
	// The claim was that in a shallow clone `git log HEAD --name-only` returns only
	// the tip commit's files, "4045 records against 1 for a depth-1 walk". It does
	// not. A shallow clone's boundary commit is GRAFTED with no parents, so
	// `git log --name-only` diffs it against the empty tree and lists the entire
	// tracked tree in that one commit. Measured on clones of this worktree at
	// `5e4bc48`, the commit before the repair — the checkout is named because these
	// counts move with every commit, and this file is the wrong place of all to
	// quote a number without saying which tree produced it:
	//
	//	checkout              history records   floor of 500   guard
	//	full                            4,046         passes   passes
	//	git clone --depth 1             1,484         passes   FAILS, 177 offenders
	//	git clone --depth 5             1,499         passes   FAILS, 177 offenders
	//
	// So the floor let through exactly the checkout it was written to stop, and the
	// failure text asserts of 177 paths that each "is not, and never was, in this
	// repository" — false for all of them. The "1" appears to have come from
	// `git log -1` on a FULL clone, which is not what shallowness does. The lesson
	// is the one this project keeps paying for: a count is a PROXY, and a proxy for
	// a property nobody measured is a guess with a number attached.
	//
	// `git rev-parse --is-shallow-repository` asks the property itself, and is
	// measured too: `true` in both truncated clones above, `false` in the full one.
	//
	// One unit error died with the floor and is recorded so it is not rebuilt: the
	// count it compared was RECORDS — one per commit-file pair, repeats across
	// commits included — while the justification beside it was written against
	// DISTINCT paths, so "500 is well under the ~200 this branch adds beyond HEAD"
	// compared two different quantities and read as its own contradiction.
	shallow, err := historyIsTruncated(root)
	if err != nil {
		t.Fatalf("asking git whether this checkout's history is truncated: %v", err)
	}
	if shallow {
		t.Skipf("this checkout is a shallow clone, so it cannot say what was EVER " +
			"tracked and every citation to a deleted file would be reported as " +
			"dangling. Fetch full history — `git fetch --unshallow` — to run this guard.")
	}
	known, bases, err := everTrackedPaths(root)
	if err != nil {
		t.Fatalf("enumerating tracked and formerly-tracked paths: %v", err)
	}

	out, err := output(root, "git", "ls-files", "-z", "--", "*.md")
	if err != nil {
		t.Fatalf("listing tracked markdown: %v", err)
	}

	var offenders []string
	usedExemption := make([]bool, len(allowedDanglingCitations))
	for _, rel := range strings.Split(out, "\x00") {
		if rel = strings.TrimSpace(rel); rel == "" {
			continue
		}
		// ⚠️ `reviews/` is out of scope, and this is the project's own doctrine
		// applied here rather than an exemption invented for convenience.
		//
		// `TestNoLivePointerCitesTheRecordByPath` and
		// `TestRetractedFiguresAreNotQuotedAsCurrent` both already exclude it, for
		// the reason notes_test.go states at length: a review record is a DATED
		// ATTESTATION about a named commit. A citation inside one is a claim about
		// what was true then, not a pointer a reader should follow now, and
		// "rewriting the path inside it would make it attest to a location that did
		// not exist".
		//
		// This guard did not exclude it and appeared not to need to, because
		// `resolvesInHistory` accepts anything ever tracked — which covered every
		// review record's citations for free.
		//
		// ⚠️ The v1 history reset removed that cover. With a single root commit
		// "ever tracked" collapses to "tracked now", and 225 citations inside
		// `reviews/` became offenders in one step without a character of any of them
		// changing. That is the clearest possible demonstration that they were never
		// being checked on their merits — only shielded by the length of the history.
		//
		// Excluding it is therefore not a weakening. It restores the scope the other
		// two guards already have, and the alternative — editing 225 dated
		// attestations so they point at files that exist today — is exactly what the
		// doctrine forbids.
		// The same argument reaches `stats/findings/` and `stats/snapshots/`, and it
		// is the same argument rather than a second one. A findings file is a dated
		// record of a run; a pre-registration is a dated commitment made before one;
		// a banked snapshot is a dated measurement. Each cites the tree as it stood
		// on its own day, and each was likewise covered for free by the history
		// escape hatch until the reset removed it.
		//
		// ⚠️ What is NOT excluded is every live surface: `AGENTS.md`, `README.md`,
		// `docs/`, `.claude/` and the Go sources. Those are read forward, a stale
		// pointer in one is a premise rather than a dated claim, and five of them
		// were repaired rather than exempted when this scope was drawn — they cited
		// a bare `FINDINGS.md` that the findings relocation had already moved.
		if strings.HasPrefix(rel, "reviews/") ||
			strings.HasPrefix(rel, "stats/findings/") ||
			strings.HasPrefix(rel, "stats/snapshots/") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		eachUnfencedLine(string(body), func(n int, line string) {
			for _, tok := range citationsOn(line) {
				stem, ok := citedFile(tok)
				if !ok || resolvesInHistory(stem, rel, known, bases) {
					continue
				}
				digest := citationDigest(stem)
				if exempt(allowedDanglingCitations, rel, n, digest, usedExemption) {
					continue
				}
				offenders = append(offenders, rel+":"+itoa(n)+"  digest "+digest)
			}
		})
	}

	// An exemption matching nothing is a citation that was repaired, a record that
	// moved, or a line number that drifted — and all three read downstream as "the
	// allowlist still describes this tree". Reporting it is what makes the list
	// shrink as history allows instead of accumulating dead grants.
	//
	// Collected separately from the offenders, and the first version appended both
	// to one slice. That made the repair of a dangling citation fail the test with
	// a message announcing a NEW dangling citation, and with a count that included
	// the dead grants — the exact opposite of what had happened, reported to
	// whoever had just done the right thing.
	var dead []string
	for i, a := range allowedDanglingCitations {
		if !usedExemption[i] {
			dead = append(dead, "allowedDanglingCitations entry for "+
				a.file+":"+itoa(a.line)+" (digest "+a.digest+") matches nothing — "+
				"delete it if the citation is gone, or re-key it if the line moved")
		}
	}

	if len(offenders) > 0 {
		t.Errorf("%d Markdown citation(s) name a `.md` file that is not, and never "+
			"was, in this repository:\n  %s\n\n"+
			"The digest is the first 8 hex characters of the SHA-256 of the cited "+
			"path; the path itself is at the file and line given, and is "+
			"deliberately not repeated here.\n\n"+
			"A citation nobody can follow makes the claim it supports uncheckable "+
			"from this checkout. Repoint it at something that resolves, or drop the "+
			"pointer and state the claim — a sentence a reader can weigh beats a "+
			"path they cannot open.\n\n"+
			"If the line is a DATED ATTESTATION about a named commit — a record "+
			"under `reviews/` or `stats/snapshots/` — it must not be rewritten, "+
			"because repointing it would make it attest to something it never said. "+
			"Add it to allowedDanglingCitations with its reason; there are %d such "+
			"exemptions today, and that count is reported here rather than written "+
			"down anywhere.",
			len(offenders), strings.Join(offenders, "\n  "),
			len(allowedDanglingCitations))
	}
	if len(dead) > 0 {
		t.Errorf("%d exemption(s) in allowedDanglingCitations no longer match "+
			"anything:\n  %s\n\n"+
			"This is not a new dangling citation. It means a citation was repaired, "+
			"a record moved, or a line drifted — and a grant that describes nothing "+
			"reads downstream as an allowlist still describing this tree.",
			len(dead), strings.Join(dead, "\n  "))
	}
}

// eachUnfencedLine calls fn for every 1-indexed line of a Markdown body that is not
// inside a fenced code block.
//
// One implementation, both guards. The fence machine was written twice in this file
// — five identical lines each — in the same commit that factored `codeSpanRanges`
// out to avoid exactly that, which is this project's signature failure arriving
// inside its own remedy. It matters here more than most duplications: an odd number
// of fence markers leaves the state stuck to end of file and silently stops
// scanning the remainder, so a fence-handling fix applied to one copy and not the
// other would leave one guard half-blind with nothing to show for it.
//
// A fenced block is example text. A non-resolving filename in one is usually
// deliberate illustration, and this project's records print commands and templates
// in fences constantly.
func eachUnfencedLine(body string, fn func(n int, line string)) {
	fenced := false
	for i, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		fn(i+1, line)
	}
}

// citationsOn returns every candidate citation token on a line: Markdown link
// destinations, and the contents of inline code spans.
//
// It extracts and does not judge. Deciding what is a citation of a file in this
// checkout is `citedFile`'s job, in one place, because both forms need the same
// answer — a URL is as much a location on another host inside backticks as it is
// inside a link.
func citationsOn(line string) []string {
	var out []string
	for _, m := range citedMarkdownLink.FindAllStringSubmatch(line, -1) {
		out = append(out, m[1])
	}
	for _, r := range codeSpanRanges(line) {
		out = append(out, line[r[0]+1:r[1]])
	}
	return out
}

// uriScheme matches a leading `scheme:`, per RFC 3986's grammar. A Windows drive
// letter would match too, and that is fine: neither is a path in this checkout.
var uriScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// citedFile reports whether tok is a citation of a `.md` file in this checkout, and
// returns it with any `#fragment` or `?query` removed.
//
// The fragment is stripped BEFORE the `.md` test, so `docs/model.md#scoring` is
// checked rather than skipped — an anchor into a file that does not exist is the
// same dangling pointer with a suffix. It is stripped before the path test too, or
// the `#` would take the whole token out of scope, which is the same miss wearing
// the opposite costume.
func citedFile(tok string) (string, bool) {
	tok, _, _ = strings.Cut(tok, "#")
	tok, _, _ = strings.Cut(tok, "?")
	// A location on another host is not a path here, and asking whether this
	// checkout produces one is a category error: it would fail forever and could
	// only ever be silenced by an exemption. The scheme's colon is outside
	// `citationPath`'s character class, but the protocol-relative `//` is NOT —
	// `citationPath` admits a leading slash so a repo-root citation is classified,
	// and that admits `//host/x.md` with it.
	if uriScheme.MatchString(tok) || strings.HasPrefix(tok, "//") {
		return "", false
	}
	if !strings.HasSuffix(tok, ".md") || !citationPath.MatchString(tok) {
		return "", false
	}
	// A bare `.md` is the extension being discussed as a string, not a file. The
	// regexp above cannot reject it without also rejecting `.claude/…`.
	if strings.TrimSuffix(path.Base(tok), ".md") == "" {
		return "", false
	}
	return tok, true
}

// resolvesInHistory reports whether a cited path names something a reader of this
// checkout can produce, from the citing file `from`.
//
// A path with a directory is tried relative to the citing file first and then
// relative to the repository root, in that order, since both spellings are in use
// here and a reader would try both.
//
// A leading `/` is stripped and the result then goes through those same two
// spellings — deliberately more permissive than "the root", which is what an
// earlier version of this comment claimed. Nothing in this repository is an
// absolute filesystem citation, so the choice is between reading `/x/y.md` as the
// repository root or refusing to classify it at all; refusing is what let the form
// through unexamined before.
func resolvesInHistory(tok, from string, known, bases map[string]bool) bool {
	if !strings.Contains(tok, "/") {
		return bases[tok]
	}
	rooted := strings.TrimPrefix(tok, "/")
	for _, cand := range []string{
		path.Join(path.Dir(from), rooted),
		path.Clean(rooted),
	} {
		if known[cand] {
			return true
		}
	}
	return false
}

// historyIsTruncated reports whether this checkout's history has been cut short,
// by asking git the question rather than inferring it from a count.
//
// `git rev-parse --is-shallow-repository` prints `true` for anything grafted —
// `--depth N`, `--shallow-since`, CI at `fetch-depth: 1` — and `false` for a
// complete clone. It is the property `everTrackedPaths` actually depends on, so
// there is no gap between what is asked and what matters. Measured on clones of
// this worktree: `true` at depth 1 and depth 5, `false` on a full clone. That
// answer is a property of the checkout and does not drift with the tree, which is
// the whole advantage over a count.
//
// Why not count records instead: a shallow clone's boundary commit has no
// parents, so `git log --name-only` diffs it against the empty tree and reports
// the whole tracked tree in that one commit. At `5e4bc48` a depth-1 clone returned
// 1,484 records — plenty to clear any floor a full clone's 4,046 would suggest,
// and missing precisely the deleted files this guard resolves against. See the
// note at the caller for the two floors that failed this way, and note that both
// figures move with every commit, which is a second reason not to gate on them.
func historyIsTruncated(root string) (bool, error) {
	out, err := output(root, "git", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return false, err
	}
	// git prints `true`/`false`; anything else is a git that does not answer this
	// question, and guessing on its behalf is what the counting version did.
	switch strings.TrimSpace(out) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, errors.New("git rev-parse --is-shallow-repository said " +
		strconv.Quote(strings.TrimSpace(out)) + ", which is neither true nor false")
}

// everTrackedPaths returns every path this branch has ever tracked and the set of
// their basenames.
//
// The union of HEAD and history, deliberately: `git log --name-only` reports
// nothing for a merge commit, so a file that arrived only through a merge
// resolution would be missing from the history half. Adding HEAD's own listing
// closes that in the safe direction — a path wrongly present only makes the
// resolver more permissive, and this guard's expensive failure is the false
// positive.
//
// It assumes complete history and does not check for it, because the union is
// exactly what would hide the lack: HEAD alone supplies ~1,500 paths, so no
// property of the returned sets distinguishes a full clone from a truncated one.
// `historyIsTruncated` is that check and the caller runs it first.
func everTrackedPaths(root string) (paths, bases map[string]bool, err error) {
	paths = map[string]bool{}
	bases = map[string]bool{}
	add := func(out string) {
		for _, rel := range strings.Split(out, "\x00") {
			if rel = strings.TrimSpace(rel); rel == "" {
				continue
			}
			paths[rel] = true
			bases[path.Base(rel)] = true
		}
	}
	// `-z` for the reason `trackedFiles` gives: git C-quotes non-ASCII paths
	// depending on the reader's `core.quotePath`, so a newline-delimited listing
	// is a resolver whose reach varies with local configuration.
	out, err := output(root, "git", "ls-files", "-z")
	if err != nil {
		return nil, nil, err
	}
	add(out)
	// `HEAD` and not `--all`: `--all` walks whatever refs this clone happens to
	// hold, so the same commit would resolve differently in two checkouts.
	out, err = output(root, "git", "log", "HEAD", "-z", "--pretty=format:", "--name-only")
	if err != nil {
		return nil, nil, err
	}
	add(out)
	return paths, bases, nil
}

// citationDigest is the exemption key: the first 8 hex characters of the SHA-256 of
// the cited path.
//
// Eight characters over a population of single-digit size, so a collision is not
// the risk being managed — being able to name a citation without writing it down
// is.
func citationDigest(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])[:8]
}

// exempt reports whether this file, line and digest are on the allowlist, marking
// the entry used so a dead one can be reported.
func exempt(list []danglingCitationExemption, file string, line int, digest string, used []bool) bool {
	for i, a := range list {
		if a.file == file && a.line == line && a.digest == digest {
			used[i] = true
			return true
		}
	}
	return false
}

// codeSpanRanges returns the inclusive byte ranges of each `inline code` span on a
// line, left to right, backtick characters included.
//
// One implementation, two readers, deliberately. The guard above BLANKS spans and
// the one below READS them, and the only thing they share is where a span starts
// and stops — which is exactly the kind of quantity this project has twice shipped
// two copies of. An unterminated backtick opens a span that never closes and is
// therefore never returned, so it can neither swallow the rest of the prose above
// nor manufacture a citation below.
func codeSpanRanges(line string) [][2]int {
	var out [][2]int
	open := -1
	for i := 0; i < len(line); i++ {
		if line[i] != '`' {
			continue
		}
		if open < 0 {
			open = i
			continue
		}
		out = append(out, [2]int{open, i})
		open = -1
	}
	return out
}

// blankCodeSpans replaces the contents of `inline code` with spaces, preserving
// length so reported columns stay meaningful and so an unterminated backtick cannot
// swallow the rest of the line's prose.
func blankCodeSpans(line string) string {
	out := []byte(line)
	for _, r := range codeSpanRanges(line) {
		for j := r[0]; j <= r[1]; j++ {
			out[j] = ' '
		}
	}
	return string(out)
}

// TestACitationIsToldFromProse pins the discrimination
// TestNoTrackedMarkdownCitesAMissingFile rests on, which its doc comment can only
// assert.
//
// Four of the negative cases below — the command, the template placeholder, the
// glob and the bare `.md` — were observed in this tree on the guard's first run,
// and a looser classifier would have reported each. The URL and the `.md.bak` are
// pinned against arrival rather than sighted. `internal/snapshot/watched.go` is
// sighted but *resolves*, so it pins the `.md` suffix test rather than a false
// positive.
//
// That distinction is the point of stating it: this guard fires on written records
// rather than on code, the cost of a false positive is an author being told to
// repair a sentence that is already correct, and a guard that does that twice gets
// deleted. So the classifier is tested directly rather than only through the
// repository it happens to be scanning today — the repository is one sample, and a
// green scan is not evidence that the next sentence is safe.
func TestACitationIsToldFromProse(t *testing.T) {
	for _, c := range []struct {
		tok  string
		want string // "" means: not a citation
		why  string
	}{
		{"docs/model.md", "docs/model.md", "the ordinary form"},
		{"docs/model.md#scoring", "docs/model.md", "an anchor is stripped, not a reason to skip"},
		{"docs/model.md?v=2", "docs/model.md", "a query is stripped for the same reason"},
		{".claude/agents/fpl-stats-review.md", ".claude/agents/fpl-stats-review.md",
			"a leading dot is a directory here, not an extension"},
		{"README.md", "README.md", "a bare basename is still a citation"},
		{"/docs/model.md", "/docs/model.md",
			"a repo-root-relative citation must be CLASSIFIED — the first version " +
				"anchored it out, so the guard passed the form in silence while the " +
				"resolver carried unreachable code for it"},

		{"git log --all -- docs/TODO.md", "", "a command: spaces"},
		{"reviews/<dir>/review.md", "", "a template placeholder: angle brackets"},
		{"docs/*.md", "", "a glob names a set, and this resolver resolves individuals"},
		{"https://example.invalid/foo.md", "", "another host: the scheme colon"},
		{"//example.invalid/foo.md", "",
			"protocol-relative, and NOT caught by the character class once a leading " +
				"slash is admitted — this is the case that keeps the check in citedFile"},
		{".md", "", "the extension discussed as a string, not a file"},
		{"internal/snapshot/watched.go", "", "not Markdown"},
		{"docs/model.md.bak", "", "does not end in .md"},
	} {
		got, ok := citedFile(c.tok)
		if !ok {
			got = ""
		}
		if got != c.want {
			t.Errorf("citedFile(%q) = %q, want %q — %s", c.tok, got, c.want, c.why)
		}
	}
}

// TestACitationResolvesTheWayAReaderWould pins the resolver's two spellings and the
// history rule that keeps the allowlist short.
func TestACitationResolvesTheWayAReaderWould(t *testing.T) {
	known := map[string]bool{
		"docs/model.md":        true,
		"stats/README.md":      true,
		"gone/deleted-note.md": true, // history only: not at HEAD
	}
	bases := map[string]bool{}
	for k := range known {
		bases[path.Base(k)] = true
	}

	for _, c := range []struct {
		tok, from string
		want      bool
		why       string
	}{
		{"docs/model.md", "AGENTS.md", true, "repo-root spelling"},
		{"model.md", "AGENTS.md", true, "a bare basename resolves by name"},
		{"../docs/model.md", "stats/README.md", true, "relative to the citing file"},
		{"/docs/model.md", "AGENTS.md", true, "a leading slash reaches the repo root"},
		{"/docs/model.md", "stats/README.md", true,
			"and from a SUBDIRECTORY too — the stripped form is tried by both " +
				"spellings, so this is deliberately more permissive than 'the root'. " +
				"The only prior case cited from AGENTS.md, whose directory IS the " +
				"root, so the two candidates coincided and this was untested"},
		{"gone/deleted-note.md", "AGENTS.md", true,
			"deleted, but git show at the commit still produces it — this is the rule " +
				"that takes 182 violations down to 5"},
		{"docs/never-existed.md", "AGENTS.md", false, "the defect"},
		// ⚠️ A THREE-SEGMENT `from` is load-bearing here and must stay three. This
		// case asserts that a two-level prefix from a file three deep lands one
		// level short. A repoint of the findings layer rewrote it to a two-segment
		// path on 2026-08-16 and inverted the expected result, because from two deep
		// the same prefix reaches the repository root and resolves. The path need not
		// exist — it is a depth fixture, not a citation.
		{"../../docs/model.md", "stats/snapshots/run/FINDINGS.md", false,
			"one level short: resolves to stats/docs/model.md, which is nothing"},
	} {
		if got := resolvesInHistory(c.tok, c.from, known, bases); got != c.want {
			t.Errorf("resolvesInHistory(%q, from %q) = %v, want %v — %s",
				c.tok, c.from, got, c.want, c.why)
		}
	}
}

// TestOneGuardReadsCodeSpansAndTheOtherBlanksThem pins the one place these two
// scans deliberately disagree, because it is the pairing most likely to be
// "simplified" into a single walk — and doing so would silently switch off the form
// nearly every citation in this repository's records actually uses.
func TestOneGuardReadsCodeSpansAndTheOtherBlanksThem(t *testing.T) {
	const line = "the ledger in `docs/model.md` and [the harness](docs/replay.md), " +
		"but not blocks[[b]] in R"

	want := map[string]bool{"docs/replay.md": true, "docs/model.md": true}
	for _, g := range citationsOn(line) {
		delete(want, g)
	}
	if len(want) > 0 {
		t.Errorf("citationsOn missed %v in %q — the code-span form is the one the "+
			"records use, so losing it costs nearly all of this guard's reach", want, line)
	}
	if strings.Contains(blankCodeSpans(line), "docs/model.md") {
		t.Error("blankCodeSpans no longer blanks spans, so the wikilink guard would " +
			"start scanning code — the false positive that scan's comment records")
	}

	// An unterminated backtick opens a span that never closes. Neither reader may
	// treat the rest of the line as code.
	const ragged = "an open ` and then docs/model.md"
	if len(citationsOn(ragged)) != 0 {
		t.Error("an unterminated backtick manufactured a citation")
	}
	if blankCodeSpans(ragged) != ragged {
		t.Error("an unterminated backtick swallowed the rest of the line's prose")
	}
}

// TestADanglingCitationExemptionIsNarrow is the positive control on the escape
// hatch: an exemption must match one line and one citation, never a file.
func TestADanglingCitationExemptionIsNarrow(t *testing.T) {
	list := []danglingCitationExemption{{"a/record.md", 7, "deadbeef", "because"}}
	for _, c := range []struct {
		file   string
		line   int
		digest string
		want   bool
	}{
		{"a/record.md", 7, "deadbeef", true},
		{"a/record.md", 8, "deadbeef", false},
		{"a/record.md", 7, "0badcafe", false},
		{"b/record.md", 7, "deadbeef", false},
	} {
		used := make([]bool, len(list))
		if got := exempt(list, c.file, c.line, c.digest, used); got != c.want {
			t.Errorf("exempt(%s:%d, %s) = %v, want %v", c.file, c.line, c.digest, got, c.want)
		}
		if used[0] != c.want {
			t.Errorf("exempt(%s:%d, %s) marked used=%v, want %v — a dead entry is "+
				"only reportable if a live one is marked",
				c.file, c.line, c.digest, used[0], c.want)
		}
	}
}

// itoa avoids pulling strconv in for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
