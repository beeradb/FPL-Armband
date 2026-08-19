package capture

import (
	"path/filepath"
	"testing"
)

func liveCaptureDir() string {
	return filepath.Join("..", "..", "data", "captures", LiveCapture)
}

// TestReplayDecodesACommittedCapture is the deterministic-input guarantee that the visual
// regression suite rests on. If this fails, no screenshot comparison downstream means
// anything, because the inputs moved.
func TestReplayDecodesACommittedCapture(t *testing.T) {
	boot, fixtures, err := Replay(liveCaptureDir())
	if err != nil {
		t.Fatalf("replaying %s: %v", LiveCapture, err)
	}
	if len(boot.Elements) < 500 {
		t.Errorf("the capture decoded to %d players; a Premier League bootstrap carries "+
			"about 600", len(boot.Elements))
	}
	if len(boot.Teams) != 20 {
		t.Errorf("the capture decoded to %d clubs, want 20", len(boot.Teams))
	}
	if len(boot.Events) != 38 {
		t.Errorf("the capture decoded to %d gameweeks, want 38", len(boot.Events))
	}
	if len(fixtures) == 0 {
		t.Error("the capture decoded to no fixtures; the FDR strips would be empty")
	}

	// Season empty is the whole point — it means the live game, and so today's scoring
	// rules. A capture that arrived with a season set would be pinning the engine to
	// historical rules without anyone having asked.
	if boot.Season != "" {
		t.Errorf("the replayed bootstrap carries season %q; empty means the live game, "+
			"and these are live bytes", boot.Season)
	}
}

// TestTheCaptureIsPinnedToTheGameweekTheDesignAssumes pins the fixture's identity.
//
// The design was drawn against GW1 opening on Friday 21 August at 17:30, and every
// screenshot in the handoff shows that deadline. A golden image compared against a
// different gameweek would differ everywhere for a reason that is not a regression, so the
// capture's own idea of which week it is has to be asserted rather than assumed.
func TestTheCaptureIsPinnedToTheGameweekTheDesignAssumes(t *testing.T) {
	boot, _, err := Replay(liveCaptureDir())
	if err != nil {
		t.Fatalf("replaying %s: %v", LiveCapture, err)
	}
	var next int
	for _, e := range boot.Events {
		if e.IsNext {
			next = e.ID
		}
	}
	if next != 1 {
		t.Errorf("the capture's next gameweek is %d, want 1 — this fixture is meant to "+
			"be the pre-season state the design was drawn against", next)
	}
	deadline := boot.Events[0].DeadlineTime.UTC().Format("2006-01-02T15:04Z")
	if deadline != "2026-08-21T17:30Z" {
		t.Errorf("GW1's deadline in the capture is %s, want 2026-08-21T17:30Z", deadline)
	}
}

// TestReplayRefusesADirectoryThatIsNotACapture pins that a wrong path fails loudly.
//
// A test fixture that silently resolved to an empty bootstrap would produce an empty squad
// and a screenshot of an empty pitch, which compares fine against a golden of an empty
// pitch. Failing here is what stops that.
func TestReplayRefusesADirectoryThatIsNotACapture(t *testing.T) {
	if _, _, err := Replay(t.TempDir()); err == nil {
		t.Error("Replay accepted a directory with no capture in it")
	}
}
