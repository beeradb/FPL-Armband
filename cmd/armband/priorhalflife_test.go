package main

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"armband/internal/config"
)

// `prior_half_life` is the one FPL_WEIGHT key that cannot reach a replay, and it
// looks exactly like the keys that can. These two tests pin the warning that says
// so, and the fact that makes the warning true.
//
// The hazard is worth the file. `Weights.PriorHalfLife` and
// `SimConfig.PriorHalfLife` are different fields with the same name and the same
// meaning; the engine reads NEITHER on its own, and the replay honours the second
// only when `SimConfig.OlderPriors` is populated, which happens nowhere outside
// `cmd/priorblend` and one test. So a sweeper who sets
// `FPL_WEIGHT=prior_half_life=1` and runs `armband backtest` gets a byte-identical
// run and no indication that the knob did nothing — which is this project's
// signature failure, "one quantity, two implementations". AGENTS.md carries it as
// a standing rule and names four other instances.

// TestPriorHalfLifeSaysItCannotReachTheReplay fails if the override stops warning.
func TestPriorHalfLifeSaysItCannotReachTheReplay(t *testing.T) {
	t.Setenv("FPL_WEIGHT", "prior_half_life=1")

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	var cfg config.Config
	applyWeightOverrides(&cfg)

	w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stderr: %v", err)
	}
	got := string(out)

	if cfg.Weights.PriorHalfLife != 1 {
		t.Errorf("the override stopped applying: PriorHalfLife = %v, want 1",
			cfg.Weights.PriorHalfLife)
	}
	// Assert on the claim rather than the sentence, so rewording is free and
	// deleting the warning is not.
	for _, want := range []string{"prior_half_life", "inert", "OlderPriors"} {
		if !strings.Contains(got, want) {
			t.Errorf("setting prior_half_life printed no warning naming %q.\n"+
				"That warning is the only thing standing between a sweeper and a\n"+
				"byte-identical run they would write down as a null. Got:\n%s",
				want, got)
		}
	}
}

// TestNothingWiresPriorHalfLifeIntoTheReplay guards the claim the warning actually
// makes, which is about `armband backtest` and not about `internal/analysis`.
//
// **An earlier version of this file scanned the wrong tree**, and code review caught
// it by mutation: the route that makes the warning false does not pass through
// `internal/analysis` at all. That package consumes a `PriorSeason` interface; the
// multi-season machinery is `newPriorIndexMulti` in `internal/backtest`, reached
// through `SimConfig.priors()`. So the seam that would ship this feature is the
// `backtest.SimConfig` literal in cmd/armband/backtest.go — add `PriorHalfLife:
// cfg.Weights.PriorHalfLife` and a populated `OlderPriors` there and the replay
// honours it, while a scan of `internal/analysis` still reports green and the
// warning three lines above goes on telling every sweeper the key is inert.
//
// Recorded because it is this project's own failure arriving inside the guard
// written to prevent it: a test that checks a real property of the wrong component.
//
// So this scans the tree that builds the replay's config. It is deliberately blunt —
// any mention outside a comment, not just an assignment — because the point is to
// make someone read the warning, not to parse Go.
func TestNothingWiresPriorHalfLifeIntoTheReplay(t *testing.T) {
	// Files allowed to name the fields, and why. An entry that stops being needed
	// fails too, so removing a pin removes its licence rather than leaving a hole.
	allowed := map[string]string{
		"cmd/armband/sweep.go": "the warning's own text names both fields, in a " +
			"string literal that no comment-strip will remove",
		// `asof` reads the key to REFUSE on it, not to honour it. The live path
		// blends several prior seasons through recent.LoadPriors, which needs a
		// client an as-of run does not have and must not acquire; rather than
		// silently score a different model, the command errors out when the key is
		// set. So this is a reader that guarantees the value is never acted on —
		// the opposite of wiring it — and `armband backtest` is untouched, which is
		// the clause sweep.go's warning actually makes.
		// TestAsOfRefusesWhenPriorHalfLifeWouldBlendMultipleSeasons pins the
		// behaviour this licence is granted for.
		"cmd/armband/asof.go": "reads the key only to refuse the run; does not " +
			"reach the replay, and does not honour the blend",
	}
	used := map[string]bool{}

	root := filepath.Join("..", "..")
	var found []string
	for _, tree := range []string{"cmd/armband", "internal/backtest"} {
		err := filepath.WalkDir(filepath.Join(root, tree),
			func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || !strings.HasSuffix(path, ".go") ||
					strings.HasSuffix(path, "_test.go") {
					return nil
				}
				rel, _ := filepath.Rel(root, path)
				rel = filepath.ToSlash(rel)
				src, rerr := os.ReadFile(path)
				if rerr != nil {
					return rerr
				}
				for i, line := range strings.Split(string(src), "\n") {
					if !mentionsPriorBlend(stripLineComment(line)) {
						continue
					}
					if _, ok := allowed[rel]; ok {
						used[rel] = true
						continue
					}
					found = append(found, rel+":"+itoa(i+1)+" "+strings.TrimSpace(line))
				}
				return nil
			})
		if err != nil {
			t.Fatalf("walking %s: %v", tree, err)
		}
	}

	// internal/backtest legitimately declares and reads SimConfig's own fields, so
	// the expected baseline is not zero. Pin the sites rather than the count: a new
	// one is what matters, and naming them is what tells the next reader where the
	// real seam is.
	expected := map[string]bool{
		"internal/backtest/simulate.go":    true, // declares SimConfig's two fields
		"internal/backtest/omniscience.go": true, // SimConfig.priors() reads them
		// The LIVE consumer, and the one the warning names as the only reader:
		// main.go gates recent.LoadPriors on Weights.PriorHalfLife. It sits before
		// the command switch, so it fires for `armband backtest` too — see the
		// warning text, which says so because a reader who is told the key is inert
		// and then watches it fetch four seasons of history will not believe it.
		"cmd/armband/main.go": true,
	}
	var novel []string
	for _, f := range found {
		file := f[:strings.Index(f, ":")]
		if !expected[file] {
			novel = append(novel, f)
		}
	}

	if len(novel) > 0 {
		t.Errorf("something now wires prior_half_life toward the replay:\n  %s\n\n"+
			"That is a real change and probably a welcome one — but the warning in\n"+
			"cmd/armband/sweep.go says the key is INERT on `armband backtest`, and\n"+
			"it would now be false. Update the warning in the same commit.",
			strings.Join(novel, "\n  "))
	}
	for file, why := range allowed {
		if !used[file] {
			t.Errorf("%s no longer names the prior-blend fields, so its licence in "+
				"this test is dead and should be removed (%s)", file, why)
		}
	}
}

