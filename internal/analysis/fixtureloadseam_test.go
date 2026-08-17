package analysis

import (
	"math"
	"testing"
)

// loadSeamSquad is a legal fifteen with distinct scores, so which eleven and
// which captain get picked is unambiguous.
//
// Scores descend with the index, so player 0 is the captain and the cheapest
// four end up on the bench.
func loadSeamSquad() []PlayerMetrics {
	pos := []string{
		"GKP", "GKP",
		"DEF", "DEF", "DEF", "DEF", "DEF",
		"MID", "MID", "MID", "MID", "MID",
		"FWD", "FWD", "FWD",
	}
	// Descending so the ordering is stable, and interleaved across positions so
	// the eleven is not simply the first eleven of the slice.
	score := []float64{
		4.0, 3.0,
		6.5, 6.0, 5.5, 5.0, 2.0,
		9.0, 8.0, 7.0, 4.5, 2.5,
		8.5, 7.5, 3.5,
	}
	sq := make([]PlayerMetrics, len(pos))
	for i := range pos {
		sq[i] = PlayerMetrics{
			ID: i + 1, Position: pos[i], Score: score[i],
			FixtureLoad: 1, ExpectedMinutes: 80, StartShare: 0.9,
			// As Metrics sets it. xiValueForTransfer gates the multiply on this
			// rather than on FixtureLoad > 0, because a load of exactly 0 is a
			// club that blanks the whole window and must be multiplied by rather
			// than skipped. Omitting it here would exempt the whole squad from
			// the term and this test would pass on a disabled multiplier.
			loadSet: true,
		}
	}
	return sq
}

// TestFixtureLoadSeparatesTransfersFromSquadBuilding pins the one seam that keeps
// the fixture-load term profitable.
//
// "Fixture load" is matches per gameweek: a club with a double gameweek plays
// twice and its players are worth roughly twice as much that week, and a club that
// blanks is worth nothing. Applied to *every* score the term loses about 53 points
// a season, because the opening fifteen is built before a ball is kicked and would
// trade present quality for a double months away that several transfer windows will
// have had a chance to move first. Applied to the weekly decision alone it is the
// largest reliable gain measured in this project.
//
// So it is applied inside the *exported* XIValue and nowhere else. The unexported
// xiValue, which squad construction reaches through objective and bestXI, must not
// see it.
//
// # Why this test rather than a list of callers
//
// The comment on XIValue says the seam holds because "XIValue is reached from
// RankSwaps, RankPairs, BuildPlans and the unified search — every one of them a
// transfer decision". That is an inventory of callers, and an inventory rots: a
// fifth caller added next year inherits transfer-only behaviour silently, and a
// squad builder that reaches for the exported name by mistake starts paying for a
// double it will not own by the time it arrives. Neither failure produces an error,
// a wrong squad size, or an illegal fifteen — only a slightly different squad.
//
// The load-bearing fact is not who calls what. It is that the two functions differ
// in exactly this one respect, which is what this pins:
//
//   - XIValue responds to FixtureLoad;
//   - the squad-building path does not;
//   - and XIValue does not leave scaled scores behind in the caller's slice, which
//     would leak the transfer view into whatever ran next.
//
// # What this does not say
//
// Not "squad building never sees fixture load". It pins the *objective* layer: that
// xiValue, objective and bestXI ignore the FixtureLoad field. A Score that already
// has the load folded in is invisible to them and to this test — and that is a real
// configuration, since Engine.FixtureLoadInScore() is true at a horizon of 1, which
// ApplyChipPlan can produce by shortening the horizon before a wildcard. That is
// defensible (at horizon 1 the load is the imminent week's actual fixture count) and
// FixtureLoadInScore reports it honestly, but do not read a pass here as ruling it
// out.
func TestFixtureLoadSeparatesTransfersFromSquadBuilding(t *testing.T) {
	if !fixtureLoadTransfers {
		t.Skip("FPL_NO_LOAD_TRANSFERS is set, which is the arm that turns this seam off")
	}

	flat := loadSeamSquad()
	const bw = DefaultBenchWeight

	baseXI := XIValue(flat)
	baseSquad := objective(flat, bw, false)

	// A double gameweek for the squad's best player: the case with the most to
	// gain from the transfer view and the most to lose from the squad view.
	doubled := loadSeamSquad()
	doubled[7].FixtureLoad = 2

	gotXI := XIValue(doubled)
	if gotXI <= baseXI+1e-9 {
		t.Errorf("XIValue is %.4f with a double and %.4f without; the transfer "+
			"objective must see fixture load, or the weekly decision is blind to a "+
			"double it could buy into", gotXI, baseXI)
	}
	// The captain is player 7 either way, so his score is counted twice and the
	// gain is exactly twice his own Score. Pinned as arithmetic rather than as a
	// direction, so a term that silently starts averaging or clamping fails here.
	want := baseXI + 2*flat[7].Score
	if math.Abs(gotXI-want) > 1e-9 {
		t.Errorf("XIValue rose to %.4f, want %.4f — doubling the captain's fixtures "+
			"should double his contribution, and he is counted twice", gotXI, want)
	}

	if got := objective(doubled, bw, false); math.Abs(got-baseSquad) > 1e-9 {
		t.Errorf("the squad-building objective moved to %.4f from %.4f when a "+
			"fixture load changed; it must not. Applied to squad construction this "+
			"term costs about 53 points a season, because a fifteen bought in August "+
			"pays today for a double in April that a transfer could buy nearer the "+
			"time", got, baseSquad)
	}

	// The same claim from the other side: picking the eleven must ignore the load
	// too, or a blanking club's player is benched by the squad view rather than by
	// the imminent-week view that is entitled to bench him.
	blank := loadSeamSquad()
	blank[7].FixtureLoad = 0.2
	before, _, _ := bestXI(flat)
	after, _, _ := bestXI(blank)
	for i := range before {
		if before[i].ID != after[i].ID {
			t.Fatalf("bestXI picked a different eleven once a club blanked (slot %d: "+
				"%d became %d); selection reads Score, and fixture load must not have "+
				"reached it", i, before[i].ID, after[i].ID)
		}
	}

	// XIValue must copy before it scales. Mutating the caller's slice would leak
	// the transfer view into every later use of the same squad — including
	// objective, which is measured as being damaged by exactly this signal.
	probe := loadSeamSquad()
	probe[7].FixtureLoad = 2
	scoreBefore := probe[7].Score
	_ = XIValue(probe)
	if probe[7].Score != scoreBefore {
		t.Errorf("XIValue left Score at %.4f, was %.4f: it scaled the caller's slice "+
			"in place, so anything reusing this squad now holds the transfer view of it",
			probe[7].Score, scoreBefore)
	}
}
