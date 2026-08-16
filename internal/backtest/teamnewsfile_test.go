package backtest

import (
	"testing"
)

// TestTheExportCoversEverySeasonTheGridPlays is the difference between "nothing to
// see" and "could not run".
//
// A season with no recovered captures makes the oracle inert in six of the grid's
// cells, and this project's own rule is that a season producing identical output
// under an intervention is not a tie — it is a season where the intervention could
// not run. That failure is silent by construction: the arm still replays, still
// writes 36 rows, and still reports a mean.
func TestTheExportCoversEverySeasonTheGridPlays(t *testing.T) {
	news, err := LoadTeamNews(TeamNewsFilter{MaxEventsBehind: -1})
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]int{}
	for _, s := range news.Summary() {
		covered[s.Season] = s.Kept
	}
	for _, pair := range sweepPairNames() {
		// pair[1] is the season played; pair[0] is only a source of prior-season
		// aggregates and has no deadlines of its own to recover news for.
		if covered[pair[1]] == 0 {
			t.Errorf("%s is played by the sweep grid and has no recovered team news, "+
				"so every one of its cells would be byte-identical to the baseline "+
				"while still reporting a mean", pair[1])
		}
	}
}

// TestTheExportCarriesWhatTheReconstructionCannot is the liveness check on the data
// itself, and it names the specific gap.
//
// `statusAt` emits "a" and a final "u" or "i", and **never** "d": doubtful is
// transient, so its end-of-season value says nothing about the rest of the year and
// the reconstruction deliberately declines to carry it back. A published percentage
// it cannot produce at all — nothing in the replay has ever set that field.
//
// If either disappears from the export, the two arms stop measuring what their names
// say and the flag arm collapses toward the availability oracle it exists to
// generalise.
func TestTheExportCarriesWhatTheReconstructionCannot(t *testing.T) {
	news, err := LoadTeamNews(TeamNewsFilter{MaxEventsBehind: -1})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	chances := 0
	for _, byCode := range news.flags {
		for _, f := range byCode {
			seen[f.Status]++
			if f.Chance != nil {
				chances++
			}
		}
	}
	if seen["d"] == 0 {
		t.Error("the export carries no doubtful flags at all — that is the state " +
			"statusAt refuses to reconstruct, and without it the flag arm is the " +
			"availability oracle with extra steps")
	}
	if chances == 0 {
		t.Error("the export carries no published chance of playing, so the second " +
			"arm would set nothing and report a clean null")
	}
	// And the statuses are FPL's own alphabet rather than something re-encoded on
	// the way through: availabilityFactor switches on exactly these letters, and an
	// unrecognised one falls through to its 1.0 default silently.
	for status := range seen {
		switch status {
		case "a", "d", "i", "n", "s", "u":
		default:
			t.Errorf("the export carries status %q, which availabilityFactor does "+
				"not recognise — it would price him at full strength without "+
				"anything saying so", status)
		}
	}
}

// TestTheStalenessFilterActuallyDrops pins the knob against being inert.
//
// A filter that silently keeps everything is worse than none: the headline would
// carry a freshness claim nothing enforces, and the robustness run would reproduce
// the headline exactly and read as confirmation.
func TestTheStalenessFilterActuallyDrops(t *testing.T) {
	all, err := LoadTeamNews(TeamNewsFilter{MaxEventsBehind: -1})
	if err != nil {
		t.Fatal(err)
	}
	tight, err := LoadTeamNews(TeamNewsFilter{MaxHoursBefore: 48, MaxEventsBehind: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(tight.covered) >= len(all.covered) {
		t.Errorf("a 48-hour filter keeps %d gameweeks against %d unfiltered — the "+
			"knob is inert, so any robustness run using it reproduces the headline "+
			"and reads as confirmation", len(tight.covered), len(all.covered))
	}
	if len(tight.covered) == 0 {
		t.Error("a 48-hour filter keeps nothing at all")
	}
	// The dropped gameweeks must also lose their flags, or the filter would move
	// coverage without moving what the model reads.
	for key := range all.covered {
		if tight.covered[key] {
			continue
		}
		if len(tight.flags[key]) > 0 {
			t.Errorf("%s GW%d was filtered out and still carries %d flags",
				key.Season, key.GW, len(tight.flags[key]))
			break
		}
	}
}

// TestACoveredGameweekIsAuthoritative pins the contract that makes this a
// replacement for the reconstruction rather than a patch on it.
func TestACoveredGameweekIsAuthoritative(t *testing.T) {
	news, err := LoadTeamNews(TeamNewsFilter{MaxEventsBehind: -1})
	if err != nil {
		t.Fatal(err)
	}
	var key seasonWeek
	for k := range news.covered {
		if len(news.flags[k]) > 0 {
			key = k
			break
		}
	}
	if key.Season == "" {
		t.Fatal("no covered gameweek carries any flag")
	}
	// A code nobody has: present in the payload's gameweek, absent from the flag
	// list, therefore available with no percentage.
	status, chance, ok := news.FlagAt(key.Season, key.GW, -1)
	if !ok || status != "a" || chance != nil {
		t.Errorf("an unflagged player in a covered gameweek reads (%q, %v, %v), "+
			"want available with no percentage", status, chance, ok)
	}
	// An uncovered gameweek says so, rather than answering "available" for
	// everybody — which would silently convert a gap into a clean bill of health
	// for the whole league.
	if _, _, ok := news.FlagAt(key.Season, 99, 1); ok {
		t.Error("an uncovered gameweek answers as though it were known")
	}
}
