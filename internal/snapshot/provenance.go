package snapshot

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Provenance is what a sweep records about itself, written as a side effect of
// emitting cells rather than by anybody remembering to.
//
// The one field that cannot be recovered from the cells file afterwards is
// DeclaredArms. A sweep killed under load leaves a cells file containing however
// many arms finished, and nothing in it says how many were asked for — so three
// arms of six reads downstream as a complete three-arm sweep. That has happened:
// AGENTS.md records a block killed four times at 1, 3, 3 and 4 arms of 6, whose
// missing arms were reported as "unverified rather than corrected" only because
// somebody noticed by hand. Declaring the arms up front makes the gap arithmetic.
type Provenance struct {
	Sweep        string   // sweep label, e.g. "MINHL#1"
	RunID        string   // per-process id, so two runs of one block stay separate
	Commit       string   // git HEAD at the time of the run
	Dirty        bool     // true when the working tree had uncommitted changes
	Digest       string   // Fingerprint.Digest: the constants in force
	Seasons      []string // season pairs replayed, as "cur<-prior"
	StartGWs     []int    // entry gameweeks
	BankUpTo     int      // free-transfer bank rule pinned for every cell
	DeclaredArms []string // every arm the sweep intended to run, in order
	Constants    []Constant
	Env          []Constant
	// WatchedDigest and WatchedPaths are WatchedDigest's composite and per-path
	// breakdown (see watched.go) over SnapshotWatchedPaths at Commit. Neither
	// Commit nor Digest above can answer "is this banked cells file stale":
	//
	//   - Commit is a history pointer, and this repository's history was
	//     squashed at 61bf00a ("FPL Armband v1", 2026-08-16). A commit banked
	//     before that root is not an ancestor of anything on origin/main any
	//     more, so `git merge-base --is-ancestor` returns false for it forever
	//     — not because the content changed, but because the pointer was cut
	//     loose. Most commit SHAs sitting in banked sidecars are in this state.
	//   - Digest (constants_digest, from FingerprintOf) hashes config.json's
	//     modelSubtrees plus env switches — see fingerprint.go — and nothing
	//     else. It is structurally blind to a change in internal/analysis or
	//     internal/backtest, since neither is walked to produce it: the code
	//     that turns those constants into a score can change completely and
	//     this digest does not move. It is also a false-positive detector in
	//     the other direction — congestion.status_last_verified is a plain
	//     documentary date under the "congestion" modelSubtree (see
	//     internal/analysis/congestion.go's LastVerified, read nowhere) that
	//     moves this digest without moving anything the model computes.
	//
	// WatchedDigest is a content digest over the code and shipped constants
	// (see watched.go's own comment for why it survives a rebase and the
	// squash that Commit cannot). Absent on every sidecar written before this
	// field existed — that is reported as "unknown" by the reader, not
	// silently treated as a match or a mismatch.
	WatchedDigest string
	WatchedPaths  []Constant
}

const provenanceSuffix = ".provenance.csv"

// ProvenancePath is where a cells file's provenance lives: beside it, same base.
//
// The suffix rule is duplicated in no other language. stats/*.R derives the
// .means.csv path independently and the two rules once disagreed — Go trimmed a
// ".csv" suffix and R substituted a ".csv$" anchor, which differ for a path not
// ending in .csv, and the mismatch killed a run *after* the replay had been paid
// for. Nothing outside Go reads provenance, so there is only one rule here.
func ProvenancePath(cells string) string {
	return strings.TrimSuffix(cells, ".csv") + provenanceSuffix
}

// provenanceBlock renders one sweep's provenance rows as CSV bytes, header row
// included when withHeader is true.
//
// A pure builder over a bytes.Buffer, deliberately never touching the sidecar
// file: WriteProvenance's entire job is to turn this into exactly one write(2).
// bytes.Buffer.Write never errors, so the per-row csv.Writer.Write calls below
// are genuinely infallible here — unlike the same call written over an *os.File,
// where an ignored error used to mean an ignored write failure.
func provenanceBlock(p Provenance, withHeader bool) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if withHeader {
		_ = w.Write([]string{"sweep", "run_id", "key", "value"})
	}
	put := func(k, v string) {
		_ = w.Write([]string{p.Sweep, p.RunID, k, v})
	}
	put("commit", p.Commit)
	put("dirty", strconv.FormatBool(p.Dirty))
	put("constants_digest", p.Digest)
	put("bank_up_to", strconv.Itoa(p.BankUpTo))
	for _, s := range p.Seasons {
		put("season", s)
	}
	for _, g := range p.StartGWs {
		put("start_gw", strconv.Itoa(g))
	}
	// One row per arm, in declaration order, so the reader can name which arms
	// are missing rather than only counting them.
	for i, a := range p.DeclaredArms {
		put("declared_arm", fmt.Sprintf("%d\t%s", i, a))
	}
	for _, c := range p.Constants {
		put("constant", c.Path+"\t"+c.Value)
	}
	for _, c := range p.Env {
		put("env", c.Path+"\t"+c.Value)
	}
	if p.WatchedDigest != "" {
		put("watched_digest", p.WatchedDigest)
	}
	for _, c := range p.WatchedPaths {
		put("watched_path", c.Path+"\t"+c.Value)
	}
	w.Flush()
	return b.Bytes()
}

