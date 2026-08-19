package stats

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// goSource is one Go file of this repository as the source scans read it: its
// path relative to the repository root, always slash-separated, and its text.
type goSource struct {
	rel  string
	body string
}

// goSources lists every .go file the source scans consider.
//
// Shared by [TestTheMiddleValueHasOneImplementation] and
// [TestTheCopiedExpressionsHaveOneImplementation] so the *reach* of a scan is
// decided in one place. A guard whose reach is assumed is worse than one whose
// reach is known, and two walkers drifting apart would be this record's signature
// failure inside the very thing that exists to catch it.
//
// The skipped directories are matched on the path, not the base name. Skipping any
// directory *called* "stats" would also skip `internal/stats`, which is where the
// median lives and therefore the one package a second median is most likely to
// appear in.
func goSources(root string) ([]goSource, error) {
	var out []goSource
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable corner of the tree is not a scan's business
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			// The root is itself a checkout, so it must be exempted before the
			// nested-checkout test below or the scan skips everything and the
			// "no Go sources found" guard fires.
			if rel == "." {
				return nil
			}
			switch rel {
			case "node_modules", "data", "stats":
				return filepath.SkipDir
			}
			// A directory that is ITSELF a checkout, detected by a `.git` entry
			// rather than by name, so it holds for a worktree (a `.git` file), a
			// clone (a `.git` directory) and a vendored repository alike, and does
			// not go stale when the directory those live in is renamed.
			//
			// Without this the scan descends into every sibling worktree under
			// `.claude/worktrees/`. Measured from the main checkout: 9,101 of 9,450
			// Go files reached were another branch's. That breaks the guard in BOTH
			// directions — it reports a copy that exists only on a branch nobody is
			// merging, and `sanctioned` is keyed on repo-relative paths that cannot
			// match a sibling's, so a real exemption stops applying. ⚠️ It is
			// invisible from inside a worktree, where `.claude/worktrees` is empty;
			// it only bites where this test actually runs.
			if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
				return filepath.SkipDir
			}
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		out = append(out, goSource{rel: rel, body: string(b)})
		return nil
	})
	return out, err
}

// copiedExpression is one quantity that must have a single implementation, plus
// the shape a copy of it takes and where the sanctioned occurrences are.
//
// The next quantity to guard is a ROW here, not a new test. That is the whole
// point: a bespoke guard per copy stops one divergence, a table stops the next
// copy — and this repository already carries several bespoke ones, which is the
// pattern being retired rather than a number worth quoting.
type copiedExpression struct {
	// quantity is what the expression computes, in words a reader who has not
	// just been in that code can check the offender against.
	quantity string
	// cost is what a second copy has already cost, or would cost. Printed on
	// failure, because a guard that only says "no" gets exempted rather than obeyed.
	cost string
	// skipTests drops _test.go files. Set it only where a test file legitimately
	// writes the shape for a different reason, and say which.
	skipTests bool
	// match reports whether one line of source is an occurrence of the shape.
	match func(line string) bool
	// sanctioned is how many occurrences each file is allowed, homes included. It
	// is the debt list, and it must SHRINK: a file carrying fewer than recorded
	// fails too, so nobody can fold a copy in and leave the debt listed as
	// outstanding. `why` beside each entry is not decoration — an entry whose
	// argument nobody can restate is an entry that should be a bug.
	sanctioned map[string]sanction
}

type sanction struct {
	n   int
	why string
}

// runningTopTwo matches `a, b = <expr>, a` — a tuple assignment whose second
// target takes the first target's OLD value.
//
// That is the running best-and-second-best update, and it is the shape of the
// captaincy arithmetic with the names filed off. Go's regexp has no
// backreferences, so the equality of the first and last identifier is checked
// after the match rather than inside it.
//
// The trailing `(\s*//.*)?` is not cosmetic: without it a copy annotated
// `captain, vice = p.Score, captain // demote` is invisible, and annotating a
// line one has just pasted is what a careful author does.
var runningTopTwo = regexp.MustCompile(
	`^\s*([A-Za-z_]\w*)\s*,\s*([A-Za-z_]\w*)\s*=\s*(.+?),\s*([A-Za-z_]\w*)\s*(//.*)?$`)

