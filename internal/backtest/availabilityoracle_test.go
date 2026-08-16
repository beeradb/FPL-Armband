package backtest

// What is perfect team news worth? Far less than the record claimed — and the
// generalisation of it, the minutes oracle, is where the number turned out to be.
//
// This file was headed "the largest number in this project, measured". It is not:
// against a common baseline this arm reads **≈14 a season held**, and although its
// CR2 t = 3.19 reaches its own threshold of 14, that is p = 0.0497 restated rather
// than a second witness — it fails Holm at 0.149 and is inert in 13 of 24 held cells,
// so ≈14 is a design average against ≈32 conditional on firing. Read it as
// unresolved. The header is left corrected rather than deleted, because "322 is the
// biggest thing here" is the belief the whole diagnostic was built to test.
//
// Nor is the answer that some neighbouring oracle carries the number instead. This
// comment said so for two commits — naming `OracleMinutes` at "+183 held, CR2
// t = 4.30" — and that is a **relabelled** figure, not a live one: the arm behind it
// grants a *season-average* window, which resolves as such but bounds nothing about
// knowing a player's trajectory. The bounded arms of this family are ≈73 for lineups
// (t = 1.32) and ≈47 for minutes (t = 0.62), and **neither resolves either**.
//
//	DIAG=1 EXP=ORACLEAVAIL FPL_CELLS=/tmp/oracleavail/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagAvailabilityOracle$' -v -timeout 3h
//	Rscript stats/sweep_inference.R /tmp/oracleavail/cells.csv
//	Rscript stats/variance_components.R --out=stats/out/oracleavail /tmp/oracleavail/cells.csv
//
// # Why this exists
//
// "Perfect team news is worth +322 points held" is quoted in AGENTS.md and in
// three design documents, where it sets the oracle build order and is described
// as two orders of magnitude above every constant the project argues over. It was
// **unsourced**: it entered the record as a parenthetical with no table, no cell
// count and no metric detail beyond "held", and until this file existed there was
// no diagnostic anywhere in this package that measured it. FPL_ORACLE_AVAILABILITY
// was wired in simulate.go and never run.
//
// Four reasons the magnitude was worth doubting, all of them recorded beside the
// figure and none of them acted on:
//
//   - It predates the doubles-counting fix (+115 a season on POLICY, +106 on
//     HOLD), the defcon-visibility fix and the vice-captain fix. Three
//     contamination events, against this project's own rule that a figure is valid
//     only at the setting of everything it shares a population with.
//   - Its sibling in the same paragraph of the same commit was "+16 for perfect
//     price timing", which was later re-measured with a standard error, fell to
//     +5.6 and was reclassified as unresolvable. One of the pair was re-measured
//     and shrank threefold; the other never was.
//   - The nearest quantity that *was* re-measured is the availability
//     **reconstruction**: +273 on three cells collapsed to about 8 points a season
//     at twenty-four, a 34x inflation, for the understood reason that buying a
//     summer departure is a GW1-shaped problem while five of six entry points are
//     not GW1. This oracle is in exactly that family.
//   - It is a hindsight oracle, so whatever it reports is a **bound** on what
//     perfect team news could be worth, never an achievable gain.
//
// The *direction* is not in doubt and this diagnostic is not needed to establish
// it. TestDiagTransferError already puts the sell-side error at −0.100 pts/gw for
// a player who keeps playing against −2.223 for the 13% who stop, which is an
// independent measurement of the same mechanism. What is in doubt is 322.
//
// # What the oracle actually knows, which is narrower than "team news"
//
// statusAt's oracle branch marks a player unavailable from the very start of the
// season when `p.Minutes == 0` — and that field is the archive's **season total**.
// So the oracle catches exactly one population: players who record no minutes at
// all, all season. A summer departure, a season-ending pre-season injury, a
// signing who never features.
//
// It does **not** catch the case that generates most of the real cost: a player
// who plays until October and then stops. Perfect team news would see that in
// September; this oracle is blind to it, because such a player's season total is
// not zero. reportOracleScope below sizes both populations from the archive, and
// the second is much the larger.
//
// That cuts both ways and both directions must be stated. As a bound on *perfect
// team news* the figure is an **under**-statement, since real team news covers a
// bigger population. As a bound on *anything buildable* it is still an
// over-statement, because it is hindsight over a whole season rather than a
// forecast. It is best read as what it is: the value of never buying a player who
// will not appear at all.
//
// # The positive control
//
// The price oracle's control was HOLD at exactly 0.000 in all 24 cells, proving a
// price advantage can only reach the transfer path. This one is the opposite:
// knowing a player will not appear changes which fifteen you buy, so **HOLD must
// move**. If it does not, the oracle is not reaching squad selection and the
// wiring is wrong rather than the effect being absent — check that before reading
// any size off the table.
//
// # Why it is a runPolicySweep and not its own harness
//
// TestDiagViceCaptainFix used to be its own harness, with its own cell map, its
// own paired difference and its own retired cluster-SE estimator, and the figure
// it produced was quoted in AGENTS.md long after the estimator had been retired
// elsewhere. So this is a two-arm sweep over the standard grid, which is what
// runPolicySweep is for, and every standard error, degree of freedom and
// Holm-adjusted p-value comes from stats/. Nothing here computes inference.
//
// The oracle is a bit on SimConfig.Oracles rather than an environment variable,
// so the two arms toggle inside a single process and pair properly, and the
// per-cell CSV carries the stamp of what each cell actually ran under. It used to
// be an os.Getenv inside statusAt — once per player per gameweek per cell — with
// the arm labels as the only record of what was toggled; the sidecar stamped the
// environment as it stood before the first cell and therefore recorded the oracle
// as unset for the whole sweep. That gap is closed: the stamp is derived from the
// same value the simulation consumed.
//
// The baseline arm is un-oracled by construction and validateOracleArms
// *enforces* it, so the sweep cannot silently measure oracle against oracle if a
// variant is ever inserted above it.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagAvailabilityOracle(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	// Printed before the sweep, because it decides how the sweep's number may be
	// worded. It costs no replay: it reads the archive and nothing else.
	reportOracleScope(t)

	starts := sweepStarts()
	fmt.Printf("\n=== the availability oracle, full grid: %s.\n",
		gridLabel(len(sweepPairNames()), len(starts)))
	fmt.Printf("Baseline is the shipped model. Positive means the oracle gains,\n")
	fmt.Printf("so the reported mean is an upper bound on perfect team news.\n")
	fmt.Printf("HOLD is the primary metric — this changes which fifteen is bought —\n")
	fmt.Printf("and HOLD moving at all is the control that the oracle reaches it.\n")
	fmt.Printf("It declares no cell invariance for exactly that reason: it moves\n")
	fmt.Printf("everything legitimately, and rests on the Tier 1 input diff instead.\n")
	runPolicySweep(t, []policyVariant{
		{label: "real (ships)", apply: func(sc *SimConfig) {}},
		oracleVariant(Oracles{Info: OracleAvailability}, "perfect team news", nil),
	}, starts)
}

