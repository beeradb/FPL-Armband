package backtest

// What FPL's 2026/27 Bonus Points System change does to `Bonus90`.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBPSRuleChange -v -timeout 10m
//
// # Why this exists, and what it retracts
//
// AGENTS.md recorded the 2026/27 BPS change as **unmeasurable for a season**, on
// the reasoning that all four archived seasons predate it. That is wrong, and the
// claim is what stopped anyone trying. Bonus is awarded on a **ranking within a
// match**, and the archive carries everything the ranking needs: `bps` and
// `fixture` per player-gameweek, plus the action counts the two arithmetic changes
// act on. So the new rules can be applied to old football, the awards recomputed,
// and the shift read off directly. No replay, no model, one CSV.
//
// The distinction that makes it work is that this measures a **rule**, not a
// model. Nothing here is a prediction, so nothing here needs a replay cell or a
// standard error: 380 matches under two award rules is a complete enumeration of
// the population, and the only uncertainty is in the one input the archive lacks
// (below).
//
// # The rules, from FPL's published material
//
// `game_config.scoring` in bootstrap-static publishes the *points* table and not
// the BPS schedule or the per-match divisors — TestWhatTheScoringTableCannotVerify
// records exactly that gap — so these come from FPL's own announcement:
//
//   - **Tackled** (−1 BPS each time a player was dispossessed) is removed.
//   - **Clearances, blocks and interceptions** earn 1 BPS per **three** actions
//     rather than per two.
//   - **Saves** are restructured: 2 BPS for any save, +1 for a save from inside
//     the box, the separate outside-the-box line item removed, and a new +1 for
//     saving a **big chance**.
//   - **A penalty save** drops from 8 BPS to 7. Those are BPS units, not points —
//     a penalty save is worth 5 *points* and always has been.
//
// # What each change contributes to the delta, and what cancels
//
// Only the *changes* matter, because the recomputation starts from the recorded
// `bps` and adjusts it. Everything FPL left alone — tackles made, recoveries,
// passing accuracy, big chances created, shots on target — cancels in the
// subtraction and never has to be modelled.
//
//   - **CBI: exactly computable.** floor(cbi/3) − floor(cbi/2), from the archive's
//     own per-gameweek count. This is the whole of the defender effect.
//   - **Penalty save: exactly zero.** −1 for the 8→7 cut, +1 because a penalty is
//     a big chance, which is precisely the offset FPL describes. Both legs are
//     applied so the cancellation is visible rather than assumed.
//   - **Ordinary saves: net zero at the schedule level.** An inside-the-box save
//     was 3 BPS and is now 2+1 = 3; an outside-the-box save was 2 and is now 2. So
//     the restructure moves nothing except through the new big-chance line.
//   - **Big-chance saves: the one unknown.** The archive carries total `saves` and
//     no breakdown by shot quality, so how many of a keeper's saves were big
//     chances cannot be recovered. It is swept as `bigChanceRates` rather than
//     silently assumed, and the sweep is reported.
//   - **Tackled: not in the archive at any granularity.** The column is `tackles`
//     — tackles *made* — which is a different event and is unchanged by the rule
//     anyway. So this diagnostic **understates the gain to the players the removed
//     penalty was hurting**: dribbling wingers and attacking full-backs. The MID
//     and FWD columns are therefore lower bounds, and they are the two positions
//     that change was aimed at. Read them as "at least this".
//
// # Why the big-chance credit is modelled as a whole BPS unit
//
// A fractional credit cannot change an integer ranking. Modelled as `rate x saves`
// carried as a real number, a keeper picks up +0.15 to +0.6 BPS and BPS gaps
// between players are almost always a whole unit or more, so the awards come back
// **identical to the rate-zero arm** — measured, not assumed. Spreading the credit
// fractionally is therefore arithmetically the same as ignoring the big-chance
// metric entirely, which is not a model of it. So it is rounded to whole saves per
// player-fixture, and the honest consequence is stated with the result: at a rate
// of 0.15 a keeper needs four saves in a match to be credited one, so within a
// given rate this too is conservative.
//
// # Two self-checks, both load-bearing
//
// The recomputation is only worth reading if the machinery reproduces the *old*
// awards from the *old* BPS, and that is checked rather than trusted: 29,747
// player-fixtures, and the test **fails** on a single disagreement. It found two
// on the first run, which turned out to be an archive defect rather than a tie-rule
// error (see below). And the awards are re-derived from a rule stated as
// "count how many players scored strictly more", which reproduces all four of
// FPL's documented tie cases without special-casing any of them — see award().
//
// # The archive defect this found
//
// **2025-26's merged_gw.csv carries ten duplicate (element, fixture) rows**, all
// byte-identical, for exactly two players: element 100 (Junior Kroupi) and element
// 391 (Ben Gannon-Doak). 2023-24 and 2024-25 have none, so it is specific to
// 2025-26 rather than systemic. It is reported by this diagnostic rather than
// worked around silently, because `loadGameweeks` **accumulates** — the fix for
// double gameweeks — so a duplicate row double-counts those players' minutes,
// points and every counting stat in the affected gameweek, *and* inflates
// `GW.Fixtures`, which makes an ordinary gameweek read as a double and halves the
// recency index's per-match minutes. Two players, small, and real.
//
// # Why 2025-26 only
//
// `clearances_blocks_interceptions` exists in the archive for 2025-26 and not for
// 2023-24 or 2024-25, checked against the headers directly. That is not a
// limitation to engineer around: defensive contribution only became a scoring
// category in 2025-26, so it is also the only season whose football was played
// under the rules the change is amending.

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"testing"
)

