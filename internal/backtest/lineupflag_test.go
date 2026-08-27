package backtest

import (
	"context"
	"fmt"
	"sort"
	"testing"
)

// TestDiagFlaggedCoverage counts how many of the fifteen a held squad owns are
// carrying an availability flag at each weekly cutoff.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagFlaggedCoverage -v
//
// # Why this exists
//
// The flagged-only restriction on the lineups oracle returned **exactly zero**:
// not a small effect, but 0.0% of gameweeks changed across 918 of them, with the
// unflagged arm reproducing the unrestricted arm to the digit. Two readings fit
// that, and they are opposite in meaning:
//
//   - the flags are worthless — knowing the truth about flagged players changes
//     no decision, because the model already handles them correctly;
//   - the arm never fired — the population it restricts to is empty inside the
//     squads being scored, so the measurement is of nothing.
//
// This project has confused those two before, which is why every oracle here
// reports a mediator. So this counts the population directly rather than
// inferring it from an effect size.
//
// # What "flagged" can mean on the replay, which is less than it means live
//
// `statusAt` reconstructs availability from end-of-season status plus its
// timestamp, and the archive carries no `chance_of_playing`. So it emits only
// `a` (available), `u` (unavailable/left) and `i` (injured) — never `d`, the
// doubtful flag that carries the percentage. A live run sees a much richer
// signal than this, so a null here bounds the coarse version only.
func TestDiagFlaggedCoverage(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	type row struct {
		season      string
		cutoffs     int
		heldSlots   int // fifteen per cutoff
		flagged     int // held players carrying a flag at that cutoff
		flagPlayed  int // ... of whom actually recorded minutes that gameweek
		flagBlank   int // ... whose club had no fixture, so there is nothing to be right about
		poolPlayers int // every player the season carries, summed over cutoffs
		poolFlagged int
		poolPlayed  int
		byStatus    map[string]int
	}
	var rows []row

	for _, pair := range sweepPairNames() {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatalf("loading %s: %v", pair[0], err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatalf("loading %s: %v", pair[1], err)
		}
		r := row{season: pair[1], byStatus: map[string]int{}}
		byID := map[int]*Player{}
		for _, p := range cur.Players {
			byID[p.ID] = p
		}

		for _, start := range sweepStarts() {
			sc := sweepConfig(cfg, start, true)
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatalf("simulating %s@%d: %v", pair[1], start, err)
			}
			for gw := start; gw <= 38; gw++ {
				cutoff := gameweekStart(cur, gw)
				r.cutoffs++
				for _, id := range res.OpeningSquad {
					p, ok := byID[id]
					if !ok {
						continue
					}
					r.heldSlots++
					st := statusAt(p, gw, cutoff, Oracles{})
					r.byStatus[st]++
					if st != "a" {
						r.flagged++
						// The quantity that decides whether a flagged-only oracle
						// can fire at all: a flag the truth agrees with is nothing
						// to correct.
						if g, ok := p.GWs[gw]; !ok {
							r.flagBlank++
						} else if g.Minutes > 0 {
							r.flagPlayed++
						}
					}
				}
			}
		}

		// The pool is counted ONCE per season, outside the entry-point loop, and
		// that is the difference between a rate and a headline count. A held slot
		// legitimately appears once per cell, because a different entry point buys
		// a different fifteen — but a player-gameweek in the pool is the same
		// player-gameweek whichever week the manager entered, so summing it over
		// six starts multiplies the denominator by about six and reports a league
		// six times its real size. The rate survives that; the count does not, and
		// the count is what gets quoted.
		for gw := 1; gw <= 38; gw++ {
			cutoff := gameweekStart(cur, gw)
			for _, p := range cur.Players {
				r.poolPlayers++
				if statusAt(p, gw, cutoff, Oracles{}) != "a" {
					r.poolFlagged++
					if g, ok := p.GWs[gw]; ok && g.Minutes > 0 {
						r.poolPlayed++
					}
				}
			}
		}
		rows = append(rows, r)
	}

	fmt.Printf("\nAvailability flags visible at the weekly cutoff, as statusAt reconstructs them\n")
	fmt.Printf("A held squad is fifteen players; a cutoff is one gameweek of one cell.\n\n")
	fmt.Printf("Held slots are summed over cells (%d entry points buy %d different\n",
		len(sweepStarts()), len(sweepStarts()))
	fmt.Printf("fifteens). Pool slots are counted once per season, since a player-gameweek\n")
	fmt.Printf("is the same player-gameweek whichever week the manager entered.\n\n")
	fmt.Printf("%-10s %8s %10s %10s %9s %12s %10s\n",
		"season", "cutoffs", "held slots", "flagged", "%", "pool slots", "pool %")

	var totHeld, totFlag, totPool, totPoolFlag, totPoolPlayed int
	statuses := map[string]int{}
	for _, r := range rows {
		fmt.Printf("%-10s %8d %10d %10d %8.2f%% %12d %9.2f%%\n",
			r.season, r.cutoffs, r.heldSlots, r.flagged,
			100*float64(r.flagged)/float64(r.heldSlots),
			r.poolPlayers, 100*float64(r.poolFlagged)/float64(r.poolPlayers))
		totHeld += r.heldSlots
		totFlag += r.flagged
		totPool += r.poolPlayers
		totPoolFlag += r.poolFlagged
		totPoolPlayed += r.poolPlayed
		for k, v := range r.byStatus {
			statuses[k] += v
		}
	}
	fmt.Printf("%-10s %8s %10d %10d %8.2f%% %12d %9.2f%%\n",
		"pooled", "", totHeld, totFlag, 100*float64(totFlag)/float64(totHeld),
		totPool, 100*float64(totPoolFlag)/float64(totPool))

	keys := make([]string, 0, len(statuses))
	for k := range statuses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("\nheld-squad status codes seen: ")
	for _, k := range keys {
		fmt.Printf("%q=%d  ", k, statuses[k])
	}
	fmt.Printf("\n")

	// The decisive column. A flagged-only oracle can only ever change a decision
	// for a player the flag is WRONG about — one carrying a flag who nonetheless
	// played. If that count is zero the arm is inert on the data, quite apart from
	// whether the scoring path could have transmitted it.
	fmt.Printf("\nof the flagged held player-gameweeks, how many were the flag was WRONG\n")
	fmt.Printf("%-10s %10s %10s %9s %10s\n",
		"season", "flagged", "played", "%", "club blank")
	var tp, tb int
	for _, r := range rows {
		fmt.Printf("%-10s %10d %10d %8.2f%% %10d\n", r.season, r.flagged,
			r.flagPlayed, 100*safePct(r.flagPlayed, r.flagged), r.flagBlank)
		tp += r.flagPlayed
		tb += r.flagBlank
	}
	fmt.Printf("%-10s %10d %10d %8.2f%% %10d\n", "pooled", totFlag, tp,
		100*safePct(tp, totFlag), tb)
	fmt.Printf("\nacross the whole pool rather than the held fifteen: %d of %d flagged "+
		"player-gameweeks played (%.2f%%)\n", totPoolPlayed, totPoolFlag,
		100*safePct(totPoolPlayed, totPoolFlag))
	fmt.Printf("\nThe reconstruction is STICKY and therefore CONSERVATIVE: p.Status is the\n")
	fmt.Printf("END-of-season status and NewsAdded is the LAST news item, so a player is\n")
	fmt.Printf("flagged only from the date of the news that stuck. Someone injured in\n")
	fmt.Printf("October who returned in December and finished the season fit reads\n")
	fmt.Printf("available all year. So a flag here overwhelmingly means terminal, and a\n")
	fmt.Printf("flagged-only oracle has almost nothing to correct.\n")

	fmt.Printf("\nIf the held share is near zero the flagged-only oracle arm is INERT and\n")
	fmt.Printf("its 0.0 is the measurement of an empty population, not a null result.\n")
	fmt.Printf("The gap between the held share and the pool share is the opening squad's\n")
	fmt.Printf("own selection — MinExpectedMinutes cliffs absent players out of the pool,\n")
	fmt.Printf("which the record already flags as why the minutes onset finding cannot\n")
	fmt.Printf("resolve on the held metric.\n")
}

// safePct is a/b with an empty numerator reported as zero rather than NaN, since
// "no flagged players at all" and "flagged players who all sat out" are different
// facts and the count beside it already distinguishes them.
func safePct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
