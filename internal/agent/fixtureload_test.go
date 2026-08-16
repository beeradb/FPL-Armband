package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// TestFixtureLoadIsReportedToTheAgent — a scoring multiplier the agent cannot see
// is one it has to take on trust.
//
// Fixture load is matches per gameweek over the horizon: 2.0 in a double gameweek,
// below 1.0 when a club blanks. It is a shipped multiplier on expected points and
// the largest reliable gain measured in this project, and for a while it appeared
// in no tool output at all — the field existed on PlayerMetrics and nothing carried
// it to the model. This project's standing rule is that every scoring term is a
// *reported* multiplier, so the agent can explain a number rather than assert it.
//
// Two halves, and the second is why it went unnoticed. It must appear when there is
// something to say, and it must stay out of the payload when there is not: tool
// results are replayed on every subsequent API call, so a column reading 1.0 for
// seven hundred players is paid for repeatedly and forever.
func TestFixtureLoadIsReportedToTheAgent(t *testing.T) {
	ordinary := row(analysis.PlayerMetrics{Name: "Ordinary", FixtureLoad: 1})
	if ordinary.Load != 0 {
		t.Errorf("an ordinary fixture run set Load to %v; it must stay zero so the "+
			"field is omitted", ordinary.Load)
	}
	if got := mustJSON(t, ordinary); strings.Contains(got, "fixtures_per_gameweek") {
		t.Errorf("an ordinary player's row carries fixtures_per_gameweek:\n%s\n"+
			"Every row of every search is replayed on each later API call, so a field "+
			"that is 1.0 for almost everyone is pure cost", got)
	}

	double := row(analysis.PlayerMetrics{Name: "Doubling", FixtureLoad: 1.4})
	if double.Load != 1.4 {
		t.Errorf("a double gameweek set Load to %v, want 1.4", double.Load)
	}
	if got := mustJSON(t, double); !strings.Contains(got, `"fixtures_per_gameweek":1.4`) {
		t.Errorf("a doubling club's row does not report it:\n%s", got)
	}

	blank := row(analysis.PlayerMetrics{Name: "Blanking", FixtureLoad: 0.8})
	if blank.Load != 0.8 {
		t.Errorf("a blank set Load to %v, want 0.8 — a club with no fixture is as much "+
			"a scoring fact as one with two", blank.Load)
	}

	// The note travels with the field and only with it. A number whose meaning
	// depends on the consumer is worse than no number, so no payload may carry the
	// field silently.
	e := horizonEngine(5)
	plain := map[string]any{}
	if noteFixtureLoad(plain, e, []playerRow{ordinary}) {
		t.Error("an all-ordinary result set was given the explanatory note")
	}
	if _, ok := plain[fixtureLoadNoteKey]; ok {
		t.Error("the note was written into a payload that has nothing to explain")
	}
	noted := map[string]any{}
	if !noteFixtureLoad(noted, e, []playerRow{ordinary}, []playerRow{double}) {
		t.Error("a result set containing a double was not given the explanatory note")
	}
	if _, ok := noted[fixtureLoadNoteKey]; !ok {
		t.Errorf("noteFixtureLoad reported success without writing %q", fixtureLoadNoteKey)
	}
}

// TestFixtureLoadNoteSaysWhetherItIsAlreadyInTheScore — the note has to be right
// about the one thing the agent would otherwise get wrong.
//
// At the shipped five-gameweek horizon the multiplier is *reported and not
// applied*: it is confined to the imminent-week eleven and the transfer objective,
// because applying it to squad building costs about 53 points a season. So `score`
// does not contain it, and an agent told otherwise would double-count a double
// gameweek. Configure the horizon to 1 and the same engine is the imminent-week
// view, and it does contain it. The note asks the engine rather than assuming
// either.
func TestFixtureLoadNoteSaysWhetherItIsAlreadyInTheScore(t *testing.T) {
	e := horizonEngine(5)
	if e.FixtureLoadInScore() {
		t.Fatal("a five-gameweek engine reports the load as already applied to Score; " +
			"the term ships confined to the imminent-week view and the transfer objective")
	}
	if note := fixtureLoadNote(e); !strings.Contains(note, "NOT inside the `score`") {
		t.Errorf("the horizon note does not say the multiplier is outside `score`:\n%s", note)
	}

	w := horizonEngine(1)
	if !w.FixtureLoadInScore() {
		t.Fatal("a horizon-1 engine reports the load as not applied to Score; that engine " +
			"*is* the imminent-week view, which is the one place Score carries it")
	}
	if note := fixtureLoadNote(w); !strings.Contains(note, "already inside `score`") {
		t.Errorf("the horizon-1 note does not say the multiplier is inside `score`:\n%s", note)
	}
}

// horizonEngine is an empty engine at a given fixture horizon — enough to answer
// FixtureLoadInScore, which is all these tests ask of it.
func horizonEngine(horizon int) *analysis.Engine {
	w := analysis.DefaultWeights()
	w.Horizon = horizon
	return analysis.NewEngineFull(&fpl.Bootstrap{}, nil, w,
		analysis.Congestion{}, analysis.RoleRisk{})
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
