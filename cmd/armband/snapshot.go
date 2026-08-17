package main

// `armband snapshot` — render the model-and-harness accuracy snapshot.
//
// The requirement this satisfies is that a snapshot is a **side effect of running
// a sweep, not a discipline to remember**. Two thirds of that is already true
// without this command: a sweep with FPL_CELLS set writes its own provenance, and
// a diagnostic with FPL_MODEL_CSV set writes its own numbers. What is left is the
// rendering, and it needs R, so it cannot live inside `go test`.
//
// So this command does the remembering instead. It invokes the R inference itself
// rather than requiring a separate step, defaults every path to the conventional
// one, finds the previous snapshot on its own, and writes into a dated directory
// whose name sorts chronologically. The intended interaction is one line with no
// arguments.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"armband/internal/config"
	"armband/internal/snapshot"
)

const snapshotHelp = `armband snapshot — model-and-harness accuracy snapshot

Reads the per-cell CSVs a sweep already emits and renders a dated, stamped
snapshot: what the scoring model gets right about football, and what size of
effect the replay harness can see at all.

Typical use, after a sweep and the calibration diagnostics have run:

  armband snapshot

Flags:
  -cells string      Per-cell CSV from a sweep (default /tmp/cells.csv)
  -model string      Model-accuracy CSV from the diagnostics (default /tmp/model.csv)
  -out string        Snapshot root; a dated subdirectory is created (default stats/snapshots,
                     which is tracked by git — the snapshot is meant to be committed)
  -inference string  Where the R tables are written (default stats/out)
  -note string       A caveat to stamp in. Repeatable.
  -no-r              Do not run the R inference; read whatever is already in -inference
  -constants         Print the constants in force at each sweep and exit
  -previous string   Diff against this snapshot directory instead of the latest

Every path is optional and a missing input is reported in the snapshot rather
than being fatal: a section that is absent must never read like a section that
had nothing to say.

The one exception is the baseline. If several snapshots already hold figures and
none of their recorded commits is in this history — after a rebase, or in a
shallow clone — this refuses rather than guessing which one to diff against, and
-previous names it. A guessed baseline is not a missing section; it is a moved
figures table that reads exactly like a real one.
`

