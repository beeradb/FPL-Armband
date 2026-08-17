package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestTheSnapshotStampCarriesNoBranchName holds shut a channel that had nobody
// standing in it.
//
// # What happened
//
// `armband snapshot` used to read the current branch from git and render it as a
// `| branch | ... |` row in the provenance table. Two banked snapshots therefore
// carried whatever that branch happened to be called. Nobody typed it into a
// file — the generator did, and a generated row has no prose for a reviewer to
// read, so every human gate was blind to it by construction.
//
// # Why the field is gone rather than filtered
//
// The obvious repair is to screen the branch name and refuse or blank it. That
// repair cannot live here: any allowlist or rejected-name check has to spell the
// names being screened in a tracked file, which publishes the thing the check
// exists to keep out. So the fix is subtractive.
//
// Nothing was lost. The commit SHA identifies the code exactly; a branch is a
// mutable label that can be renamed, deleted, or reused for unrelated work, so it
// never identified anything the commit did not identify better. Nothing in the
// tree ever read the row back — it was written, rendered, and never consumed.
//
// # The three checks, and why one alone would not hold
//
// The rendered-output check alone would pass the moment someone re-added the
// field and rendered it under a different label. The struct check alone would
// pass if a caller began interpolating a branch name into `Notes`. The command
// check alone says nothing about this package. Together they cover the field, the
// row, and the lookup that fed them.
func TestTheSnapshotStampCarriesNoBranchName(t *testing.T) {
	// 1. The type cannot carry it. A field is how it came back last time it was
	//    wanted, and a reflective check names the exact thing not to re-add.
	for _, f := range reflect.VisibleFields(reflect.TypeOf(Inputs{})) {
		if strings.Contains(strings.ToLower(f.Name), "branch") {
			t.Errorf("Inputs.%s is back: a snapshot must not carry a branch name. "+
				"Read this test's doc comment before restoring it.", f.Name)
		}
	}

	// 2. A fully populated render emits no branch row. Populated rather than
	//    zero-valued, because a zero Inputs passes this check even with the field
	//    present and rendered — which is precisely the state that shipped this.
	//
	//    ⚠️ Notes carries a probe deliberately. Operator notes render as free
	//    prose, and the command builds note strings out of environment-derived
	//    paths, so a check keyed only on a table row does not cover them.
	const probe = "branch-name-probe"
	md, _ := Render(Inputs{
		Commit:    "0123456789abcdef",
		Dirty:     true,
		CellsPath: "stats/out/cells.csv",
		ModelPath: "stats/out/model.csv",
		Notes:     []string{"an operator note about " + probe},
	})
	if strings.Contains(strings.ToLower(md), "| branch") {
		t.Error("the provenance table still renders a branch row")
	}
	if !strings.Contains(md, probe) {
		t.Error("an operator note did not render at all, so the free-prose path " +
			"this check exercises is no longer the one the command uses")
	}
}

// TestTheSnapshotCommandDoesNotAskGitForTheBranch checks the other half: the
// value never has to exist in the first place.
//
// Scanning source rather than behaviour is deliberate. The property worth holding
// is that the command never obtains the string, not that one code path currently
// declines to print it. A behavioural test would have to run on a branch whose
// name demonstrates the problem, which means creating one and writing the name
// down.
//
// The scan is deliberately loose — any git invocation resolving a symbolic name.
// A tighter match on one spelling passes on the next spelling of the same lookup,
// and this guard exists because the previous version of this code had no guard.
func TestTheSnapshotCommandDoesNotAskGitForTheBranch(t *testing.T) {
	root := repoRootForBranchScan(t)
	src, err := os.ReadFile(filepath.Join(root, "cmd", "armband", "snapshot.go"))
	if err != nil {
		t.Fatalf("cannot read the snapshot command: %v", err)
	}
	for _, probe := range []string{"abbrev-ref", "symbolic-ref", "show-current",
		"name-rev", "gitBranch"} {
		if strings.Contains(string(src), probe) {
			t.Errorf("cmd/armband/snapshot.go asks git for the branch name (%q). "+
				"The snapshot stamps the commit; a branch name is an unreviewed "+
				"string in a published file. See "+
				"TestTheSnapshotStampCarriesNoBranchName.", probe)
		}
	}
}

// repoRootForBranchScan resolves the repository root, failing rather than
// skipping. A skip reports the same green as a run that found nothing, which is
// the failure this area keeps paying for.
func repoRootForBranchScan(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot resolve the repository root: %v", err)
	}
	return strings.TrimSpace(string(out))
}