// writeProvenanceTo writes one sweep's provenance block to w in exactly one
// Write call, so a concurrent writer sharing the same underlying file cannot
// interleave with it mid-row.
//
// A real block is 16-25 KB (measured across every banked
// stats/cells/**/*.provenance.csv), 4-7x bufio's 4096-byte default buffer — so a
// csv.Writer built directly over the file, as this used to be, already issued
// several independent write(2) calls per sweep. os.File.Write does not loop
// internally; it calls write(2) once and reports a short write as an error
// rather than retrying, so one call here is one syscall.
func writeProvenanceTo(w io.Writer, p Provenance, withHeader bool) error {
	blk := provenanceBlock(p, withHeader)
	n, err := w.Write(blk)
	if err != nil {
		return err
	}
	if n != len(blk) {
		return io.ErrShortWrite
	}
	return nil
}

// WriteProvenance appends one sweep's provenance to the sidecar file.
//
// Append rather than truncate, for the same reason the cells file appends:
// several sweeps run in one session and losing the earlier ones is the failure
// mode. Written before the first cell rather than after the last, so a killed
// sweep still leaves its declaration behind — which is the entire point, since a
// killed sweep is exactly when the declaration is needed.
func WriteProvenance(path string, p Provenance) error {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	return writeProvenanceTo(f, p, st.Size() == 0)
}

// ReadProvenance reads every sweep's provenance from a sidecar file, keyed by
// (sweep, run_id) exactly as the cells file is.
func ReadProvenance(path string) (map[string]Provenance, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	if _, err := r.Read(); err != nil { // header
		if err == io.EOF {
			return map[string]Provenance{}, nil
		}
		return nil, err
	}
	out := map[string]Provenance{}
	// Arms are collected with their declared index so the order survives a file
	// whose rows got interleaved by two concurrent sweeps.
	type armRef struct {
		idx  int
		name string
	}
	arms := map[string][]armRef{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 4 {
			continue
		}
		// Two concurrent sweeps can both see Size()==0 and both emit a header
		// row. A duplicate parses fine on its own — key "sweep\x00run_id"
		// matches no case below — but without this skip it still creates a
		// phantom out["sweep\x00run_id"] entry.
		if rec[0] == "sweep" && rec[2] == "key" {
			continue
		}
		key := rec[0] + "\x00" + rec[1]
		p := out[key]
		p.Sweep, p.RunID = rec[0], rec[1]
		k, v := rec[2], rec[3]
		name, rest, _ := strings.Cut(v, "\t")
		switch k {
		case "commit":
			p.Commit = v
		case "dirty":
			p.Dirty = v == "true"
		case "constants_digest":
			p.Digest = v
		case "bank_up_to":
			p.BankUpTo, _ = strconv.Atoi(v)
		case "season":
			p.Seasons = append(p.Seasons, v)
		case "start_gw":
			if n, err := strconv.Atoi(v); err == nil {
				p.StartGWs = append(p.StartGWs, n)
			}
		case "declared_arm":
			i, _ := strconv.Atoi(name)
			arms[key] = append(arms[key], armRef{idx: i, name: rest})
		case "constant":
			p.Constants = append(p.Constants, Constant{Path: name, Value: rest})
		case "env":
			p.Env = append(p.Env, Constant{Path: name, Value: rest})
		case "watched_digest":
			p.WatchedDigest = v
		case "watched_path":
			p.WatchedPaths = append(p.WatchedPaths, Constant{Path: name, Value: rest})
		}
		out[key] = p
	}
	for key, list := range arms {
		sort.Slice(list, func(i, j int) bool { return list[i].idx < list[j].idx })
		p := out[key]
		for _, a := range list {
			p.DeclaredArms = append(p.DeclaredArms, a.name)
		}
		out[key] = p
	}
	return out, nil
}

// GitState reports HEAD and whether the tree was clean.
//
// A dirty tree is recorded rather than refused. Measurements get taken mid-change
// constantly and refusing would only mean they got taken with no stamp at all;
// what matters is that "commit abc1234, tree dirty" cannot later be mistaken for
// "commit abc1234".
func GitState(dir string) (sha string, dirty bool) {
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	sha = run("rev-parse", "HEAD")
	if sha == "" {
		return "unknown", false
	}
	return sha, run("status", "--porcelain") != ""
}
