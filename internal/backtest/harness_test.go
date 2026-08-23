package backtest

// The shared replay harness: the season grid, the season loader, and the two
// SimConfigs a diagnostic can legitimately want.
//
// # Why this exists
//
// Before it, the four season pairs appeared verbatim in eleven files, the
// six-entry-point grid in five, the SimConfig literal in eight, and the season
// loader — each with its own cache — in five. Counts are grep-verified against the
// commit before this change, because a count is the claim nobody re-checks.
// That is the same failure this package has hit repeatedly at smaller scale —
// `DefaultBenchWeight` against `Weights.BenchWeight`, `fixtureSensitivePart`
// drifting from `baseXP90`, and most recently two copies of the season-clustered
// standard error, one of which had already been retired while still producing a
// figure quoted in AGENTS.md. Copies do not stay equal, and nothing notices.
//
// Concretely: adding a fifth season pair meant eleven edits, and a diagnostic
// that missed one would silently measure a different population from the sweep it
// was being compared against.
//
// # What is deliberately NOT unified
//
// The literals were not all identical, and two of the differences are real. One
// constructor covering both would have quietly changed what several diagnostics
// measure, which is worse than the duplication:
//
//   - **The bank limit.** A *sweep* pins `sweepBankLimit` (5) everywhere so cells
//     governed by different transfer rules stay comparable. A diagnostic
//     *reporting what a season would actually have scored* wants
//     `BankLimitFor(season)`, or 2022-23 and 2023-24 get simulated saving
//     transfers nobody could save. That is the distinction `BankLimitFor` exists
//     for, so it gets two named constructors rather than a boolean.
//   - **`WeeklyXI`.** Some diagnostics set it and some do not, and it changes
//     which eleven is fielded. It is a parameter here, so a call site states its
//     choice rather than expressing it by omission from a six-line literal.
//
// The point is not fewer lines. It is that the *differences* between call sites
// are now visible in one argument each, where before they were buried in
// near-identical blocks nobody diffed.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"armband/internal/config"
)