// reportOracleScope sizes the population the oracle sees against the one it does
// not, from the archive alone.
//
// The distinction is the whole interpretation of this diagnostic. "Never played a
// minute" is a hindsight fact the oracle carries back to August; "played, then
// stopped" is the case a live judgement layer reads the news for, and it is
// invisible here. Reporting only the first and calling the result "perfect team
// news" would repeat the error this file was written to correct.
func reportOracleScope(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t)

	fmt.Printf("\n=== what the oracle can see, and what it cannot\n\n")
	fmt.Printf("%-9s %7s %9s %9s %11s %11s\n",
		"season", "players", "0 mins", "of those", "played then", "of those")
	fmt.Printf("%-9s %7s %9s %9s %11s %11s\n",
		"", "", "all season", "est. prior", "stopped", "established")

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		priorByCode := prior.ByCode()

		var zero, zeroEstablished, stopped, stoppedEstablished int
		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			// "Established" in the prior season: half a season of minutes, which
			// is the threshold priors.ThinSeason already uses for a player whose
			// prior record is worth believing. A pre-season model rates these
			// players; it has nothing to say about the rest, so the oracle
			// removing them changes no decision.
			est := false
			if q := priorByCode[p.Code]; q != nil && q.Minutes >= 1710 {
				est = true
			}
			if p.Minutes == 0 {
				zero++
				if est {
					zeroEstablished++
				}
				continue
			}
			// Played, then stopped: five appearances before the halfway point and
			// nothing after it. Deliberately conservative — it misses anyone who
			// stops in the closing weeks — so the column is a floor on the
			// population the oracle is blind to.
			var before, after int
			for gw := 1; gw <= 19; gw++ {
				if g, ok := p.GWs[gw]; ok && g.Minutes > 0 {
					before++
				}
			}
			for gw := 20; gw <= 38; gw++ {
				if g, ok := p.GWs[gw]; ok && g.Minutes > 0 {
					after++
				}
			}
			if before >= 5 && after == 0 {
				stopped++
				if est {
					stoppedEstablished++
				}
			}
		}
		fmt.Printf("%-9s %7d %9d %9d %11d %11d\n",
			cur.Name, len(cur.Players), zero, zeroEstablished,
			stopped, stoppedEstablished)
	}

	fmt.Printf("\n'0 mins all season' is the whole of what the oracle marks, and\n")
	fmt.Printf("'est. prior' is the subset a pre-season model would have rated at\n")
	fmt.Printf("all — only those can change a decision. 'played then stopped' is a\n")
	fmt.Printf("floor on the population perfect team news would catch and this\n")
	fmt.Printf("oracle cannot, so the figure below understates team news and\n")
	fmt.Printf("overstates anything forecastable.\n")
}