func isRunningTopTwo(line string) bool {
	m := runningTopTwo.FindStringSubmatch(line)
	// m[1] == m[4] is the demotion — the old leader becoming the runner-up. m[2]
	// != m[4] rules out `a, b = x, a` where b IS a, which is not an assignment
	// Go accepts anyway but costs nothing to exclude.
	return m != nil && m[1] == m[4] && m[2] != m[4]
}

// archiveSeasonFormat matches the archive's season name being assembled: a
// four-digit year, a dash, and the next year's last two digits.
var archiveSeasonFormat = regexp.MustCompile(`"%d-%02d"`)

// priorProjection matches an `analysis.PriorPlayer` composite literal that
// actually sets fields — either inline (`PriorPlayer{Minutes: …`) or opened at the
// end of a line. `PriorPlayer{}` is not a match: it projects nothing.
//
// priorContainer then takes back the CONTAINER literals, whose braces belong to a
// map or a slice rather than to a player: `map[int]analysis.PriorPlayer{` opens on
// a `{` that sets no field. Every such literal in the tree today is written with
// empty braces, so the exclusion is currently redundant — and that is exactly why
// it is here rather than left to luck: the first non-empty one would otherwise be
// reported as a second projection, with advice ("one projection per SOURCE") that
// does not apply to it.
//
// ⚠️ The elements INSIDE such a literal are then unscanned, which is the
// deliberate trade. A full ten-field projection could hide in a map literal's
// values; a fixture, which is what those literals hold, could not be told from it.
var (
	priorProjection = regexp.MustCompile(`PriorPlayer\{\s*($|[A-Za-z])`)
	priorContainer  = regexp.MustCompile(`(map\[[^\]]*\]|\[\])\s*\*?[\w.]*PriorPlayer\{`)
)

func isPriorProjection(line string) bool {
	return priorProjection.MatchString(line) && !priorContainer.MatchString(line)
}

// saturatingRatio matches `x / (x + k)` — a quantity divided by itself plus
// something else.
//
// That is the option-value decay with the names filed off: the curve that takes a
// held option's worth to exactly zero at its expiry and saturates toward 1 with a
// long window. Four levers read it — a banked transfer, a wildcard, a bench boost
// and a free hit — and four copies of it is precisely the failure this table
// exists for.
//
// Go's regexp has no backreferences, so the equality of the numerator and the
// first term of the denominator is checked after the match rather than inside it.
// The identifier pattern admits a selector (`w.Remaining`), because a copy written
// against a struct field is what somebody reaching for the obvious writes.
var saturatingRatio = regexp.MustCompile(
	`([A-Za-z_][\w.]*)\s*/\s*\(\s*([A-Za-z_][\w.]*)\s*\+`)

// goStringLiteral matches a double-quoted Go string, non-greedily, so the shape
// can be looked for in CODE rather than in prose.
//
// ⚠️ **This row needs it and the other three do not**, because this shape is one a
// diagnostic PRINTS: four `fmt.Printf` lines in this tree explain `n/(n+k)` to a
// reader, and a scan that counted them would report a copy in a file containing
// only a sentence. The sibling rows match shapes nobody writes in prose.
//
// Approximate — it does not understand escapes or raw backquoted strings — which
// is the right trade for a tripwire: the failure it can produce is a MISSED copy
// hidden inside a string, and a copy inside a string does not execute.
var goStringLiteral = regexp.MustCompile(`"[^"]*"`)

func isSaturatingRatio(line string) bool {
	m := saturatingRatio.FindStringSubmatch(goStringLiteral.ReplaceAllString(line, `""`))
	return m != nil && m[1] == m[2]
}