// sweepPairNames is the replayed season grid: {priors, season played}.
//
// 2021-22 appears only as a prior. It cannot be *played* in any measurement that
// touches xG — the archive carries no expected_goals before 2022-23, so the model
// runs crippled and an intervention that scales xG returns byte-identical output,
// which reads as "no effect" and is not.
//
// **That last sentence is now false, and the default is six seasons.** `xgRepairs`
// backfills expected goals and assists into 2019-20, 2020-21 and 2021-22 from
// Understat, so those seasons are no longer blind, and the measurement pass
// extendedPairNames demanded has been run — see gridwidth_test.go for the
// pre-registration and stats/snapshots/2026-08-11-6acc5ad for the cells.
//
// # Why the "incomparable with itself" objection did not survive
//
// The four pairs below were kept for a long time on the argument that every figure
// in AGENTS.md is measured on them, so widening the default would invalidate the
// record at a stroke. That argument was **wrong on a checkable point**: the shipped
// four are a strict subset of the extended six, and the cells they produce inside a
// six-season run are *byte-identical* to an independently run four-season sweep —
// 48 of 48 overlapping cells agreeing on every outcome column, and 192 across all
// arms. So no published figure is invalidated. Each remains correct **as a
// four-season figure**, and the record needs annotating rather than re-deriving.
//
// What the widening buys is degrees of freedom, which no number of entry points can
// move: 3 to exactly 5, dropping the smallest detectable effect from 12.4 to 8.4
// points a season on the positive control's HOLD arm.
//
// **Read the shape rather than that ratio.** Across all fifteen four-season subsets
// of the six the same threshold ranges 8.2 to 16.0 (median 12.8; the shipped four
// happen to land at 12.4), so any single four-season figure carries about ±30% of
// which-seasons-you-got. All six five-season subsets fall between 8.4 and 11.0 and
// every one beats the median four-season subset — monotone across grid width, which
// is the standard this package demands instead of an argmax.
//
// ⚠️ **Two corrections from review, both of which this comment had wrong.** First,
// this is a HOLD result: on POLICY the same control's threshold *rose*, 14.4 to
// 16.1, because its between-season spread nearly doubled when 2021-22 arrived. So
// transfer settings want FPL_SWEEP_SEASONS=default until more POLICY arms with real
// effects have been run on both grids.
//
// ⚠️ **That last sentence was SUPERSEDED on 2026-08-13 and is left standing because
// it is the thing being retracted.** The POLICY arms it asks for have been run: ten
// more, four of them min_gain — a transfer setting — and widening helps 10 of 11,
// median ratio 0.62. One arm was the exception and it does not generalise. **Sweep
// transfers on six too.** FPL_SWEEP_SEASONS=default is for reproducing a recorded
// figure on its own grid, not for choosing one. See AGENTS.md, "What the harness can
// resolve"; this comment is what a sweeper reads before picking a grid, so the two
// disagreeing silently was the cost.
//
// Second, the claim that the reconstruction's
// extra noise is "invisible in the clustered standard error because that is driven
// by how much whole seasons disagree" is **false**. Within-season noise enters the
// clustered variance as the sampling error of each season's own mean: it is 37% of
// the four-season clustered variance and 57% of the six-season one. Priced, the
// borrowed offset costs about 1 point a season of threshold — 7.4 rather than 8.4 —
// which is ~12% and a lower bound. Small and real, rather than absent.
//
// `FPL_SWEEP_SEASONS=default` still returns the four, and is how any figure in the
// record gets reproduced on the grid it was measured on. Note the cost of the
// switch: every default sweep is now 36 cells rather than 24, so half again the
// compute.
func sweepPairNames() [][2]string {
	switch g := strings.TrimSpace(os.Getenv("FPL_SWEEP_SEASONS")); g {
	case "", "extended":
		return extendedPairNames()
	case "default":
	case "scoring":
		// HOLD-only, and runPolicySweep refuses to report POLICY on it rather than
		// trusting an operator to remember. See the guard in reportPairedDifferences'
		// caller: this grid plays 2019-20, whose transfer path is not a sample of the
		// same process, and a POLICY figure computed over it would look exactly like
		// every other POLICY figure in the record.
		return scoringPairNames()
	default:
		panic("FPL_SWEEP_SEASONS=" + g + ": expected \"default\" (the historical four), " +
			"\"extended\" (the six that now ship) or \"scoring\" (seven, HOLD-only). " +
			"Silence is not allowed to read as success here — an unrecognised grid would " +
			"silently fall through to whatever the default happens to be, and every figure " +
			"would carry a season count its operator never chose.")
	}
	return [][2]string{
		{"2021-22", "2022-23"}, {"2022-23", "2023-24"},
		{"2023-24", "2024-25"}, {"2024-25", "2025-26"},
	}
}

