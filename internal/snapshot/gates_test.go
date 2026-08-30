package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestComplexityRatchetGate, TestWorkflowLintGate and TestClaimsRegistryGate move
// three CI gates from "a step in ci.yml that go test ./... never touches" to "a
// package go test ./... already runs". Before this file, all three were Python
// scripts invoked only from .github/workflows/ci.yml: nothing failed until a push,
// and `go test ./...` — the one command AGENTS.md tells a developer to run before
// shipping anything touching config.json, an FPL_* switch, or stats/*.R — could not
// see them. Rung 2 of the shift-left ladder is exactly this: the same check, in
// both places, from one invocation, so CI and a local run cannot drift apart.
//
// # Why t.Fatalf and never t.Skip
//
// This package's whole argument, made at length in staleness_test.go and
// branchstamp_test.go, is that a guard which turns itself off silently is
// indistinguishable from one that passed. A test that skips because gocyclo, or
// python3, or PyYAML happens to be missing is that same failure wearing a different
// name: the developer sees green and ships. So a missing prerequisite is fatal
// here, with the exact command to fix it, never a skip.
//
// # Why the selftest runs first
//
// Same reason ci.yml runs "Prove the X can fail" immediately before "X": a green
// run of a checker that has never been shown to go red is not evidence of
// anything. Each script ships a --selftest that mutates its own inputs and asserts
// the checker catches it; running that here, inside the same test, means the proof
// travels with the checker rather than living in a separate CI step someone can
// delete without noticing the checker went blind.
func mustHavePython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatalf("python3 is not on PATH, so this gate cannot run.\n\n" +
			"Install it, e.g. `apt-get install python3` (Debian/Ubuntu) or " +
			"`brew install python3` (macOS), then re-run `go test ./internal/snapshot/...`.")
	}
}

// mustHavePyYAML fails, rather than skips, when python3 cannot `import yaml`.
// workflow-lint.py and claims.py both depend on it and neither this repository's
// go.mod nor its module graph can pin a Python package — see requirements.txt,
// which the "Install pinned Python gate dependencies" CI step installs from.
func mustHavePyYAML(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("python3", "-c", "import yaml")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("python3 cannot `import yaml`, so this gate cannot run.\n\n"+
			"Install it: python3 -m pip install -r requirements.txt\n\n%s", out)
	}
}

// mustHaveGocyclo fails, rather than skips, when the complexity ratchet's own
// dependency is absent. The version is pinned so a local run and CI's "Install
// gocyclo" step measure with the same tool.
func mustHaveGocyclo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gocyclo"); err != nil {
		t.Fatalf("gocyclo is not on PATH, so the complexity ratchet cannot run.\n\n" +
			"Install it:\n\n" +
			"\tgo install github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0\n\n" +
			"and make sure $(go env GOPATH)/bin is on PATH.")
	}
}

// runGateScript runs a scripts/<name> Python gate rooted at root, and fails the
// test with the script's combined stdout+stderr on a nonzero exit — so a failure
// here tells the developer WHY without them having to re-run anything by hand.
func runGateScript(t *testing.T, root, name string, args ...string) {
	t.Helper()
	cmd := exec.Command("python3", append([]string{filepath.Join("scripts", name)}, args...)...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("python3 scripts/%s %s failed: %v\n\n%s",
			name, strings.Join(args, " "), err, out)
	}
}

// TestComplexityRatchetGate is scripts/complexity-ratchet.py, moved from a
// standalone ci.yml step to here. See that script's own doc comment for why the
// gate is a ratchet on NEW/WORSE complexity rather than a global threshold.
func TestComplexityRatchetGate(t *testing.T) {
	declareInputs(t, mustRepoRoot(t), "scripts/complexity-ratchet.py", "internal/*/*.go", "cmd/*/*.go")
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("could not resolve the repository root: %v", err)
	}
	mustHavePython3(t)
	mustHaveGocyclo(t)

	// Part 1: prove the guard can still fail before trusting its green run.
	runGateScript(t, root, "complexity-ratchet.py", "--selftest")

	// Part 2: the real check, against the tree as it stands.
	runGateScript(t, root, "complexity-ratchet.py")
}

// TestWorkflowLintGate is scripts/workflow-lint.py, moved from a standalone
// ci.yml step to here. It exists because a workflow file GitHub cannot parse
// reports failure with zero jobs and zero logs — nothing else in CI could ever
// have caught that on its own, which is why this check has to run somewhere.
func TestWorkflowLintGate(t *testing.T) {
	declareInputs(t, mustRepoRoot(t), "scripts/workflow-lint.py", ".github/workflows/*.yml")
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("could not resolve the repository root: %v", err)
	}
	mustHavePython3(t)
	mustHavePyYAML(t, root)

	runGateScript(t, root, "workflow-lint.py", "--selftest")
	runGateScript(t, root, "workflow-lint.py")
}

// TestClaimsRegistryGate is scripts/claims.py, moved from a standalone ci.yml
// step to here. It re-derives every quantitative claim in stats/claims.yaml
// rather than trusting a number written down beside it — see that script's own
// doc comment for why a stored VALUE is the design already proved to rot here.
func TestClaimsRegistryGate(t *testing.T) {
	declareInputs(t, mustRepoRoot(t), "scripts/claims.py", "stats/claims.yaml")
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("could not resolve the repository root: %v", err)
	}
	mustHavePython3(t)
	mustHavePyYAML(t, root)

	runGateScript(t, root, "claims.py", "--selftest")
	runGateScript(t, root, "claims.py")
}

// declareInputs makes Go's test cache aware of files a SUBPROCESS reads.
//
// The cache records what the test binary itself opens, via internal/testlog. A
// script spawned with exec.Command is invisible to it, so without this a
// mutation to stats/claims.yaml or a workflow file leaves the cached result
// standing and the gate silently does not run -- verified: mutate a claim,
// re-run, get "ok (cached)". staleness_test.go records the same failure for the
// same reason and answers it with -count=1 at the caller.
//
// Reading the files here is the other half of that answer and the half a caller
// cannot forget: it declares the dependency, so an edit to any of them
// invalidates the cache on its own.
//
// ⚠️ It is NOT complete for claims.py, whose inputs are the commands it re-runs
// and therefore effectively the whole tree. -count=1 in ci.yml covers that;
// this covers the local edit-and-rerun loop AGENTS.md points a developer at.
func declareInputs(t *testing.T, root string, globs ...string) {
	t.Helper()
	for _, g := range globs {
		matches, err := filepath.Glob(filepath.Join(root, g))
		if err != nil {
			t.Fatalf("globbing %s: %v", g, err)
		}
		if len(matches) == 0 {
			t.Fatalf("%s matched no files; this gate's cache dependency is not "+
				"being declared and an edit to it would leave a stale green", g)
		}
		for _, m := range matches {
			if _, err := os.ReadFile(m); err != nil {
				t.Fatalf("reading %s to declare it as a cache input: %v", m, err)
			}
		}
	}
}

// mustRepoRoot is repoRoot with the error turned into a test failure, so the
// three gate tests read the same way as their neighbours.
func mustRepoRoot(t *testing.T) string {
	t.Helper()
	r, err := repoRoot()
	if err != nil {
		t.Fatalf("locating the repository root: %v", err)
	}
	return r
}