// TestTheCopiedExpressionsHaveOneImplementation.
//
// # What this guards, and why a source scan rather than a runtime check
//
// Three quantities that were consolidated onto one implementation, and are now
// held there by shape rather than by hope. Each of them had already been written
// out more than once, and in two of the three the copies had DIVERGED before
// anyone noticed. `cmd/armband` and `cmd/priorblend` both derived the previous
// season's end year from the season's END while the canonical
// `backtest.PriorSeasonName` derives it from the START, so `"2024-30"` gave
// "2023-29" from the two copies and "2023-24" from the one implementation — they
// agreed on every well-formed input and nowhere else. And the archive-side
// constructions of the prior projection all omitted `DefCon` while the live path
// carried it, so an experiment's ordering statistics were computed against a prior
// differing from the shipped one by a whole statistic, for reasons nobody had
// chosen.
//
// The failure this stops is not a rewrite. It is somebody needing the quantity in
// a fifth place and writing the four lines inline, because an import felt heavier
// than the arithmetic. Copies agree on the day they are written — which is exactly
// why a runtime equivalence check cannot catch it and a source scan can. It is
// the same guard as [TestTheMiddleValueHasOneImplementation] beside it — sharing
// its walker, and weaker on one axis, see the limits below — and the same guard as
// `TestTheSharedCellQuantitiesHaveOneImplementation` in `internal/snapshot` one
// language over, except that one scans NAMES, which is the distinction the next
// section rests on.
//
// # It matches the idiom, not the name
//
// That is the lesson the median guard paid for: nine of its eleven copies had no
// name at all, so a scan for the word "median" would have found two. A name-only
// scan here is defeated by anyone who calls his copy `bestAndSecond` or
// `previousSeason`, which is precisely what a fresh author does. So each row below
// asks what an UNNAMED copy would look like — a tuple assignment that demotes the
// leader, the archive's season format string, a `PriorPlayer` literal with fields
// in it — and matches that.
//
// ⚠️ **Each row is keyed on one spelling, and that is a known limit rather than a
// proof.** `if p.Score > captain { vice = captain; captain = p.Score }` writes the
// top two without the tuple assignment; a one-line function body escapes the
// line anchors; a season name built with `strconv.Itoa` escapes the format
// literal; a projection assembled field by field onto a pre-declared variable
// escapes the composite literal. None of those exists in the tree today — checked
// when this was written — so they are tripwires, and the reason they are worth
// having anyway is that the shapes below are what somebody *reaching for the
// obvious* writes.
//
// ⚠️ **Two live near-misses, named so nobody reads the paragraph above as "the
// blind spot is empty".** `backtest.captainAndVice` is the same running top-two
// over the same `Score`, and its own doc says so — it escapes row 1 because its
// tuple targets are PAIRED (`viceScore, vice = capScore, captain`), which the
// first-equals-last test declines. `cmd/armband`'s `priorSeasonName` is the same
// year decrement as row 2, escaping it by emitting the four-digit form. Both are
// out of each row's stated `quantity` — one returns ids rather than a value, the
// other derives a season from a clock rather than from a season name — so neither
// is an offender, and neither is folded in here: `captainAndVice` sits on the
// replay's scoring path where tie order is load-bearing, and moving it is a
// behaviour change rather than a deduplication. They are recorded because they are
// the templates the NEXT copy gets written from.
//
// ⚠️ **Sanctions are counted per FILE, not pinned to an occurrence**, which is
// weaker than the median guard's key-on-the-expression. It has to be: the two
// sanctioned occurrences in `squad.go` are the same 27 characters, so there is no
// text to tell them apart. A third copy added to a sanctioned file *while one of
// its sanctioned copies is deleted in the same edit* therefore passes. Counting
// LINES is a second such seam — two occurrences written on one line read as one.
// Both are visible in a diff of the file, which the pinned key was never needed
// for; what neither the key nor the count can see is a copy in a file nobody
// diffed, and that is the case this scan does catch.
func TestTheCopiedExpressionsHaveOneImplementation(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skip(err)
	}
	srcs, err := goSources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) == 0 {
		t.Fatal("no Go sources found; this guard is scanning the wrong tree")
	}

	for _, c := range []copiedExpression{{
		quantity: "the eleven's value: every score summed, plus the armband again, " +
			"plus ViceCaptainWeight times the runner-up (analysis.xiValueShrunk)",
		cost: "The armband terms are max and second-max, so this is the one part " +
			"of the objective that is NOT a plain sum — and it feeds an argmax, " +
			"where a one-ULP difference flips a player and changes a replayed " +
			"season. Call xiValueShrunk, or xiValue for the unshrunk armband.\n\n" +
			"This shape is a running top-two of ANYTHING. If yours ranks something " +
			"other than an eleven by Score — cells in a diagnostic, seasons in a " +
			"table — it is not this quantity and the advice above does not apply: " +
			"add it to `sanctioned` with that argument.",
		match: isRunningTopTwo,
		sanctioned: map[string]sanction{
			"internal/analysis/squad.go": {5, "" +
				"Five, and the count is positional — this guard excuses the first " +
				"n matches in FILE order, so the list below is in that order and " +
				"not in order of importance. foldPair twice (lines ~117 and ~122), " +
				"then xiValueShrunk, the one implementation, then xiValueOfParts, " +
				"then bestFormation's prefix-record builder.\n\n" +
				"foldPair and the prefix builder are one mechanism. The formation " +
				"loop replayed its eleven players for each of eight formations; " +
				"the builder now folds each position's players once into a prefix " +
				"record and foldPair merges two such records, so a formation's " +
				"total is a constant-time fold of four. Both carry a running " +
				"top-two because the armband is max and second-max, and a record " +
				"has to hold both to be foldable at all — neither can call " +
				"xiValueShrunk for the reason xiValueOfParts exists, that " +
				"materialising the eleven is the cost being removed. Two things " +
				"keep them exact: the scores are still summed in GKP, DEF, MID, " +
				"FWD order, and the fold's equivalence to sequential play is " +
				"pinned by TestThePairFoldMatchesSequentialPlay over 200k heavily " +
				"tied sequences. ⚠️ The ties are why that test exists and why " +
				"this entry is not merely 'it is a top-two': the sequential update " +
				"does NOT promote on equal scores, so a plain max/second-max " +
				"diverges on a tie, and the argmax above it turns that into a " +
				"different footballer.\n\n" +
				"xiValueOfParts is a deliberate copy with a live reason: " +
				"bestFormation " +
				"evaluates eight candidate formations per XI, and materialising an " +
				"eleven-player slice for each purely to hand it to xiValue was one " +
				"of the allocations behind the objective's recorded 176 KB and 61 " +
				"blocks per evaluation — the profile that made the optimiser " +
				"GC-bound rather than compute-bound. Folding it in is not " +
				"free either — it walks four sorted position slices in GKP, DEF, " +
				"MID, FWD order, and floating-point addition is not associative, so " +
				"summing the same scores in a different order lands a ULP away and " +
				"the argmax above it turns that into a different footballer. " +
				"Re-checked when this guard was written: bestFormation still calls " +
				"it inside the formation loop, so the reason still holds."},
			"internal/analysis/pairfold_check_test.go": {2, "" +
				"seqFold, the sequential reference the fold above is checked " +
				"against, and the two-line normalisation that orders the random " +
				"prior state it is checked from. seqFold is the same argument as " +
				"refXIValue below — a reference implementation that called the " +
				"thing it checks would check nothing — and it is what licenses " +
				"the foldPair and prefix-builder copies in squad.go, so folding " +
				"it into a call to foldPair would destroy the evidence for the " +
				"copies it exists to justify."},
			"internal/analysis/optimizerdiff_test.go": {1, "" +
				"refXIValue, the frozen differential oracle. It is copied verbatim " +
				"on purpose and its own comment forbids sharing code with the " +
				"implementation — an oracle that calls the thing it checks tests " +
				"nothing. Listing it here means 'tidying' it into a delegation " +
				"drops the count and fails this test, which is the outcome that " +
				"file asks for."},
		},
	}, {
		quantity: "the season before a given one, in the archive's YYYY-YY form " +
			"(backtest.PriorSeasonName)",
		cost: "Three implementations of this parse coexisted and disagreed: one " +
			"derived the new end year from the season's END and the canonical one " +
			"from its START, so they agreed only on well-formed input. The name is " +
			"a cache key and a URL path segment, so a copy that emits the wrong " +
			"width makes every older season a 404 and the multi-season blend " +
			"silently degrades to the single season it was meant to improve on.",
		match: archiveSeasonFormat.MatchString,
		sanctioned: map[string]sanction{
			"internal/backtest/replay.go": {1,
				"PriorSeasonName itself, which is where the arithmetic lives."},
			"cmd/armband/main.go": {1, "" +
				"seasonBefore normalising a four-digit name into the archive's form " +
				"before delegating, then restoring its caller's width. The FORMAT is " +
				"that caller's concern and the ARITHMETIC is not, which is the split " +
				"that fixed the regression recorded in its doc comment — a first " +
				"consolidation forwarded straight through and returned '2025-2026' " +
				"followed by '2024-25'."},
		},
	}, {
		quantity: "a source's season totals projected into analysis.PriorPlayer",
		cost: "This ten-field list has been written out repeatedly and the copies " +
			"disagreed. All four archive-side constructions omitted DefCon while " +
			"the live path carried it, and three of the four omitted the " +
			"capability flags as well; cmd/priorblend's two channel arms omitted " +
			"DefCon too; and the pair inside internal/priors agreed, which is worse " +
			"— it was found by looking rather than by anything failing. So the same " +
			"footballer reached the blend as different numbers depending on which " +
			"binary asked, and every copy was correct in isolation, so no test " +
			"caught it. One projection per SOURCE, and nothing else builds the " +
			"literal.\n\nIf what you wrote is not a projection of a source — a " +
			"fixture, a container, a transformation of an existing prior — say so " +
			"in `sanctioned` with the argument, rather than deleting the row.",
		// A test fixture inventing a player is not a projection of a source — it
		// has no source — so scanning tests here would mean a dozen exemptions
		// that say nothing. The failure guarded is a source ADAPTER growing a
		// second field list, and adapters are not test files.
		skipTests: true,
		match:     isPriorProjection,
		sanctioned: map[string]sanction{
			"internal/backtest/simulate.go": {1,
				"backtest.PriorFrom — the archive's projection."},
			"internal/priors/adapter.go": {1,
				"priors.priorFrom — the CSV/JSON season mirror's projection. It was " +
					"two literals until this change; LoadBlended now calls it."},
			"internal/recent/priors.go": {1,
				"the projection of FPL's history_past, which spells its fields " +
					"differently again and so cannot share a function with the others."},
			"internal/analysis/priorblend.go": {1, "" +
				"BlendPriors' own RESULT, assembled from weighted sums across " +
				"seasons. It is what the projections feed, not another of them."},
			"cmd/priorblend/main.go": {1, "" +
				"graftRates, which rescales one prior's rates onto another's " +
				"minutes base. A transformation of a PriorPlayer rather than a " +
				"projection of a source, and it is one function because its two " +
				"call sites are exact mirrors."},
		},
	}, {
		quantity: "the option-value decay: how much of a held option's worth " +
			"survives, given the gameweeks of exercise window left " +
			"(analysis.OptionDecay)",
		cost: "Four levers price a held option — a banked transfer, a wildcard, a " +
			"bench boost and a free hit — and every one of them was a CONSTANT " +
			"before this curve existed. Four copies of the curve is the same " +
			"failure one layer up, and it is worse than the usual case because " +
			"the copies would agree on the day they were written and then be " +
			"re-tuned separately: the whole argument for one curve is that the " +
			"four are the same quantity, so a second implementation quietly " +
			"withdraws the argument while the code still compiles.\n\n" +
			"Call analysis.OptionDecay, or one of the three faces above it — " +
			"TransferHoldFactor, ChipReservationAt, ChipBarAt — which add a base " +
			"price and nothing else.\n\n" +
			"This shape is a saturating ratio of ANYTHING. If yours is a share, a " +
			"probability or a shrinkage weight rather than an option's remaining " +
			"life, it is not this quantity: add it to `sanctioned` with that " +
			"argument.",
		match: isSaturatingRatio,
		sanctioned: map[string]sanction{
			"internal/analysis/optionvalue.go": {1,
				"OptionDecay itself, which is where the curve lives."},
			// The seven live shrinkage weights. They are the same ALGEBRA and a
			// different QUANTITY: a shrinkage weight puts EVIDENCE against a
			// prior, where the option curve puts remaining TIME against a
			// half-life. Neither would ever be tuned by looking at the other, so
			// folding them together would be a pun rather than a deduplication.
			//
			// ⚠️ Listing them is what makes this row a tripwire rather than
			// noise: with seven known occupants accounted for, an EIGHTH anywhere
			// fails, and that eighth is the copy this row exists to catch.
			// (4 + 1 + 2 = 7. An earlier draft of these two lines said six and
			// seventh, which is the count a reader would check the row against.)
			"internal/analysis/blend.go": {4, "" +
				"the rate blend's own weight, four times: the shrink-to-league " +
				"arm on n, and the recency-weighted arm, the flat arm and the " +
				"assembled result on n90. All four weigh EVIDENCE against a " +
				"prior, on BlendRateK or LeagueShrinkK."},
			"internal/analysis/metrics.go": {1,
				"the same blend weight, clamped, on the metrics path."},
			"internal/analysis/teamstrength.go": {2, "" +
				"the club prior's two weights, on matches played against " +
				"teamConcededK and teamScoredK. This record already notes that " +
				"team strength wants a far heavier prior than player rates " +
				"(k=70/35 against 8/5), which is the clearest statement that " +
				"these are not one constant with the option curve's."},
		},
	}} {
		seen := map[string]int{}
		var offenders []string
		for _, f := range srcs {
			if c.skipTests && strings.HasSuffix(f.rel, "_test.go") {
				continue
			}
			// This file carries the patterns and the sanction list, so it would
			// match itself — the same reason the sibling scanners skip their homes.
			if f.rel == "internal/stats/copies_test.go" {
				continue
			}
			for i, line := range strings.Split(f.body, "\n") {
				// A line that is entirely a comment cannot be a second
				// implementation of anything, and these files DISCUSS the shapes
				// they carry at length — the sanction notes below quote the season
				// format, and squad.go's doc comment names the arithmetic it must
				// not repeat. Without this, documenting the rule breaks it. Same
				// skip, for the same reason, as the raw-read scan in
				// `TestTheSharedCellQuantitiesHaveOneImplementation`.
				//
				// ⚠️ Line comments only. A copy commented out with `/* */` is
				// neither caught nor excused; it is also not code.
				if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//") {
					continue
				}
				if !c.match(line) {
					continue
				}
				seen[f.rel]++
				if seen[f.rel] <= c.sanctioned[f.rel].n {
					continue
				}
				offenders = append(offenders,
					f.rel+":"+strconv.Itoa(i+1)+"  "+strings.TrimSpace(line))
			}
		}
		if len(offenders) > 0 {
			t.Errorf("a second implementation of %s:\n  %s\n\n%s",
				c.quantity, strings.Join(offenders, "\n  "), c.cost)
		}
		// The debt list must shrink. A sanctioned file that no longer carries its
		// occurrences has had the copy folded in — good — and leaving it listed
		// records a debt that has been paid, which is how a debt list stops being
		// read.
		var stale []string
		for rel, s := range c.sanctioned {
			if seen[rel] < s.n {
				stale = append(stale, fmt.Sprintf("%s carries %d occurrence(s), "+
					"listed as %d — %s", rel, seen[rel], s.n, s.why))
			}
		}
		sort.Strings(stale)
		if len(stale) > 0 {
			t.Errorf("recorded duplication of %s has been removed but is still "+
				"listed:\n  %s\n\nLower the count in `sanctioned` above, or delete "+
				"the entry. A debt list that overstates the debt stops being read.",
				c.quantity, strings.Join(stale, "\n  "))
		}
	}
}