// extendedPairNames is the six-season grid the Understat backfill makes possible.
//
// **This is what sweepPairNames now returns**, and it stays a named grid so the
// distinction is readable from the name rather than inferred from two extra rows —
// the xgPairNames precedent.
//
// It reached the default the long way round. The standing objection was that a
// backfilled season is a noisier cell rather than an equivalent one, so pooling it
// changes what a paired difference estimates as well as how precisely, and that
// wiring it in wanted its own measurement pass. **The pass was run** — see
// gridwidth_test.go's pre-registration and stats/grid_width.R — and the objection
// did not survive it: the extra noise lands in the within-season spread, which the
// season-clustered standard error divides away, and not in the between-season spread
// that drives it.
//
// # What it buys, which is degrees of freedom rather than precision
//
// The binding constraint on this whole enterprise is that four seasons give the
// season-clustered standard error **three** degrees of freedom, so the 5% critical
// value is 3.18 rather than 2. Two more seasons take df to 5 and t_crit to 2.571,
// which scales the detection threshold by t_crit(S-1)/sqrt(S): 0.66 of today's, so
// the canonical median of 39 points a season becomes about 26. Six pairs need 2019-20
// as a prior, which is why the backfill covers three seasons to add two playable ones.
//
// For comparison, the measured alternative — densifying the entry-point grid — buys
// 20% off the standard error at twelve entry points and **cannot move the df at all**.
//
// # Four caveats that travel with it
//
// Review found two more than the two originally recorded here, and the third is the
// one most likely to mislead: it silently disables a scoring term rather than adding
// noise to one.
//
// **2019-20 is `HOLD`-only.** FPL granted unlimited free transfers before the GW30+
// deadline and froze prices for three months, so its transfer path and its wallet are
// not samples of the same process as the other seasons'. Its *scoring* is fine, which
// is exactly the split between the two metrics this package already draws. It appears
// here only as a prior for that reason; a seventh pair playing it belongs in a
// HOLD-only grid.
//
// **The three backfilled seasons carry xG on a borrowed provider offset**, because
// they have no FPL expected-goals column to fit one against — see xgRepairMeta. The
// level error that leaves is the benign kind (shared by every player in a season, and
// an argmax consumes an ordering); the per-player dispersion it leaves is not, and no
// rescaling removes it. Priced at about **1 point a season of detection threshold,
// ~12%**, and that is a lower bound; see sweepPairNames.
//
// ⚠️ **Superseded by the xGC repair at `7cb769e` (2026-08-12); this block was
// written the day before it and describes the archive as it was.**
// `applyXGCRepair` reconstructs expected goals conceded for exactly these
// seasons from their opponents' repaired xG, is **on by default**
// (`FPL_NO_XGC_REPAIR` is an opt-out, read on every `Load`), and takes coverage
// from 0% to ~100% there — so the clean sheet and the goals-conceded deduction
// are **live** in 2020-21 and 2021-22 on any run after that date. The cell count
// below is wrong in its own terms too: the hole was **18 of 36 cells, not 12**,
// because 12 counts only the fully blind seasons and misses 2022-23, whose
// GW1-15 rows every entry sees since `PointInTime` accumulates from GW1. See
// `xgcrepair.go`'s header, which is the current statement.
//
// **Kept rather than deleted**: this is the pre-repair baseline, and a review
// finding has already been built on it and correctly declined.
// **Two of the six played seasons have NO CLEAN SHEET TERM AT ALL, and this is the
// widest of the four gaps.** `xgRepairs` fills expected goals and assists and
// deliberately does not fill expected goals *conceded* (see xgrepair.go), because
// Understat publishes a team figure where the model needs the per-player one — which
// carries the substitution channel a club rate would delete. Verified against the
// archive: weekly `expected_goals_conceded` is exactly zero in 2019-20, 2020-21 and
// 2021-22, and begins at GW16 in 2022-23 — the same cutoff as xG and `starts`, so
// that is one archive event and not three. `baseXP90` gates both the clean sheet and
// the goals-conceded deduction on `XGC90 > 0` (metrics.go:1885, :1892), so in
// 2020-21 and 2021-22 defenders and keepers are scored with neither, and the clean
// sheet is 26-45% of their points.
//
// The archive facts behind it stand: weekly `expected_goals_conceded` is exactly zero in
// 2019-20, 2020-21 and 2021-22 and begins at GW16 in 2022-23 — the same cutoff as xG and
// `starts`, so one archive event and not three — and `baseXP90` gates both the clean
// sheet and the goals-conceded deduction on `XGC90 > 0`, so those seasons were scored
// with neither term. Realised points were unaffected; the model **mis-picked** there.
//
// ⚠️ **The count was also wrong, and by half a season's worth of cells: it is 18 of 36,
// not 12.** The 12 counts *fully* blind played seasons and does not survive the hole in
// 2022-23 being partial — `PointInTime` accumulates from GW1, so all six 2022-23 entries
// see the repaired GW1-15 rows. Measured 2026-08-13 by sweeping `FPL_NO_XGC_REPAIR` as
// two separate processes: exactly 18 cells move and exactly 18 are byte-identical. See
// stats/snapshots/2026-08-13-4d61058/. Anything acting through the clean sheet was inert
// in all eighteen, and `FPL_CS_XGC_FACTOR`, `DefConCleanCoupling` and the defensive half
// of `FPL_DEF_FIXTURE_SCALE` were all swept under that dilution.
//
// **`starts` is reconstructed in all three backfilled seasons**, being absent as a
// column before 2022-23 — about 8,200 / 7,400 / 7,000 rows. reconstructStarts' own
// third boundary says never to use it as evidence about a rotation or returning
// player, and it is biased 3:1 toward making fringe players look nailed. Anything
// about minutes, rotation or blanking now takes a third of its cells from that
// population — including the grid-width pass's own positive control, which is why
// P4's native/backfilled split cannot separate the offset from the reconstruction.
func extendedPairNames() [][2]string {
	return [][2]string{
		{"2019-20", "2020-21"}, {"2020-21", "2021-22"},
		{"2021-22", "2022-23"}, {"2022-23", "2023-24"},
		{"2023-24", "2024-25"}, {"2024-25", "2025-26"},
	}
}

