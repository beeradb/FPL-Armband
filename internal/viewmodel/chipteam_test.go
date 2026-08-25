package viewmodel

import (
	"testing"

	"armband/internal/analysis"
)

// TestBuildChipTeamCarriesRebuildFailure pins the viewmodel half of the
// /api/wildcard defect: buildChipTeam used to read only wv.Squad/.XI/.Bench,
// never wv.Rebuilt or wv.RebuildFailed. A failed rebuild (Optimize errored,
// so wv.Squad is the squad passed in, unchanged) and a successful rebuild
// that happens to confirm the same squad both produce Changes: 0, Out: nil
// from the diff against today -- so without RebuildFailed making the trip
// across this boundary, the two are indistinguishable in the JSON this
// public, ungated, 300s-cached route serves. See
// analysis.WeekView.RebuildFailed and ChipTeam.RebuildFailed for the fuller
// account.
func TestBuildChipTeamCarriesRebuildFailure(t *testing.T) {
	squad := []analysis.PlayerMetrics{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}

	t.Run("failed rebuild", func(t *testing.T) {
		wv := analysis.WeekView{
			Squad:         squad,
			Rebuilt:       false,
			RebuildFailed: true,
			Caveat:        "The wildcard could not be rebuilt for this gameweek.",
		}
		// today equals wv.Squad, matching the real failure mode exactly: the
		// rebuild block leaves weekSquad as the squad passed in, and callers
		// pass the account's own current fifteen as both squad and today.
		ct := buildChipTeam(wv, 100, nil, nil, squad)

		if !ct.RebuildFailed {
			t.Fatal("ChipTeam.RebuildFailed must be true when the WeekView's is -- " +
				"buildChipTeam is silently dropping the one signal that a rebuild " +
				"did not run")
		}
		if ct.RebuildCaveat == "" {
			t.Error("ChipTeam.RebuildCaveat must carry the WeekView's Caveat when " +
				"RebuildFailed is true -- an empty string here is the original bug: " +
				"a failed rebuild with nothing to say about it")
		}
		if ct.Changes != 0 || ct.Out != nil {
			t.Errorf("a failed rebuild passes the squad through UNCHANGED, so the "+
				"plain diff against today must read 0 changes -- got Changes=%d "+
				"Out=%v. That is expected; RebuildFailed is what keeps this from "+
				"being mistaken for a real recommendation", ct.Changes, ct.Out)
		}
	})

	t.Run("successful rebuild", func(t *testing.T) {
		wv := analysis.WeekView{
			Squad:   squad,
			Rebuilt: true,
			// A thin-evidence caveat can co-occur with a SUCCESSFUL rebuild --
			// it is a different message from the "did not run" one (see
			// WeekView.Caveat's own comment) and must not leak into
			// RebuildCaveat, which readers are told means "this did not run".
			Caveat: "Only 1 gameweek has been played, so the minutes behind this fifteen are mostly last season.",
		}
		ct := buildChipTeam(wv, 100, nil, nil, squad)

		if ct.RebuildFailed {
			t.Error("a successful rebuild must never also read as a failed one -- " +
				"the two are meant to be mutually exclusive")
		}
		if ct.RebuildCaveat != "" {
			t.Errorf("RebuildCaveat must stay empty on a successful rebuild -- got "+
				"%q. The thin-evidence note belongs to ChipTeams.Caveat (the "+
				"page-level field), not this chip-specific 'did not run' slot",
				ct.RebuildCaveat)
		}
	})
}
