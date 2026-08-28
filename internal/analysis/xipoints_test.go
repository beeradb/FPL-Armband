package analysis

import "testing"

// XIPoints is the unit of squad drift, and drift is about to be shown to a live
// manager about his own team rather than only computed inside a replay. These
// pin the two properties that make the number mean what the surface will say it
// means.
//
// ⚠️ They build a synthetic engine rather than calling `testEngine`, which skips
// when the FPL API is unreachable. A guard that skips is a guard that ships
// unrun, and these assert behaviour that does not depend on live data at all.

// benchOptimizeEngine assigns element ids as i+1 over a fixed position cycle of
// 1 GKP, 5 DEF, 5 MID, 2 FWD per 13, so positions are addressable by id.
const (
	xiGK1, xiGK2 = 1, 14 // i = 0 and 13, both element_type 1
	xiDEF        = 2     // ids 2..6 are element_type 2
	xiMID        = 7     // ids 7..11 are element_type 3
	xiFWD        = 12    // ids 12..13 are element_type 4
)

func legalFifteen() []int {
	return []int{
		xiGK1, xiGK2,
		xiDEF, xiDEF + 1, xiDEF + 2, xiDEF + 3, xiDEF + 4,
		xiMID, xiMID + 1, xiMID + 2, xiMID + 3, xiMID + 4,
		xiFWD, xiFWD + 1, 25, // 25 is i=24, 24%13=11, also a forward
	}
}

// ⚠️ An id the bootstrap does not know must be SKIPPED, not scored as zero. A
// squad carrying an id from another season would otherwise report a drift made
// entirely of absences, which reads exactly like a squad that has decayed — and
// drift is the number a manager is being asked to act on.
func TestXIPointsSkipsPlayersTheBootstrapDoesNotKnow(t *testing.T) {
	e := benchOptimizeEngine(60)
	squad := legalFifteen()

	want := XIPoints(e, squad)
	if want <= 0 {
		t.Fatalf("precondition: a legal fifteen should score something; got %.4f", want)
	}

	// 999999 is not in a 60-element bootstrap.
	got := XIPoints(e, append(append([]int(nil), squad...), 999999))
	if got != want {
		t.Errorf("an unknown id changed the total: %.6f with it, %.6f without. It must be "+
			"skipped — scoring it as zero would drag a real squad's drift up by an "+
			"absence rather than a decline.", got, want)
	}

	// ⚠️ Skipping must not become IGNORING THE LOSS, and showing that needs an
	// ELEVEN rather than a fifteen. Dropping the fifteenth player changes
	// nothing — he was on the bench — so a fifteen cannot separate "skipped the
	// unknown id" from "did not notice a player is missing". Every member of a
	// legal eleven is in the eleven by construction, so losing one must cost.
	xi := []int{
		xiGK1,
		xiDEF, xiDEF + 1, xiDEF + 2, xiDEF + 3,
		xiMID, xiMID + 1, xiMID + 2, xiMID + 3,
		xiFWD, xiFWD + 1,
	}
	whole := XIPoints(e, xi)
	broken := XIPoints(e, append(append([]int(nil), xi[:len(xi)-1]...), 999999))
	if broken >= whole {
		t.Errorf("an eleven with one player replaced by an unknown id scored %.4f "+
			"against %.4f whole; the unknown id is correctly skipped, but the squad "+
			"is genuinely a man short and must score less", broken, whole)
	}
}

// ⚠️ It must count ELEVEN, chosen legally, not the whole squad and not the top
// eleven by Score. The top eleven by score fields illegal formations — this
// fixture's ids make that testable, because a squad stacked with goalkeepers can
// only ever field one of them.
func TestXIPointsFieldsALegalElevenRatherThanTheBestEleven(t *testing.T) {
	e := benchOptimizeEngine(60)

	full := XIPoints(e, legalFifteen())
	var all float64
	for _, id := range legalFifteen() {
		if el := e.Boot.ElementByID(id); el != nil {
			all += e.Metrics(el).Score
		}
	}
	if full >= all {
		t.Errorf("the best eleven (%.4f) scored at least as much as all fifteen (%.4f); "+
			"four players are supposed to be left on the bench", full, all)
	}

	// Every goalkeeper in the fixture: i%13 == 0, so ids 1, 14, 27, 40, 53.
	keepers := []int{1, 14, 27, 40, 53}
	stacked := append(append([]int(nil), keepers...),
		xiDEF, xiDEF+1, xiDEF+2, xiDEF+3, xiDEF+4,
		xiMID, xiMID+1, xiMID+2, xiMID+3, xiMID+4)

	var keeperSum float64
	for _, id := range keepers {
		if el := e.Boot.ElementByID(id); el != nil {
			keeperSum += e.Metrics(el).Score
		}
	}
	got := XIPoints(e, stacked)

	// A legal eleven takes exactly one keeper, so at most one keeper's score can
	// be inside the total. If the implementation took the top eleven by Score it
	// could take several.
	if keeperSum <= 0 {
		t.Fatal("precondition: the fixture's goalkeepers should score something")
	}
	// A five-keeper squad cannot have all five keepers inside its eleven.
	if got >= keeperSum {
		t.Errorf("a squad of five goalkeepers scored %.4f, which is at least the sum of "+
			"all five keepers (%.4f) — a legal eleven fields exactly one", got, keeperSum)
	}
}