// scoringPairNames is the seven-season grid, and it is `HOLD`-only by construction.
//
// A *fourth* named grid rather than a seventh row on extendedPairNames, because the two
// answer different questions and the difference should be readable from the name. This
// one adds {2018-19, 2019-20}, which is only possible at all because the loader now
// accepts a season with no teams.csv as prior-only — 2018-19 publishes players,
// fixtures and gameweeks and no clubs, so it can be a prior and can never be played.
//
// # Why it cannot be used for POLICY, and why that costs nothing
//
// The pair it adds *plays* 2019-20, and TransferPathComparable says no: FPL granted
// unlimited free transfers before the GW30+ deadline after the COVID restart and froze
// prices for three months, so that season's transfer path and wallet are not samples of
// the same process. Its scoring is fine.
//
// That is why this grid gains nothing for POLICY, and the arithmetic is worth stating
// plainly rather than left to be assumed: POLICY has six usable seasons with or without
// 2018-19, since the season 2018-19 unlocks is the one POLICY has to exclude anyway.
//
// # The degrees of freedom, which is the entire point
//
// The binding constraint is that the season-clustered standard error rests on S-1
// degrees of freedom and no number of entry points moves it. The threshold scales as
// t_crit(S-1)/sqrt(S):
//
//	seasons       df  t_crit  scale vs today  the canonical 39/season becomes
//	4 (historical) 3   3.182   1.000           39
//	6 (ships)      5   2.571   0.660           26
//	7              6   2.447   0.581           23
//
// and `HOLD`'s own 33 becomes about 19. For comparison, the measured alternative —
// densifying the entry-point grid — buys 20% off the standard error at twelve entry
// points and cannot move the df at all.
//
// # What that arithmetic assumes, and it is not free
//
// It assumes the seventh cell is as quiet as the four that ship, and a backfilled season
// is **not**: its xG sits on a *borrowed* provider offset, and the per-player dispersion
// two xG models leave — the record puts the p90 of the per-player ratio at 1.54 — is an
// ordering error no rescaling removes. So read 23 as the figure if the extra cells were
// equivalent, and treat the real gain as smaller by an amount nobody has measured.
//
// **Nothing uses this by default**, and TestTheGridIsDeclaredOnce is the reason that
// matters: a diagnostic measuring a different season population from the sweeps it is
// quoted beside is a silent failure, so wiring this into a sweep is a decision that wants
// its own measurement pass.
func scoringPairNames() [][2]string {
	return append([][2]string{{"2018-19", "2019-20"}}, extendedPairNames()...)
}