// bpsRuleSeason is the only season that can answer this, for the reason in the
// header: it is the only one carrying per-gameweek clearances, blocks and
// interceptions, and also the only one played under defensive contribution.
const bpsRuleSeason = "2025-26"

// bigChanceRates is the swept share of a keeper's saves that were saves from a big
// chance — the one input FPL's change needs that the archive cannot supply.
//
// 0.00 is not a guess, it is the **assumption-free floor**: at zero the only thing
// applied is the CBI divisor and the penalty-save cancellation, both exact. Every
// verdict that holds at 0.00 holds without any assumption about the saves
// restructure at all.
//
// 0.15 is the reported primary and is an estimate rather than a measurement.
// Premier League sides face roughly 1.3 big chances a match and a keeper makes
// roughly 2.8 saves, and a good share of big chances are scored or miss the
// target, which puts big-chance saves near 0.4 a match against 2.8 saves. It is
// stated so it can be argued with; 0.40 is included as a deliberately generous
// upper end.
var bigChanceRates = []float64{0.00, 0.10, 0.15, 0.25, 0.40}

const bpsPrimaryRate = 0.15

// bpsRow is one player-fixture, carrying only what the rule change touches.
type bpsRow struct {
	Element, Fixture             int
	Name, Pos                    string
	Minutes, BPS, Bonus          int
	CBI, Saves, PenSaves, DefCon int
}