// TestNothingInAnalysisReadsPriorHalfLife guards the warning's other clause: that
// the field has no consumer in the scoring engine.
//
// Narrower than it looks, and kept separate from the replay scan above because the
// two clauses can go false independently.
func TestNothingInAnalysisReadsPriorHalfLife(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "analysis")

	var readers []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(src), "\n") {
			code := stripLineComment(line)
			if !strings.Contains(code, "PriorHalfLife") {
				continue
			}
			// The struct field declaration is not a read. Matched on FIELDS rather
			// than on the literal "PriorHalfLife float64", which review found is
			// gofmt-alignment-fragile: add a longer field name beside it and gofmt
			// pads the gap to several spaces, the skip misses, and this test fails
			// spuriously on a change with nothing to do with the feature.
			if f := strings.Fields(code); len(f) >= 2 &&
				f[0] == "PriorHalfLife" && f[1] == "float64" {
				continue
			}
			rel, _ := filepath.Rel(filepath.Join("..", ".."), path)
			readers = append(readers, rel+":"+itoa(i+1)+" "+strings.TrimSpace(line))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if len(readers) > 0 {
		t.Errorf("internal/analysis now reads Weights.PriorHalfLife:\n  %s\n\n"+
			"The warning in cmd/armband/sweep.go says the only consumer is\n"+
			"recent.LoadPriors on the live path, and it is now wrong. Update it,\n"+
			"and check whether the replay's SimConfig.OlderPriors path was wired in\n"+
			"the same change or left behind.",
			strings.Join(readers, "\n  "))
	}
}

func mentionsPriorBlend(code string) bool {
	return strings.Contains(code, "PriorHalfLife") || strings.Contains(code, "OlderPriors")
}

// stripLineComment removes a trailing `//` comment.
//
// It does not understand string literals, so `f("http://x", w.PriorHalfLife)` loses
// its real reference — noted by review as the one way this loses information rather
// than adding it. Accepted: the alternative is a Go parser, the pattern does not
// occur, and the replay scan above pins its one string-literal site by name instead.
func stripLineComment(line string) string {
	if j := strings.Index(line, "//"); j >= 0 {
		return line[:j]
	}
	return line
}

// itoa keeps the scan above free of a strconv import for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