// xgPairNames is the grid for a diagnostic that needs expected goals in the
// *prior* season as well as the played one.
//
// It drops the 2021-22 → 2022-23 pair, because 2021-22 carries no
// `expected_goals`, `expected_assists` or `expected_goals_conceded` at all. A
// diagnostic that builds a model from the prior season and scores it on the next
// would be reading zeroes as evidence there, which is not a weaker measurement but
// a different one.
//
// It is a *named* second grid rather than a literal pasted into two files: the
// distinction is real, and the reason it exists should be readable from the name
// instead of inferred from a missing row. `saves_clean_test.go` and the
// blend-drift block in `calibration_test.go` are the two that want it.
func xgPairNames() [][2]string {
	return [][2]string{
		{"2022-23", "2023-24"}, {"2023-24", "2024-25"}, {"2024-25", "2025-26"},
	}
}

// sweepStarts is the six entry points behind every paired result.
//
// ⚠️ This said "behind every 24-cell result", and that the scarce axis was "four
// seasons with xG and there will never be more". Both were true when written and
// neither is now: `extendedPairNames` ships six pairs, so the default sweep is 36
// cells, and the Understat backfill is exactly the "there will never be more"
// that happened. A present-tense definition is the one place a reader looks to
// find out what a default sweep costs, so it is corrected here rather than
// annotated. The counts are deliberately not restated below — `gridLabel` derives
// them from what runs, and a second copy in prose is what went stale.
//
// FPL lets a manager join at any deadline with a fresh £100m, so each is a real
// scenario and a different path through the same football. Seasons are still the
// scarce axis — the archive bounds how many exist, and a backfilled one is a
// noisier cell rather than an equivalent one — and paths are not, which is why
// doubling the start points from three to six moved the transfer threshold from
// "no structure" to a monotone ladder at t = +3.36.
//
// # Why the six are not changed, and how to densify anyway
//
// Every figure in AGENTS.md is measured at these six. Changing the default would
// make every new cell incomparable with the entire record at once, which is the
// same reason FPL_ORACLE_AVAILABILITY must never become the default. So
// densification is opt-in: FPL_SWEEP_STARTS="1,2,3,4,6,11" replaces the grid for
// one run and the six are the value whenever it is unset.
//
// It exists because the arithmetic behind "more entry points shrink the clustered
// SE" — Var(mean) = (sd_season^2 + sd_resid^2/G)/S — assumes the G within-season
// residuals are independent, and entry points are strictly *nested*: SimConfig
// carries StartGW and no EndGW, so every window runs to GW38 and an entry at GW2
// shares 37 of its 38 gameweeks with an entry at GW1. At spacing 5 that
// correlation is evidently tolerable; at spacing 1 it may not be, and a grid of
// near-duplicates would shrink the reported SE without adding information. That
// is the budget-jitter trap, and testing for it needs a grid that over-samples
// short spacings deliberately.
func sweepStarts() []int {
	if s := strings.TrimSpace(os.Getenv("FPL_SWEEP_STARTS")); s != "" {
		return parseSweepStarts(s)
	}
	return []int{1, 6, 11, 16, 21, 26}
}

// parseSweepStarts reads a comma-separated entry-point grid, and panics rather
// than falling back on anything it cannot read.
//
// That is a deliberate departure from the FPL_BENCH_SLOTS pattern, which returns
// the shipped tuple on malformed input. A silently ignored bench shape costs one
// re-run; a silently ignored *grid* produces a complete-looking sweep at the
// shipped six while its operator believes it is dense, and the whole result — the
// correlation-versus-spacing table this switch exists for — would be computed on
// a population that never contained a short spacing. Silence is not allowed to
// read as success here.
func parseSweepStarts(s string) []int {
	var out []int
	seen := map[int]bool{}
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > 38 {
			panic(fmt.Sprintf("FPL_SWEEP_STARTS=%q: %q is not a gameweek in 1..38", s, f))
		}
		if seen[n] {
			panic(fmt.Sprintf("FPL_SWEEP_STARTS=%q: gameweek %d appears twice, which "+
				"would pool one cell with itself", s, n))
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) < 2 {
		panic(fmt.Sprintf("FPL_SWEEP_STARTS=%q: need at least two entry points", s))
	}
	sort.Ints(out)
	return out
}

