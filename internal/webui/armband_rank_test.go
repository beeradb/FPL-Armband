package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestXpForReadsThisWeekScores pins the live picker's sort key against the
// server. app.js used to rank on Player.xp (the horizon average) under a
// "this week" label. The ranking still happens in the client — openArmbandPicker
// sorts by xpFor — so the pin is that xpFor reads gameweeks[].week_xp first,
// which is horizon-1 Score copied off WeekView.
func TestXpForReadsThisWeekScores(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("assets", "static", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	start := strings.Index(js, "function xpFor(p)")
	if start < 0 {
		t.Fatal("xpFor is missing from app.js")
	}
	end := strings.Index(js[start:], "\nfunction ")
	if end < 0 {
		t.Fatal("could not bound xpFor")
	}
	body := js[start : start+end]
	if !strings.Contains(body, "week_xp") {
		t.Error("xpFor does not read week_xp; the picker labelled this week is ranking the horizon average")
	}
	if !strings.Contains(js, "openArmbandPicker") {
		t.Fatal("openArmbandPicker is missing")
	}
	if !strings.Contains(js, ".sort((a,b)=>xpFor(b)-xpFor(a))") {
		t.Error("openArmbandPicker is not sorting on xpFor")
	}
}