// TestOracleAvailabilityIsOffUnlessAsked pins the default, in code rather than in
// prose.
//
// Every figure in AGENTS.md was measured without this oracle, so switching it on
// would inflate all of them at once and make the record incomparable with itself.
// That rule was written down and enforced by nothing: statusAt's oracle branch is
// three lines and an inverted condition would have flipped the default silently,
// with every replay still producing plausible output.
//
// Not DIAG-gated, and it must not become so — a guard that only runs when someone
// remembers to ask for it is not a guard.
//
// It now asserts the *value* rather than the environment, which is the point of
// the move: the zero Oracles is what every ordinary caller passes, so this is
// literally the default path rather than a reconstruction of it.
func TestOracleAvailabilityIsOffUnlessAsked(t *testing.T) {
	t.Setenv("FPL_ORACLE_AVAILABILITY", "")
	t.Setenv("FPL_NO_AVAILABILITY", "")

	// A player who records no minutes all season and whom FPL never flagged. The
	// oracle's whole job is to mark him; without it he must read as available,
	// because that is what FPL was showing and what every recorded figure assumed.
	gone := &Player{ID: 1, Status: "a"}
	if got := statusAt(gone, 1, gameweekStart(&Season{}, 1), Oracles{}); got != "a" {
		t.Errorf("zero-minute player reads %q with the oracle off, want %q — the "+
			"oracle has become the default and every figure in AGENTS.md was "+
			"measured without it", got, "a")
	}
	// And the environment alone no longer reaches it: the variable is a seed read
	// where a cell config is built, not a switch read on the hot path. If this
	// ever starts returning "u", an oracle has escaped its config field and can
	// contaminate a run nobody declared it in.
	t.Setenv("FPL_ORACLE_AVAILABILITY", "1")
	if got := statusAt(gone, 1, gameweekStart(&Season{}, 1), Oracles{}); got != "a" {
		t.Errorf("zero-minute player reads %q with the environment set and no "+
			"oracle on the config — the oracle is being read somewhere other "+
			"than the value the simulation consumed", got)
	}

	if got := statusAt(gone, 1, gameweekStart(&Season{}, 1),
		Oracles{Info: OracleAvailability}); got != "u" {
		t.Errorf("zero-minute player reads %q with the oracle on, want %q — the "+
			"oracle is wired but inert, so a sweep measuring it would report a "+
			"clean null", got, "u")
	}

	// The seed still resolves, because the name appears throughout the record and
	// a figure recorded against FPL_ORACLE_PRICES must stay reproducible.
	if o := OraclesFromEnv(); !o.Has(OracleAvailability) {
		t.Error("FPL_ORACLE_AVAILABILITY no longer seeds the oracle, so every " +
			"command-line recipe in docs/replay.md silently measures the baseline")
	}
	t.Setenv("FPL_ORACLE_PRICES", "1")
	if o := OraclesFromEnv(); !o.Has(OracleTransactPrice) {
		t.Error("FPL_ORACLE_PRICES no longer seeds the oracle")
	}
	t.Setenv("FPL_ORACLE_AVAILABILITY", "")
	t.Setenv("FPL_ORACLE_PRICES", "")
	if OraclesFromEnv().Active() {
		t.Error("an oracle is on with nothing asking for it")
	}
}