// gridLabel names the grid a diagnostic is ABOUT TO RUN — "36 cells (6 seasons
// x 6 starts)" — from the counts it is actually iterating.
//
// A printed label is a second implementation of the grid, and one quantity with
// two implementations that then stop agreeing is this package's signature
// failure. Diagnostics across this package carried "24 cells (4 seasons x 6
// starts)" as a literal. How many such LABELS there were, and the method that
// establishes the figure, are in `TestPrintedGridLabelsAreDerived`'s doc comment
// and are deliberately not restated here — a bare count in a second place is the
// very defect this function exists to stop, and the number that used to sit here
// counted diagnostics rather than labels, so the two were never the same quantity.
// `sweepPairNames` (the {prior, played} season pairs every sweep runs)
// returns six pairs by default since the Understat backfill landed, so each such
// diagnostic ran 36 cells while printing a four-season label — and
// FPL_SWEEP_SEASONS/FPL_SWEEP_STARTS move both counts inside a single run, which
// no literal can follow.
//
// It takes counts rather than the slices because the two numbers also arrive
// from `playedSeasons` (a season list filtered by what a diagnostic needs) and
// from local literals, and one formatter for the label is the point.
//
// ⚠️ **A label for a RECORDED result is not this.** A figure measured on the
// four-season grid and quoted for comparison must keep saying 24: deriving it
// would relabel history with today's grid, which is the same mislabel from the
// other side. `TestPrintedGridLabelsAreDerived` holds those exemptions, one
// line each with its reason.
func gridLabel(seasons, starts int) string {
	return fmt.Sprintf("%d cells (%d seasons x %d starts)", seasons*starts, seasons, starts)
}

// seasonsLabel names a season population for a diagnostic that iterates seasons
// and has no entry-point axis — a calibration fit rather than a paired sweep.
func seasonsLabel(seasons int) string {
	return fmt.Sprintf("%d seasons", seasons)
}

// loadConfig reads the real config the replay measures against.
//
// **The error is checked, which the twenty-two call sites this replaced all
// discarded.** `config.Load` returns `Default()` alongside a parse error, so a
// malformed `config.json` silently made every DIAG figure come from the built-in
// defaults rather than the shipped config — plausible output measuring something
// other than what it claims, which is this package's signature failure. Absent is
// not an error (Load writes the defaults and returns nil), so this cannot fire on
// a fresh checkout.
func loadConfig(t *testing.T) config.Config {
	t.Helper()
	path := configPath(t)
	// Statted before Load, because Load creates what is missing and returns no
	// error — see configPath. A diagnostic measuring config.Default() while
	// reporting the shipped config is exactly the silence this guards.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not readable at %s: %v\n"+
			"Every figure below would otherwise come from config.Default() rather "+
			"than the shipped config, with nothing to say so. "+
			"Set FPL_CONFIG to point elsewhere.", path, err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("loading %s: %v\n"+
			"Every figure below would otherwise come from config.Default() rather "+
			"than the shipped config, with nothing to say so.", path, err)
	}
	// CacheDir ships relative and would otherwise resolve against this package's
	// directory, re-downloading the whole archive into internal/backtest/.cache
	// rather than reusing the repository's.
	if !filepath.IsAbs(cfg.CacheDir) {
		cfg.CacheDir = filepath.Join(filepath.Dir(path), cfg.CacheDir)
	}
	return cfg
}