func runSnapshot(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, snapshotHelp) }
	var (
		modelCSV = fs.String("model", "/tmp/model.csv", "model-accuracy CSV")
		// Inside the repository on purpose, and it must not be gitignored.
		//
		// A default run therefore leaves an untracked directory behind, which looks
		// like a command dirtying the tree. It is the deliverable: the snapshot is
		// the accuracy record, it is meant to be committed, and the *next* snapshot
		// finds it with snapshot.FindPrevious and diffs against it. Ignore this path
		// and every future record becomes invisible to git, so a fresh clone would
		// diff against whatever was last committed by hand — which is the
		// hand-maintained discipline this whole artefact exists to remove.
		//
		// Moving the default outside the repository fails the same way from the other
		// end: the file you are supposed to commit lands somewhere you have to
		// remember to fetch it from. The output below names the directory and says it
		// wants committing, which is the part that was actually missing.
		//
		// -inference is different and *is* ignored: stats/out holds R's intermediate
		// tables, which are re-derivable from the same CSV on demand.
		outRoot   = fs.String("out", filepath.Join("stats", "snapshots"), "snapshot root")
		infDir    = fs.String("inference", filepath.Join("stats", "out"), "R output directory")
		noR       = fs.Bool("no-r", false, "do not run the R inference")
		showConst = fs.Bool("constants", false, "print the constants in force and exit")
		prevDir   = fs.String("previous", "", "diff against this snapshot directory")
	)
	var cellPaths, notes stringList
	fs.Var(&cellPaths, "cells", "per-cell CSV from a sweep; repeatable")
	fs.Var(&notes, "note", "a caveat to stamp in; repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(cellPaths) == 0 {
		cellPaths = stringList{"/tmp/cells.csv"}
	}

	// --- inputs, each optional ------------------------------------------------

	var (
		sweeps   []snapshot.Sweep
		infs     []snapshot.Inference
		problems []string
		usable   []string // cells files that parsed, in order
		labels   []string // the sweep each usable file contains, for naming
		// perFile is the same information unjoined, so the inference-versus-cells
		// check can compare against this file's labels rather than every file's.
		perFile [][]string
	)
	for _, cells := range cellPaths {
		rows, err := snapshot.ReadCells(cells)
		if os.IsNotExist(err) {
			problems = append(problems, fmt.Sprintf("no cells file at %s", cells))
			continue
		}
		if err != nil {
			return err
		}
		prov, perr := snapshot.ReadProvenance(snapshot.ProvenancePath(cells))
		if perr != nil {
			if !os.IsNotExist(perr) {
				problems = append(problems, fmt.Sprintf("provenance unreadable: %v", perr))
			}
			prov = map[string]snapshot.Provenance{}
		}
		got := snapshot.GroupSweeps(rows, prov)
		sweeps = append(sweeps, got...)
		usable = append(usable, cells)
		// A sweep label means more to a reader than a path in /tmp. Several sweeps in
		// one file are joined rather than truncated: R pools them into one set of
		// components, so naming only the first would misdescribe the figure.
		var names []string
		for _, s := range got {
			names = append(names, s.Label)
		}
		if len(names) == 0 {
			names = []string{filepath.Base(cells)}
		}
		labels = append(labels, strings.Join(names, " + "))
		perFile = append(perFile, names)
	}

	if *showConst {
		return printConstants(sweeps)
	}

	// --- the R inference, once per cells file ---------------------------------
	//
	// Once per file rather than once overall, because the minimum detectable effect
	// is a property of the comparison rather than of the harness: pooling two
	// sweeps' paired differences would average the resolution of a
	// mechanism-certain change with that of a marginal scoring constant and report
	// a figure describing neither.
	//
	// Run here rather than left to the operator. Every standard error in the
	// snapshot is R's arithmetic, so a stale directory would put last week's
	// resolution beside this week's model figures — an orphaned measurement, which
	// is the failure this whole artefact exists to prevent. Cheap enough to redo
	// every time: it reads the same CSV and replays nothing.
	for i, cells := range usable {
		// Always a per-cells-file subdirectory, never -inference itself.
		//
		// It used to be the bare directory whenever there was one cells file, and a
		// twelve-cell demo run therefore overwrote stats/out/mde.csv with figures
		// that are not a replayed season — a mean paired difference of 6.1 points a
		// gameweek at t = 68 — which the next several snapshots read as current. The
		// output carried no clue which cells it came from, so nothing looked wrong.
		//
		// One directory per input makes two sweeps' tables unable to collide, and
		// the R script refuses on its own account if an explicit --out already holds
		// a different sweep's numbers. Belt and braces on purpose: this failure is
		// silent and the snapshot is the record that catches everything else.
		dir := filepath.Join(*infDir, sanitise(filepath.Base(cells)))
		if !*noR {
			if err := runInference(cells, dir); err != nil {
				problems = append(problems, fmt.Sprintf(
					"the R inference did not run for %s (%v); its harness figures will be "+
						"whatever was already in %s, which may predate these cells",
					cells, err, dir))
			}
		}
		one, err := snapshot.ReadInference(dir)
		if err != nil {
			return err
		}
		one.Source = labels[i]

		// The cells file says which sweep it holds and R's tables say which sweep
		// they were computed from. Naming the second after the first without
		// checking is how a snapshot came to describe a twelve-cell throwaway demo
		// while reporting a real sweep's numbers — twice, most recently because
		// -no-r read stale tables from a previous run's --out directory. A
		// mismatch is a hard problem rather than a note, because every harness
		// figure below it is then attributed to the wrong measurement.
		if !one.MatchesCells(perFile[i]) {
			problems = append(problems, fmt.Sprintf(
				"the inference in %s was computed from %s, but the cells file %s "+
					"contains %s — these harness figures do not belong to that sweep. "+
					"Re-run the inference on these cells (drop -no-r), or point -cells "+
					"at the file the tables came from",
				dir, strings.Join(one.Blocks, ", "), usable[i], labels[i]))
		}
		infs = append(infs, one)
	}

	var model []snapshot.Diagnostic
	if m, err := snapshot.ReadModel(*modelCSV); err == nil {
		model = m
		// ⚠️ A model CSV holding more than one run is stamped as a problem, because
		// the file appends, its path is outside the repository, and the renderer
		// keeps the newest row per (diagnostic, group, measure). "Newest" is
		// wall-clock, so on a machine where several branches share a scratch path
		// the figures can come from code the stamped commit does not contain.
		//
		// That is not hypothetical: `stats/snapshots/2026-08-15-9e743cf` shipped
		// clean-sheet calibration on 2955 team-matches where its own commit
		// produces 2870 — the 85 doubled rows `3b6a698` had already removed, and
		// `3b6a698` is an ancestor of it. See `snapshot.ModelRunIDs`.
		//
		// A problem rather than a hard failure: the count cannot distinguish "two
		// diagnostics run separately into a fresh file", which is fine, from "two
		// branches sharing /tmp", which is not. Only a commit column in the CSV can,
		// and there isn't one. So this makes the mixture visible in the artefact
		// instead of leaving it to be discovered by a figure moving on a
		// comment-only change.
		if ids, rerr := snapshot.ModelRunIDs(*modelCSV); rerr == nil && len(ids) > 1 {
			problems = append(problems, fmt.Sprintf(
				"the model CSV %s holds %d runs (newest %s wins per figure). It "+
					"appends and its path is outside the repo, so confirm every "+
					"run in it came from this commit — a stale run from another "+
					"branch is how 2026-08-15-9e743cf shipped pre-fix clean-sheet "+
					"figures under a commit that had the fix",
				*modelCSV, len(ids), ids[len(ids)-1]))
		}
	} else if os.IsNotExist(err) {
		problems = append(problems, fmt.Sprintf("no model-accuracy file at %s", *modelCSV))
		*modelCSV = ""
	} else {
		return err
	}

	// --- where it goes --------------------------------------------------------

	sha, dirty := snapshot.GitState(".")
	now := time.Now()
	dir := snapshot.DirName(now.Format("2006-01-02"), sha)
	target := filepath.Join(*outRoot, dir)

	prevName, prevValues := "", (*snapshot.Values)(nil)
	if *prevDir != "" {
		v, err := snapshot.ReadValues(filepath.Join(*prevDir, snapshot.ValuesFile))
		if err != nil {
			return fmt.Errorf("reading -previous: %w", err)
		}
		prevName, prevValues = *prevDir, v
	} else {
		n, v, err := snapshot.FindPrevious(*outRoot, dir)
		if err != nil {
			return err
		}
		prevName, prevValues = n, v
	}

	md, values := snapshot.Render(snapshot.Inputs{
		Date: now, Commit: sha, Dirty: dirty,
		Sweeps: sweeps, Inference: infs, Model: model,
		ModelPath: *modelCSV, CellsPath: strings.Join(usable, ", "),
		Notes:    append(notes, problems...),
		Previous: prevValues, PrevName: prevName,
	})

	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, snapshot.MarkdownFile),
		[]byte(md), 0o644); err != nil {
		return err
	}
	if err := snapshot.WriteValues(filepath.Join(target, snapshot.ValuesFile), values); err != nil {
		return err
	}
	if err := snapshot.WriteConstants(filepath.Join(target, snapshot.ConstantsFile), sweeps); err != nil {
		return err
	}
	// The key is what TestSnapshotCoversTheCurrentCode reads, and it is content
	// rather than a commit so a later rebase cannot orphan this snapshot.
	//
	// Digested from HEAD rather than from the index: a snapshot describes figures
	// that were *measured*, and measuring happens against a committed tree. A review
	// record is the other case and digests the index — see `IndexRev`.
	root, err := snapshot.RepoRoot(".")
	if err != nil {
		return err
	}
	digest, perPath, err := snapshot.WatchedDigest(root, "HEAD", snapshot.SnapshotWatchedPaths)
	if err != nil {
		return fmt.Errorf("digesting the watched paths: %w", err)
	}
	if err := snapshot.WriteKey(target, snapshot.Key{
		Digest: digest, RecordedAt: now, Commit: sha, PerPath: perPath,
	}); err != nil {
		return err
	}

	fmt.Printf("wrote %s\n", filepath.Join(target, snapshot.MarkdownFile))
	fmt.Printf("      %s   (figures, for the next snapshot's diff)\n", snapshot.ValuesFile)
	fmt.Printf("      %s   (the constants in force)\n", snapshot.ConstantsFile)
	fmt.Printf("      %s        (what this snapshot covers, for the staleness guard)\n", snapshot.KeyFile)
	if prevName != "" {
		fmt.Printf("diffed against %s\n", prevName)
	} else {
		fmt.Printf("no previous snapshot found; this one is the baseline\n")
	}
	// Say that the tree is now dirty and why, rather than leaving three untracked
	// files for someone to find and wonder about. They are the record, and the next
	// snapshot diffs against whatever is committed here.
	fmt.Printf("\n%s is tracked, so this is now an untracked directory. Commit it —\n"+
		"the next snapshot diffs against the newest one in the repository.\n"+
		"  git add %s && git commit -m ... -- %s\n", *outRoot, target, target)
	for _, p := range problems {
		fmt.Fprintf(os.Stderr, "note: %s\n", p)
	}
	return nil
}