func TestDiagBPSRuleChange(t *testing.T) {
	requireDiag(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()

	rows, dupes := loadBPSRows(t)
	grid := fmt.Sprintf("%s, %d matches, %d player-fixtures", bpsRuleSeason,
		countFixtures(rows), len(rows))
	t.Logf("population: %s", grid)
	if dupes > 0 {
		t.Logf("ARCHIVE DEFECT: %d duplicate (element, fixture) rows discarded. "+
			"loadGameweeks accumulates, so these double-count in the replay and "+
			"inflate GW.Fixtures — an ordinary gameweek reads as a double.", dupes)
	}

	// Self-check first: reproduce the recorded awards from the recorded BPS. If
	// this does not hold exactly, nothing below means anything.
	byFixture := groupByFixture(rows)
	var checked, wrong int
	for _, g := range byFixture {
		bps := make([]int, len(g))
		for i, r := range g {
			bps[i] = r.BPS
		}
		for i, got := range award(bps) {
			checked++
			if got != g[i].Bonus {
				wrong++
				if wrong <= 5 {
					t.Logf("  tie rule disagrees: fixture %d %s bps %d recorded %d computed %d",
						g[i].Fixture, g[i].Name, g[i].BPS, g[i].Bonus, got)
				}
			}
		}
	}
	if wrong > 0 {
		t.Fatalf("award() reproduced %d of %d recorded bonus values. The tie rule or "+
			"the fixture grouping is wrong, so every recomputation below is too.",
			checked-wrong, checked)
	}
	t.Logf("tie rule validated: %d of %d recorded bonus awards reproduced exactly", checked, checked)

	// The sweep. Per-position realised bonus per 90, old rules against new.
	t.Logf("")
	t.Logf("Per-position realised bonus per 90, old rules against 2026/27 rules.")
	t.Logf("The GKP column is the only one the big-chance rate moves; DEF is driven")
	t.Logf("entirely by the CBI divisor and is nearly flat in it.")
	t.Logf("%-5s %-6s %8s %8s %9s %9s %8s", "rate", "pos", "old", "new", "old/90", "new/90", "shift")
	for _, rate := range bigChanceRates {
		res := applyNewRules(rows, rate)
		for _, pos := range []string{"GK", "DEF", "MID", "FWD"} {
			a := res.byPos[pos]
			t.Logf("%-5.2f %-6s %8d %8d %9.4f %9.4f %+7.1f%%",
				rate, pos, a.old, a.new, a.per90old(), a.per90new(), a.shiftPct())
			sink.emitAll("bps_rule_change", grid,
				fmt.Sprintf("%s, big-chance save rate %.2f", pos, rate), a.minutes,
				measure{"old_per90", a.per90old()},
				measure{"new_per90", a.per90new()},
				measure{"shift_pct", a.shiftPct()},
			)
		}
	}

	// The within-position split, which is the part that matters. A bias shared by
	// every player in a position is invisible to an argmax; this one is not.
	for _, rate := range []float64{0.00, bpsPrimaryRate} {
		res := applyNewRules(rows, rate)
		for _, split := range []struct {
			pos, by string
			key     func(bpsTotals) float64
			emit    bool
		}{
			{"DEF", "defcon per 90 (what PlayerMetrics carries)", bpsTotals.defcon90, true},
			{"DEF", "CBI per 90 (the quantity the rule acts on)", bpsTotals.cbi90, false},
			{"MID", "CBI per 90", bpsTotals.cbi90, false},
		} {
			t.Logf("")
			t.Logf("%s, 900+ minutes, terciles by %s, big-chance rate %.2f:", split.pos, split.by, rate)
			for _, b := range terciles(res.byPlayer, split.pos, 900, split.key) {
				t.Logf("  %-14s n=%3d  dc/90 %5.2f  cbi/90 %5.2f  bonus/90 %.4f -> %.4f  %+.1f%%",
					b.label, b.n, b.tot.defcon90(), b.tot.cbi90(),
					b.tot.per90old(), b.tot.per90new(), b.tot.shiftPct())
				// Who is in the extreme buckets, because "centre-backs lose and
				// full-backs do not" is a claim about *roles* and the terciles are
				// cut on a statistic. Naming them is how a reader checks that the
				// statistic is standing in for the role it is said to.
				if b.label != "middle" {
					t.Logf("       e.g. %s", b.edge)
				}
				if rate == bpsPrimaryRate && split.emit {
					sink.emitAll("bps_rule_change", grid,
						fmt.Sprintf("%s, %s defcon third", split.pos, b.label), b.n,
						measure{"old_per90", b.tot.per90old()},
						measure{"new_per90", b.tot.per90new()},
						measure{"shift_pct", b.tot.shiftPct()},
					)
				}
			}
		}
	}

	// How one-directional the move is, per position. A whole position moving one way
	// is the signature of a systematic reallocation rather than of noise, and it is
	// a stronger statement than any single percentage: a percentage could be one
	// player, and "none of the 24 moved down" cannot be.
	t.Logf("")
	t.Logf("Direction of movement, 900+ minutes, big-chance rate %.2f:", bpsPrimaryRate)
	for _, pos := range []string{"GK", "DEF", "MID", "FWD"} {
		var n, up, down int
		for _, v := range applyNewRules(rows, bpsPrimaryRate).byPlayer {
			if v.pos != pos || v.minutes < 900 {
				continue
			}
			n++
			switch {
			case v.new > v.old:
				up++
			case v.new < v.old:
				down++
			}
		}
		t.Logf("  %-4s n=%3d  gained %3d  lost %3d  unchanged %3d", pos, n, up, down, n-up-down)
	}

	// Named movers, so the result can be checked against an independent re-run
	// rather than only against itself.
	res := applyNewRules(rows, bpsPrimaryRate)
	movers := make([]bpsTotals, 0, len(res.byPlayer))
	for _, v := range res.byPlayer {
		if v.minutes >= 900 {
			movers = append(movers, v)
		}
	}
	sort.Slice(movers, func(i, j int) bool {
		if d := (movers[i].new - movers[i].old) - (movers[j].new - movers[j].old); d != 0 {
			return d < 0
		}
		return movers[i].element < movers[j].element
	})
	t.Logf("")
	t.Logf("Biggest movers at rate %.2f (900+ minutes), for cross-checking:", bpsPrimaryRate)
	for _, m := range movers[:min(6, len(movers))] {
		t.Logf("  lost   %-28s %-4s %3d -> %3d (%+d)", m.name, m.pos, m.old, m.new, m.new-m.old)
	}
	for i := max(0, len(movers)-6); i < len(movers); i++ {
		m := movers[i]
		t.Logf("  gained %-28s %-4s %3d -> %3d (%+d)", m.name, m.pos, m.old, m.new, m.new-m.old)
	}
}

// award turns a match's BPS into 3/2/1 bonus, FPL's way.
//
// The rule is stated once as "how many players scored strictly more than this
// one", which reproduces every documented tie case without a branch for any of
// them. Worth spelling out, because the special cases look like they need code:
//
//   - two tied on top: both have nobody above them, so 3 each; the next player has
//     two above him, so 1 — which is FPL's "the third-highest is awarded 1", the
//     2 being skipped;
//   - two tied for second: each has one above, so 2 each; the fourth player has
//     three above, so 0 — FPL's "no player receives 1";
//   - three tied for third: each has two above, so 1 each;
//   - three tied on top: 3 each, and the next has three above, so 0.
//
// Validated against the archive's own recorded bonus at 29,747 of 29,747.
func award(bps []int) []int {
	out := make([]int, len(bps))
	for i, v := range bps {
		greater := 0
		for _, w := range bps {
			if w > v {
				greater++
			}
		}
		switch greater {
		case 0:
			out[i] = 3
		case 1:
			out[i] = 2
		case 2:
			out[i] = 1
		}
	}
	return out
}

// newBPS applies the 2026/27 schedule to one recorded player-fixture.
//
// It adjusts the recorded figure rather than rebuilding it, so every unchanged
// line item cancels and none of them has to be modelled. See the file header for
// which changes are exact and which one is the swept assumption.
func newBPS(r bpsRow, bigChanceRate float64) int {
	d := r.CBI/3 - r.CBI/2

	// The penalty-save cut and its offset, both applied so the cancellation is
	// visible in the code rather than asserted in a comment. A penalty is a big
	// chance, so a saved one loses 1 BPS to the 8→7 change and gains 1 back.
	d += -1 * r.PenSaves
	d += 1 * r.PenSaves

	// Big-chance saves among ordinary saves. Rounded to whole saves because a
	// fractional credit provably cannot move an integer ranking.
	ordinary := r.Saves - r.PenSaves
	if ordinary > 0 {
		d += int(math.Floor(bigChanceRate*float64(ordinary) + 0.5))
	}
	return r.BPS + d
}

// bpsTotals accumulates one group — a position, a tercile, or a single player.
type bpsTotals struct {
	element  int
	name     string
	pos      string
	old, new int
	minutes  int
	cbi      int
	defcon   int
	tackled  int
}

func (a bpsTotals) per90(n int) float64 {
	if a.minutes == 0 {
		return 0
	}
	return float64(n) / float64(a.minutes) * 90
}
func (a bpsTotals) per90old() float64  { return a.per90(a.old) }
func (a bpsTotals) per90new() float64  { return a.per90(a.new) }
func (a bpsTotals) cbi90() float64     { return a.per90(a.cbi) }
func (a bpsTotals) defcon90() float64  { return a.per90(a.defcon) }
func (a bpsTotals) tackled90() float64 { return a.per90(a.tackled) }
func (a bpsTotals) shiftPct() float64 {
	if a.old == 0 {
		return 0
	}
	return (float64(a.new)/float64(a.old) - 1) * 100
}

func (a *bpsTotals) add(b bpsTotals) {
	a.old += b.old
	a.new += b.new
	a.minutes += b.minutes
	a.cbi += b.cbi
	a.defcon += b.defcon
	a.tackled += b.tackled
}

type bpsResult struct {
	byPos    map[string]bpsTotals
	byPlayer map[int]bpsTotals
}

// applyNewRules re-ranks every match under the new schedule and totals the awards.
func applyNewRules(rows []bpsRow, bigChanceRate float64) bpsResult {
	res := bpsResult{byPos: map[string]bpsTotals{}, byPlayer: map[int]bpsTotals{}}
	for _, g := range groupByFixture(rows) {
		oldBPS := make([]int, len(g))
		newB := make([]int, len(g))
		for i, r := range g {
			oldBPS[i] = r.BPS
			newB[i] = newBPS(r, bigChanceRate)
		}
		oldAward, newAward := award(oldBPS), award(newB)
		for i, r := range g {
			one := bpsTotals{
				element: r.Element, name: r.Name, pos: r.Pos,
				old: oldAward[i], new: newAward[i],
				minutes: r.Minutes, cbi: r.CBI, defcon: r.DefCon,
			}
			p := res.byPos[r.Pos]
			p.pos = r.Pos
			p.add(one)
			res.byPos[r.Pos] = p

			q := res.byPlayer[r.Element]
			q.element, q.name, q.pos = r.Element, r.Name, r.Pos
			q.add(one)
			res.byPlayer[r.Element] = q
		}
	}
	return res
}

type bpsBucket struct {
	label string
	n     int
	tot   bpsTotals
	// edge names the three players at this bucket's outer end — the lowest of the
	// lowest and the highest of the highest — so a role claim can be checked
	// against the statistic the tercile was cut on.
	edge string
}

// terciles splits a position's players by a per-90 key. Sorted by the key and
// then by element id, so the split is reproducible: this package has already had
// one diagnostic report a different figure every run because it selected between
// equivalent rows while iterating a map.
func terciles(byPlayer map[int]bpsTotals, pos string, minMinutes int, key func(bpsTotals) float64) []bpsBucket {
	var ps []bpsTotals
	for _, v := range byPlayer {
		if v.pos == pos && v.minutes >= minMinutes {
			ps = append(ps, v)
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if a, b := key(ps[i]), key(ps[j]); a != b {
			return a < b
		}
		return ps[i].element < ps[j].element
	})
	n := len(ps)
	if n < 3 {
		return nil
	}
	k := n / 3
	groups := [][]bpsTotals{ps[:k], ps[k : n-k], ps[n-k:]}
	labels := []string{"lowest", "middle", "highest"}
	out := make([]bpsBucket, 0, 3)
	for i, g := range groups {
		b := bpsBucket{label: labels[i], n: len(g)}
		for _, p := range g {
			b.tot.add(p)
		}
		// The outer end of the bucket: the smallest keys for "lowest", the largest
		// for "highest". ps is sorted ascending, so one slice each way.
		edge := g
		if i == 0 && len(g) > 3 {
			edge = g[:3]
		} else if i == len(groups)-1 && len(g) > 3 {
			edge = g[len(g)-3:]
		}
		names := make([]string, 0, len(edge))
		for _, p := range edge {
			names = append(names, fmt.Sprintf("%s (%.1f)", p.name, key(p)))
		}
		b.edge = strings.Join(names, ", ")
		out = append(out, b)
	}
	return out
}

// groupByFixture returns each match's rows, in a fixed order.
func groupByFixture(rows []bpsRow) [][]bpsRow {
	byID := map[int][]bpsRow{}
	for _, r := range rows {
		byID[r.Fixture] = append(byID[r.Fixture], r)
	}
	ids := make([]int, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([][]bpsRow, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out
}

func countFixtures(rows []bpsRow) int {
	seen := map[int]bool{}
	for _, r := range rows {
		seen[r.Fixture] = true
	}
	return len(seen)
}

// loadBPSRows reads the archive's per-gameweek file directly.
//
// It does not go through Season, deliberately. `GW` carries none of `bps`,
// `fixture` or `clearances_blocks_interceptions`, and adding them would mean a
// cache version bump plus a matching field check in parsedByThisVersion — this
// package's recorded rule that a version bump is not a schema check. That is a
// real cost, it would force every cached season to be re-fetched, and nothing but
// this diagnostic wants the fields. Reading the CSV here keeps the blast radius at
// one file.
//
// Every column it depends on is checked for **presence**, not read with ival and
// hoped for. ival returns 0 for an absent column, so an archive that dropped
// `clearances_blocks_interceptions` would yield a delta of exactly zero in every
// row and print a clean, complete-looking table showing the rule change does
// nothing. That is this package's signature failure and it gets a hard error.
func loadBPSRows(t *testing.T) ([]bpsRow, int) {
	t.Helper()
	r, c, col, err := rows(context.Background(), bpsRuleSeason, "gws/merged_gw.csv")
	if err != nil {
		t.Skipf("archive unreachable: %v", err)
	}
	defer c.Close()

	for _, need := range []string{
		"element", "fixture", "bps", "bonus", "minutes", "position", "name",
		"clearances_blocks_interceptions", "saves", "penalties_saved",
		"defensive_contribution",
	} {
		if _, ok := col[need]; !ok {
			t.Fatalf("merged_gw.csv for %s has no %q column. Without it the "+
				"recomputation would silently read zero and report that the rule "+
				"change does nothing.", bpsRuleSeason, need)
		}
	}

	var out []bpsRow
	seen := map[[2]int]bool{}
	dupes := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("merged_gw.csv: %v", err)
		}
		row := bpsRow{
			Element:  ival(rec, col, "element"),
			Fixture:  ival(rec, col, "fixture"),
			Name:     sval(rec, col, "name"),
			Pos:      sval(rec, col, "position"),
			Minutes:  ival(rec, col, "minutes"),
			BPS:      ival(rec, col, "bps"),
			Bonus:    ival(rec, col, "bonus"),
			CBI:      ival(rec, col, "clearances_blocks_interceptions"),
			Saves:    ival(rec, col, "saves"),
			PenSaves: ival(rec, col, "penalties_saved"),
			DefCon:   ival(rec, col, "defensive_contribution"),
		}
		if row.Element == 0 || row.Fixture == 0 {
			continue
		}
		// Discard duplicate player-fixture rows rather than accumulating them, or
		// a repeated row becomes a phantom team-mate and manufactures a tie. That
		// is not hypothetical: it is what the two initial tie-rule disagreements
		// turned out to be.
		k := [2]int{row.Element, row.Fixture}
		if seen[k] {
			dupes++
			continue
		}
		seen[k] = true
		out = append(out, row)
	}
	if len(out) == 0 {
		t.Fatalf("no rows parsed for %s", bpsRuleSeason)
	}
	return out, dupes
}