// configPath resolves the shipped config every diagnostic measures against.
//
// It was a hardcoded absolute path from one developer's machine, and the wart was
// larger than the comment here allowed. `go test` runs with the package directory
// as its working directory, so the path is resolved against this package rather
// than left relative — which is the property the absolute form was reaching for,
// without the machine.
//
// **The hole it closes is that absence is survivable.** `loadConfig` checks the
// error, but `config.Load` treats a missing file as ordinary: it *writes* the
// defaults there and returns nil. On this Linux box that happens to fail, because
// /Users does not exist — so the fatal fires by luck of the platform. On any Mac
// that is not this one, /Users does exist, the default config gets created at a
// path nobody looks at, and every DIAG figure silently comes from
// `config.Default()` with a clean exit. That is the package's signature failure
// mode wearing its own clothes, and the comment this replaces described it
// accurately while leaving it in place.
//
// So absence is now an error rather than a side effect. FPL_CONFIG overrides, for
// pointing a run at an experimental config without editing fifty call sites.
func configPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("FPL_CONFIG"); p != "" {
		return p
	}
	// Two levels up from internal/backtest is the repository root.
	p, err := filepath.Abs(filepath.Join("..", "..", "config.json"))
	if err != nil {
		t.Fatalf("resolving the repo config path: %v", err)
	}
	return p
}

// seasonPair is one replayed season with the season its priors come from.
type seasonPair struct {
	PriorName, Name string
	Prior, Cur      *Season
}

// The parsed-season cache is process-global, so a test running several sweeps
// parses each archive once rather than once per sweep. Lock-guarded because this
// package's standing rule is that anything mutable and shared is — a map built
// under a plain nil check is a *fatal* concurrent map write and one has already
// taken down a live run.
//
// **Callers must treat a returned *Season as read-only.** Sharing one parse
// between tests is only safe while nothing writes to it, and a test that edited a
// player's gameweeks would silently change what every later test in the process
// measures — the contamination would look like a real effect, since the archive is
// the population. Verified at the time this cache landed: no diagnostic mutates a
// loaded season, and the tests that *do* edit players (replay_test.go,
// simulate_test.go) build synthetic ones locally, which is the right pattern and
// costs nothing. If a diagnostic ever needs to perturb an archive, deep-copy it
// rather than reaching for Load to get a private parse — the next reader will
// assume this cache is in play.
var (
	seasonMu    sync.Mutex
	seasonCache = map[string]*Season{}
)

func loadSeason(t *testing.T, cfg config.Config, name string) *Season {
	t.Helper()
	// Keyed on the directory as well as the season. Every caller today passes the
	// same relative CacheDir, so the name alone would be unambiguous — but
	// replay_test.go already loads from "../../.cache/fpl", which holds v2-v4
	// archives, and a diagnostic that passed such a cfg here would receive
	// whichever directory won the race to populate the map. A stale parser's
	// output read as current is the failure this package records as "a cache
	// version is not a schema check", and the key costs nothing.
	key := cfg.CacheDir + "|" + name
	seasonMu.Lock()
	defer seasonMu.Unlock()
	if s, ok := seasonCache[key]; ok {
		return s
	}
	s, err := Load(context.Background(), cfg.CacheDir, name)
	if err != nil {
		t.Fatal(err)
	}
	seasonCache[key] = s
	return s
}

// loadPairs returns every pair in the grid, fully parsed.
//
// Parsing happens up front rather than lazily inside the variant loop so the
// first arm of a sweep is not charged for work every later arm reuses — a sweep
// that looks like it slows down after its first arm is measuring the parser.
func loadPairs(t *testing.T, cfg config.Config) []seasonPair {
	t.Helper()
	var out []seasonPair
	for _, p := range sweepPairNames() {
		out = append(out, seasonPair{
			PriorName: p[0], Name: p[1],
			Prior: loadSeason(t, cfg, p[0]),
			Cur:   loadSeason(t, cfg, p[1]),
		})
	}
	return out
}

