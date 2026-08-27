package backtest

// Dump the xGC reconstruction's own output, so it can be compared against something.
//
//	DIAG=1 EXP=XGCDUMP FPL_XGCDUMP=/tmp/xgc-2021-22.csv \
//	    go test ./internal/backtest -run '^TestDiagXGCDump$' -v -timeout 20m
//
// # Why this exists
//
// `applyXGCRepair` reconstructs `expected_goals_conceded` for the seasons the archive
// does not publish it for — 2018-19 through 2021-22, and GW1-15 of 2022-23 — through
// the chain
//
//	repaired player xG -> club xG per gameweek -> the opponent's xGA -> prorate by minutes
//
// and its own comment puts the per-match error of that chain, on the input it actually
// runs on, at **16-20%**. That figure is about the chain's INPUT. How much of it
// survives into the OUTPUT has never been measured, because the reconstruction's values
// never leave the replay: they are written into the season in memory and read by the
// scoring path, and nothing dumps them.
//
// So the quantity everyone reasons about is the one quantity nobody can look at. This
// prints it.
//
// # What it prints, and why THIS quantity
//
// One row per (club, gameweek): the CLUB-level reconstructed xGC, taken from
// `clubXGAPerGameweek` — the value before per-player proration.
//
// The club level is the right comparison point and the per-player level is not.
// ⚠️ A player's `expected_goals_conceded` is the club figure PRORATED BY HIS MINUTES —
// on 2024-25 GW1 every Bournemouth player who went 90 reads exactly 1.30 while
// substitutes read fractions — so a per-player dump would confound the reconstruction's
// error with the proration's, and the proration is the one part the repair's own comment
// already admits is approximate.
//
// # This is a READ-ONLY diagnostic
//
// It writes a CSV and changes nothing. It must never be made to write into the archive:
// the reconstruction is point-in-time sensitive, and a dump that fed back into the model
// would be a leak of exactly the kind `TestPointInTimeHidesFutureResults` exists to stop.
//
// # It reports what it CANNOT reconstruct, rather than emitting a short file
//
// A club-gameweek with no fixture, or a season absent from the `xgRepairs` table, is
// counted and named in the summary. A silently short CSV is the failure this repository
// has paid for before: `stats/regenerate_mde.sh` skipped missing inputs without a word
// and produced 11 sweeps instead of 14 while still looking like a valid aggregate.

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
)

func TestDiagXGCDump(t *testing.T) {
	requireDiag(t)
	picker := newBlockPicker()
	defer picker.check(t)
	if !picker.want("XGCDUMP") {
		t.Skip("set EXP=XGCDUMP to run this block")
	}
	out := os.Getenv("FPL_XGCDUMP")
	if out == "" {
		t.Fatal("FPL_XGCDUMP must name the CSV to write. Refusing to guess a path: a " +
			"dump written somewhere the operator did not choose is a file nobody finds " +
			"and everybody later mistrusts.")
	}

	cfg := loadConfig(t)
	seasons := seasonOrder
	if s := os.Getenv("FPL_XGCDUMP_SEASONS"); s != "" {
		seasons = splitAndTrim(s)
	}

	f, err := os.Create(out)
	if err != nil {
		t.Fatalf("creating %s: %v", out, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"season", "club_id", "club_short", "gameweek",
		"reconstructed_xgc", "repaired"}); err != nil {
		t.Fatalf("writing header: %v", err)
	}

	var rows, repaired, skipped int
	for _, name := range seasons {
		s, err := Load(t.Context(), cfg.CacheDir, name)
		if err != nil {
			t.Fatalf("loading %s: %v — the dump refuses to skip a season quietly, "+
				"because a short file looks exactly like a complete one", name, err)
		}
		_, isRepaired := xgRepairs[s.Name]

		short := map[int]string{}
		for _, tm := range s.Teams {
			short[tm.ID] = tm.ShortName
		}

		xga := clubXGAPerGameweek(s, xgcScale)
		keys := make([][2]int, 0, len(xga))
		for k := range xga {
			keys = append(keys, k)
		}
		// Stable order: two runs must produce byte-identical files.
		sort.Slice(keys, func(i, j int) bool {
			if keys[i][0] != keys[j][0] {
				return keys[i][0] < keys[j][0]
			}
			return keys[i][1] < keys[j][1]
		})

		for _, k := range keys {
			club, gw := k[0], k[1]
			nm := short[club]
			if nm == "" {
				// Prior-only seasons carry no teams.csv, so the id cannot be named.
				// Emit the id rather than dropping the row, and say so in the summary.
				nm = "?"
				skipped++
			}
			if err := w.Write([]string{
				s.Name,
				strconv.Itoa(club),
				nm,
				strconv.Itoa(gw),
				strconv.FormatFloat(xga[k], 'f', 6, 64),
				strconv.FormatBool(isRepaired),
			}); err != nil {
				t.Fatalf("writing row: %v", err)
			}
			rows++
			if isRepaired {
				repaired++
			}
		}
		t.Logf("%s: %d club-gameweeks, repaired=%v", s.Name, len(keys), isRepaired)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("flushing %s: %v", out, err)
	}
	fmt.Printf("\nwrote %d rows to %s (%d from repaired seasons, %d club-gameweeks "+
		"whose club could not be named — prior-only seasons carry no teams.csv)\n",
		rows, out, repaired, skipped)
	if rows == 0 {
		t.Fatal("dumped zero rows, which is never a valid result here")
	}
}

// splitAndTrim reads a comma-separated season list, refusing anything empty rather than
// silently narrowing the dump.
func splitAndTrim(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			f := s[start:i]
			for len(f) > 0 && f[0] == ' ' {
				f = f[1:]
			}
			for len(f) > 0 && f[len(f)-1] == ' ' {
				f = f[:len(f)-1]
			}
			if f != "" {
				out = append(out, f)
			}
			start = i + 1
		}
	}
	return out
}
