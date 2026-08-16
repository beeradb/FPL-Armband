package analysis

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// childArg selects the subprocess arm of the test below.
//
// It is a command-line argument rather than an environment variable, and that is
// the whole point: this test's subject is a variable read from the environment,
// so plumbing it through the environment too would give an ambient value a way to
// put the PARENT into the child branch — where it prints and returns having
// asserted nothing, passing vacuously. Nothing in `go test`'s own invocation
// passes a bare positional argument, so this cannot collide.
const childArg = "fixture-scale-child"

// TestTheFixtureScalesRefuseAnUnreadableValue pins that FPL_ATK_FIXTURE_SCALE
// and FPL_DEF_FIXTURE_SCALE fail loudly rather than falling back to the shipped
// 1.0 — and, in the same breath, that they still DEFAULT to 1.0 when unset.
//
// The defect this guards was live: both knobs read through a lenient parser that
// returned 1 on anything ParseFloat could not read, while internal/snapshot
// stamps both names into the run fingerprint. So `FPL_DEF_FIXTURE_SCALE=1,5` — a
// comma for a decimal point — scored every cell at the shipped value and stamped
// the run as having been configured at 1.5. A fully configured-looking sweep
// measuring nothing, producing a null indistinguishable from "the ladder does
// nothing", which is close enough to what this record already believes about the
// fixture ladders that it would have been believed.
//
// ⚠️ The unset arm is not padding. The fix moved the shipped 1.0 from a single
// `return 1` inside the shared parser to a per-call-site argument written twice,
// so the default is now something that can be got wrong independently for each
// ladder. Review demonstrated the gap rather than supposing it: mutating only the
// attacking default to 0 — which flattens 1.30/1.15/0.85/0.72 to a flat 1.00 in
// every command and every replayed cell — left this test and the whole package
// green. `TestConcededPenaltyScalesWithFixtures` covers the defensive ladder;
// nothing covered the attacking one.
//
// It has to be a SUBPROCESS test. Both knobs are package-level vars read once at
// init, so nothing a test does in-process can re-run the parse — and a unit test
// on envScaleStrict alone would pass just as happily with the call sites wired
// back to a lenient parser, which is precisely the regression. This exercises the
// initialisation the sweep actually runs through.
//
// The arms are the pair AGENTS.md's standing rule asks for. Refusal alone is a
// confinement check, and a confinement check on a path that cannot carry the
// effect confirms nothing: if the binary failed to start for an unrelated reason
// every refusal arm would pass. So the liveness arms must show a value ARRIVING
// at the variable — including 0, the flatten-the-ladder control, which is the
// setting a silent fallback to 1 would destroy most expensively.
func TestTheFixtureScalesRefuseAnUnreadableValue(t *testing.T) {
	for _, a := range os.Args {
		if a == childArg {
			// Reached only if package initialisation did NOT panic. Print what
			// the knobs parsed to, so the parent can tell an accepted value from
			// a fallback.
			fmt.Printf("atk=%v def=%v\n", atkFixtureScale, defFixtureScale)
			return
		}
	}

	// The shipped arm, first: unset must still mean 1.0 on BOTH ladders.
	if out, err := runChild(t); err != nil || !hasScales(out, 1, 1) {
		t.Fatalf("with neither variable set the knobs read %q, want atk=1 def=1 (err %v). "+
			"The shipped default is now written once per call site rather than once in "+
			"the parser, so each ladder can be defaulted wrongly on its own — and a wrong "+
			"attacking default flattens 1.30/1.15/0.85/0.72 in every replayed cell",
			strings.TrimSpace(out), err)
	}

	// Anything ParseFloat cannot read, or reads as negative. "1,5" is the typo
	// that motivated this; the rest are the neighbouring ways to be wrong.
	// "1e400" overflows, which ParseFloat reports as +Inf WITH an error, so the
	// lenient parser defaulted on it too.
	unreadable := []string{"1,5", "1.5x", "one-point-five", "1.5 2.0", "-1", "-0.5", "-Inf", "1e400"}
	for _, name := range []string{"FPL_ATK_FIXTURE_SCALE", "FPL_DEF_FIXTURE_SCALE"} {
		for _, bad := range unreadable {
			out, err := runChild(t, name+"="+bad)
			if err == nil {
				t.Errorf("%s=%q started cleanly and parsed as %q — the silent fallback "+
					"is back. An unreadable scale must panic, because the fingerprint "+
					"records the value asked for while the cells score at the shipped one",
					name, bad, strings.TrimSpace(out))
				continue
			}
			// Distinguish "panicked for the right reason" from "the binary would
			// not run at all", which would make this arm pass vacuously.
			if !strings.Contains(out, name) {
				t.Errorf("%s=%q failed without naming the variable, so this arm cannot "+
					"tell a refusal from an unrelated crash:\n%s", name, bad, out)
			}
			if strings.Contains(out, "atk=") {
				t.Errorf("%s=%q reached the test body, so initialisation did not refuse it", name, bad)
			}
		}

		// Blank means unset, which is the shipped arm and not a misconfiguration:
		// a sweep that clears a variable is asking for the default back.
		for _, blank := range []string{"", " "} {
			out, err := runChild(t, name+"="+blank)
			if err != nil || !hasScales(out, 1, 1) {
				t.Errorf("%s=%q gave %q (err %v); blank means unset, which must read as "+
					"the shipped 1.0 rather than as an error",
					name, blank, strings.TrimSpace(out), err)
			}
		}
	}

	// Liveness. 1.5 is the value the motivating typo meant, and 0 is the
	// flatten-the-ladder control the comment on these vars names.
	out, err := runChild(t, "FPL_ATK_FIXTURE_SCALE=1.5", "FPL_DEF_FIXTURE_SCALE=0")
	if err != nil {
		t.Fatalf("a valid pair was refused (%v); the strict parser must accept every "+
			"setting the lenient one accepted\n%s", err, out)
	}
	if !hasScales(out, 1.5, 0) {
		t.Errorf("valid values did not arrive at the knobs: got %q, want atk=1.5 def=0. "+
			"A knob that does not arrive returns a byte-identical null, which reads "+
			"exactly like a knob that does nothing", strings.TrimSpace(out))
	}

	// And whitespace is honoured rather than silently defaulted, which is the one
	// input whose behaviour this change moved in the accepting direction.
	// ParseFloat never accepts surrounding whitespace, so trimming can only reach
	// inputs the lenient parser already defaulted on.
	out, err = runChild(t, "FPL_DEF_FIXTURE_SCALE= 1.5 ")
	if err != nil {
		t.Fatalf("a padded value was refused (%v)\n%s", err, out)
	}
	if !hasScales(out, 1, 1.5) {
		t.Errorf("a padded value gave %q, want atk=1 def=1.5 — under the lenient parser "+
			"this was one of the silent fallbacks", strings.TrimSpace(out))
	}
}

// hasScales matches the child's report exactly, rather than by substring.
//
// A substring test for "def=0" also matches "def=0.5", so it would accept a
// value that is not the one asked for — a weak assertion in a test whose whole
// subject is a value silently becoming something else.
func hasScales(out string, atk, def float64) bool {
	want := fmt.Sprintf("atk=%v def=%v", atk, def)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

// runChild re-executes this test binary with the given FPL_*_FIXTURE_SCALE
// settings and returns its combined output.
//
// Both scale variables are cleared first, so a sweep already running with one
// exported cannot leak into an arm and make it pass or fail for the wrong reason.
func runChild(t *testing.T, set ...string) (string, error) {
	t.Helper()
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "FPL_ATK_FIXTURE_SCALE=") ||
			strings.HasPrefix(kv, "FPL_DEF_FIXTURE_SCALE=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, set...)

	cmd := exec.Command(os.Args[0],
		"-test.run=^TestTheFixtureScalesRefuseAnUnreadableValue$", "-test.count=1", childArg)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}