// sweepConfig is the config every paired *sweep* cell uses.
//
// The bank is pinned at sweepBankLimit for the reason documented there:
// comparing a setting across cells governed by different transfer rules adds a
// nuisance factor that interacts with the knobs being swept. Absolute totals
// produced this way are therefore not comparable with figures measured under
// BankLimitFor — only the paired differences are.
//
// This and seasonConfig are the **only** two places the oracle environment
// variables are read. They are constructed per cell, so a command line that sets
// FPL_ORACLE_PRICES still gets per-cell semantics, while a diagnostic that wants
// both arms in one process sets SimConfig.Oracles in its variant's apply and
// never touches the environment. An oracle read anywhere deeper than this would
// be back to a process-global bit that concurrent cells share.
func sweepConfig(cfg config.Config, start int, weeklyXI bool) SimConfig {
	return SimConfig{
		Weights:    cfg.Weights,
		MinGain:    cfg.Review.MinGainForTransfer,
		MinGainHit: cfg.Review.MinGainForHit,
		BankUpTo:   sweepBankLimit,
		MaxHits:    cfg.Review.MaxHitsPerWeek,
		Budget:     1000,
		FreeCost:   cfg.Review.FreeTransferValue,
		// ⚠️ The scheduled early floor is deliberately NOT mapped here. The
		// shipped default lives in config and reaches the LIVE path through
		// ReviewPolicy.EffectiveFloor; the sweep baseline stays the flat
		// {2.0, 0.4} machine every banked cell was measured on, and an arm that
		// wants the schedule sets sc.EarlyFloor itself (the scheduled-floor
		// diagnostic does). Mapping it would flip the machine of every
		// diagnostic that registered the flat one.
		StartGW:  start,
		WeeklyXI: weeklyXI,
		Oracles:  OraclesFromEnv(),
	}
}

// seasonConfig is the config for a diagnostic reporting what a season would
// actually have scored, on the transfer rule that was in force.
//
// Use this when the output is a description of a season — how often the policy
// banked a transfer, what a move returned — and sweepConfig when the output is a
// contrast between two settings.
func seasonConfig(cfg config.Config, season string, start int, weeklyXI bool) SimConfig {
	sc := sweepConfig(cfg, start, weeklyXI)
	sc.BankUpTo = BankLimitFor(season)
	return sc
}

// blockPicker resolves the EXP environment variable to the sweep blocks a
// diagnostic should run, and reports a failure when EXP names a block this
// diagnostic does not have.
//
// Without the check, a typo — or, far more likely, a block name that belongs to
// a *different* diagnostic — selects nothing. The test then runs no blocks,
// writes no cells and **passes in 0.00s**, which reads downstream as a sweep
// that had nothing to say rather than one that never ran. That is the
// silent-no-op class this package catalogues: a null result indistinguishable
// from a real one.
//
// It is not hypothetical and it cost a snapshot. The recipe in stats/README.md
// paired `EXP=A` with `TestDiagProjection`; block A belongs to
// `TestDiagTransferPolicy`, so the sweep step emitted an empty cells file and
// the renderer silently fell back to a stale one from a previous session. The
// snapshot's own provenance warnings caught it afterwards; this catches it at
// the point of failure.
type blockPicker struct {
	only    string
	names   []string
	matched bool
}

func newBlockPicker() *blockPicker { return &blockPicker{only: os.Getenv("EXP")} }

// want reports whether the named block should run, and records that this
// diagnostic offers it.
func (p *blockPicker) want(name string) bool {
	p.names = append(p.names, name)
	if p.only == "" {
		return true
	}
	if p.only == name {
		p.matched = true
		return true
	}
	return false
}

// check reports a failure when EXP named nothing this diagnostic offers. Call it
// deferred, so the full set of block names has been collected by the time it
// runs. It uses Errorf rather than Fatalf because a deferred Fatalf would
// Goexit out of the deferred call itself.
func (p *blockPicker) check(t *testing.T) {
	t.Helper()
	if p.only == "" || p.matched {
		return
	}
	t.Errorf("EXP=%q names no block in this diagnostic, so nothing ran — and a "+
		"sweep that runs nothing writes no cells and still passes. This "+
		"diagnostic has: %s", p.only, strings.Join(p.names, ", "))
}