// runInference shells out to the R script that owns every standard error here.
//
// Go must not compute any of it. stats/README.md sets that rule out and
// TestInferenceLivesInOnePlace enforces it: two implementations of one quantity is
// the bug class where the measured value turns out not to be the one that runs.
func runInference(cells, out string) error {
	script := filepath.Join("stats", "variance_components.R")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("%s not found — run from the repository root", script)
	}
	cmd := exec.Command("Rscript", script, "--out="+out, cells)
	cmd.Stderr = os.Stderr
	// stdout is the long human report, which belongs on the terminal for someone
	// watching and not in the snapshot: the snapshot reads the CSVs it writes.
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func printConstants(sweeps []snapshot.Sweep) error {
	if len(sweeps) == 0 {
		return fmt.Errorf("no sweeps found in the cells file")
	}
	for _, s := range sweeps {
		if !s.HasProv {
			fmt.Printf("\n%s: no provenance sidecar, so the constants in force are unknown.\n",
				s.Label)
			continue
		}
		fmt.Printf("\n%s  fingerprint %s  commit %s\n", s.Label, s.Prov.Digest, s.Prov.Commit)
		for _, c := range s.Prov.Constants {
			fmt.Printf("  %-52s %s\n", c.Path, c.Value)
		}
		if len(s.Prov.Env) == 0 {
			fmt.Printf("  %-52s %s\n", "(no FPL_* switches set)", "the shipped defaults ran")
		}
		for _, c := range s.Prov.Env {
			fmt.Printf("  env %-48s %s\n", c.Path, c.Value)
		}
	}
	return nil
}

// sanitise turns a file name into a directory name safe on any filesystem.
//
// Gives each cells file its own R output directory. Two files whose names differ
// only in punctuation would collide, which would silently make the second file's
// inference overwrite the first's — so the original extension is dropped and
// everything else is mapped, keeping the result injective enough for the handful
// of files this ever sees.
//
// variance_components.R has its own `slug` doing the same job for a hand-run with
// no --out. That is not a mirror of this function and must not become one: this
// caller always passes an explicit --out, so the two rules are never applied to
// the same run and cannot desynchronise the way the .means.csv path rule once did.
func sanitise(name string) string {
	name = strings.TrimSuffix(name, ".csv")
	var out []rune
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "cells"
	}
	return string(out)
}

// stringList collects a repeatable flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, "; ") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}
