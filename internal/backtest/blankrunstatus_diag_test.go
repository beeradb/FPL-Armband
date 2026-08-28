package backtest

import (
	"fmt"
	"os"
	"testing"
)

// DOES A BLANK RUN MEAN THE SAME THING WHEN NOBODY HAS FLAGGED HIM?
//
//	FPL_BLANKSTATUS_CSV=/tmp/blankstatus.csv DIAG=1 go test ./internal/backtest \
//	    -run TestDiagBlankRunByStatus -v -count=1 -timeout 60m
//
// # The claim under test
//
// `blankRunFactor` takes ONE argument — the length of the run — and returns a
// flat 0.75 for a run of 1 to 3, and 1, no penalty at all, for 4 or more. It
// never looks at `Status`.
//
// So an injured player who has not featured for three weeks and a fully fit
// player who has not featured for three weeks are discounted identically. Those
// are not the same event. The injured man has an explanation and a return; the
// fit man is being CHOSEN against, every week, by a manager with him available
// on the teamsheet. The second is a statement about his standing at the club and
// the first is a statement about his ankle.
//
// # What is measured, and why it is the ERROR rather than the minutes
//
// Flagged players obviously play less — that is what the flag means, and
// measuring it would confirm only that FPL's status field works. What matters is
// whether the MODEL is differently wrong about the two groups, because that is
// the only thing a change to `blankRunFactor` could fix.
//
// So: predicted expected minutes against minutes actually played over the
// following window, per (blank-run bucket, flagged or not). A ratio near 1 means
// the model has that cell right. The comparison that carries the answer is the
// unflagged cell against the flagged one at the SAME run length — the run length
// is held fixed, so what is left is the flag.
//
// # PRE-REGISTERED, before running
//
//   - **If the flag carries information the run does not**, the two cells at the
//     same run length are differently calibrated — and the direction predicted by
//     the mechanism above is that the UNFLAGGED player is over-predicted by more,
//     because his blanks are a settled selection decision the model is reading as
//     a temporary dip.
//   - **If the run is sufficient on its own**, the two cells read alike at every
//     run length and `blankRunFactor` ignoring status is correct.
//   - **If the FLAGGED player is the more over-predicted**, the mechanism is
//     backwards: the model would be failing to price an injury it has been told
//     about, which is a different bug in a different place — `Status` already
//     feeds the availability term, so that would point there rather than here.
//   - ⚠️ **The 4+ bucket is where a penalty is currently ZERO.** Its comment
//     argues the exponential average has caught up by then. That is a claim about
//     the SAME quantity this measures, so the 4+ row is a direct test of it, and
//     the prediction is that it reads near 1 if the argument holds.
//
// ⚠️ Calibration, so it prices nothing. A cell being mis-calibrated does not say
// a correction wins points, and this record has lost points five times correcting
// a measured bias.
func TestDiagBlankRunByStatus(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	const window = 6
	entries := []int{6, 12, 20, 28}

	fmt.Printf("\n=== DOES A BLANK RUN MEAN THE SAME THING UNFLAGGED?\n")
	fmt.Printf("Predicted expected minutes over minutes actually played in the\n")
	fmt.Printf("following %d gameweeks. 1.00 is calibrated; above 1 is the model\n", window)
	fmt.Printf("expecting more football than arrives.\n")
	fmt.Printf("⚠️ Run length is held FIXED across each pair, so what differs\n")
	fmt.Printf("between the two columns is only whether anyone has flagged him.\n")
	fmt.Printf("⚠️ blankRunFactor is flat 0.75 for runs 1-3 and 1.00 for 4+, and\n")
	fmt.Printf("never reads Status. The 4+ row tests its own stated reason.\n\n")
	fmt.Printf("  %-10s %7s %7s   %7s %7s\n", "blank run", "unflag", "n", "flagged", "n")

	var csv *os.File
	if path := os.Getenv("FPL_BLANKSTATUS_CSV"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("FPL_BLANKSTATUS_CSV=%s: %v", path, err)
		}
		defer f.Close()
		csv = f
		fmt.Fprintln(csv, "season,entry_gw,run_bucket,flagged,n,pred,actual,window")
	}

	// bucket names the run length in the three regions blankRunFactor treats
	// differently, so a difference shows up against the term's own shape.
	bucket := func(run int) string {
		switch {
		case run == 0:
			return "0 (playing)"
		case run <= 3:
			return "1-3 (penalised)"
		default:
			return "4+ (no penalty)"
		}
	}

	type acc struct {
		pred, actual float64
		n            int
	}
	// [bucket][flagged] accumulated across every season and entry point.
	totals := map[string]map[bool]*acc{}

	// ⚠️ **The archive's own `status` CANNOT answer this question, and using it
	// produces a result that is true by construction.**
	//
	// `statusAt` reconstructs availability from an END-OF-SEASON snapshot, so the
	// only flag it can carry backwards is one the player was still carrying in
	// May. Every injury that resolved is invisible, and "flagged" therefore means
	// "stopped playing and never resumed".
	//
	// Run that way the flagged column reads `actual = 0.000` for thousands of
	// players and a predicted/actual ratio in the tens of thousands. That is not
	// a finding about the model; it is the definition of the stratum being read
	// back out, and it is this record's byte-identical trap wearing a number — a
	// comparison that never ran.
	//
	// `OracleTeamNews` replaces the reconstruction with FPL's own flag as it
	// stood at each deadline, from the captures under `data/captures/`. That is
	// the only honest source for this contrast. The loader REFUSES rather than
	// returning an empty table, because an oracle that reaches nothing reports
	// the same clean null as a real one.
	// ⚠️ Declaring the oracle here is NOT enough, and that is the trap this
	// diagnostic exists to document. `applyTeamNews` runs further down the
	// replay path than `EngineAt`, so an engine built directly for a
	// diagnostic carries the RECONSTRUCTION no matter what is declared on the
	// SimConfig. Setting it and believing it is how a contaminated table gets
	// published as a clean one.
	//
	// It is set anyway, so that wiring `EngineAt` to honour it later makes this
	// diagnostic correct rather than needing to be rediscovered. Until then the
	// refusal below is what protects the reader.
	news, err := LoadTeamNews(TeamNewsFilter{})
	if err != nil {
		t.Skipf("point-in-time team news unavailable, and the archive's own "+
			"status cannot answer this question: %v", err)
	}
	sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
	sc.Oracles = Oracles{Info: OracleTeamNews, News: news}
	for _, pr := range loadPairsOrSkip(t, cfg) {
		for _, through := range entries {
			e, _ := EngineAt(pr.Cur, pr.Prior, through, sc)
			if e.Recent == nil {
				continue // no recency index, so no blank run to read
			}
			for id, p := range pr.Cur.Players {
				el := e.Boot.ElementByID(id)
				if el == nil {
					continue
				}
				r, ok := e.Recent.Get(el.Code)
				if !ok || r.Matches == 0 {
					continue
				}
				// ⚠️ "Flagged" is anything FPL is not calling fully available.
				// `a` is available; everything else — injured, doubtful,
				// suspended, unavailable, on loan — is a public statement that
				// something is wrong, which is exactly the distinction under
				// test. Reading only `i` would put doubtful players in the
				// unflagged column and blunt the contrast.
				flagged := el.Status != "a"

				var actual float64
				for gw := through + 1; gw <= through+window; gw++ {
					if g, ok := p.GWs[gw]; ok {
						actual += float64(g.Minutes)
					}
				}
				b := bucket(r.BlankRun)
				if totals[b] == nil {
					totals[b] = map[bool]*acc{}
				}
				if totals[b][flagged] == nil {
					totals[b][flagged] = &acc{}
				}
				a := totals[b][flagged]
				a.pred += e.Metrics(el).ExpectedMinutes
				a.actual += actual / window
				a.n++
			}
		}
		if csv != nil {
			// Per season so R can cluster on it; the console table pools.
			for _, through := range entries {
				e, _ := EngineAt(pr.Cur, pr.Prior, through, sc)
				if e.Recent == nil {
					continue
				}
				per := map[string]map[bool]*acc{}
				for id, p := range pr.Cur.Players {
					el := e.Boot.ElementByID(id)
					if el == nil {
						continue
					}
					r, ok := e.Recent.Get(el.Code)
					if !ok || r.Matches == 0 {
						continue
					}
					var actual float64
					for gw := through + 1; gw <= through+window; gw++ {
						if g, ok := p.GWs[gw]; ok {
							actual += float64(g.Minutes)
						}
					}
					b, fl := bucket(r.BlankRun), el.Status != "a"
					if per[b] == nil {
						per[b] = map[bool]*acc{}
					}
					if per[b][fl] == nil {
						per[b][fl] = &acc{}
					}
					a := per[b][fl]
					a.pred += e.Metrics(el).ExpectedMinutes
					a.actual += actual / window
					a.n++
				}
				for b, byFlag := range per {
					for fl, a := range byFlag {
						if a.n < 4 {
							continue
						}
						fmt.Fprintf(csv, "%s,%d,%s,%t,%d,%.4f,%.4f,%d\n",
							pr.Name, through, b, fl, a.n,
							a.pred/float64(a.n), a.actual/float64(a.n), window)
					}
				}
			}
		}
	}

	// ⚠️ **THE REFUSAL.** A stratum of thousands of players whose realised
	// minutes sum to EXACTLY zero is not football, it is a definition being read
	// back out: the archive's `status` is reconstructed from the END-OF-SEASON
	// snapshot, so "flagged" means "was still flagged in May", which selects on
	// never having played again.
	//
	// Printing that ratio would put a number in front of a reader that looks like
	// a measurement and is a tautology. This record has published one of those
	// before and calls it the byte-identical trap. So the table is withheld and
	// the reason is printed instead — a refusal a reader can act on beats a
	// figure they have to know to distrust.
	for _, b := range []string{"0 (playing)", "1-3 (penalised)", "4+ (no penalty)"} {
		if a := totals[b][true]; a != nil && a.n >= 4 && a.actual == 0 {
			// A SKIP, not a failure: the measurement cannot be MADE on this
			// source. Calling it a failure would say the model is wrong, when
			// what is wrong is the instrument, and a permanently red diagnostic
			// gets muted rather than fixed.
			t.Skipf("the flagged stratum in %q has %d players whose realised "+
				"minutes sum to exactly zero.\n\n"+
				"That is the archive's end-of-season status reconstruction being "+
				"read back out, not a property of the model: statusAt can only "+
				"carry backwards a flag the player still had in May, so 'flagged' "+
				"here means 'never played again'.\n\n"+
				"This contrast needs FPL's own flag as it stood at each deadline, "+
				"from data/captures/. OracleTeamNews supplies it, but applyTeamNews "+
				"runs below EngineAt, so declaring the oracle on a SimConfig a "+
				"diagnostic builds its own engine from does NOT reach it. Wire "+
				"EngineAt to honour Oracles.News before trusting any figure here.",
				b, a.n)
		}
	}

	for _, b := range []string{"0 (playing)", "1-3 (penalised)", "4+ (no penalty)"} {
		row := totals[b]
		if row == nil {
			continue
		}
		cell := func(fl bool) (string, string) {
			a := row[fl]
			if a == nil || a.n < 4 || a.actual == 0 {
				return "-", "-"
			}
			return fmt.Sprintf("%.2f", a.pred/a.actual), fmt.Sprintf("%d", a.n)
		}
		uv, un := cell(false)
		fv, fn := cell(true)
		fmt.Printf("  %-10s %7s %7s   %7s %7s\n", b, uv, un, fv, fn)
	}

	fmt.Printf("\n⚠️ Pooled here; the CSV carries a row per season so the\n")
	fmt.Printf("inference can cluster on the season, which is the unit of\n")
	fmt.Printf("replication this harness has.\n")
	fmt.Printf("⚠️ The contrast is unflagged AGAINST flagged at the same run\n")
	fmt.Printf("length. Reading one column alone confounds the flag with the run.\n")
}
