// A per-observation CSV dump, for diagnostics whose inference must not be done
// on binned means.
//
// The project's standing division is "Go for the engine, R for the inference,
// CSV as the contract", and the aggregate sinks (modelSink, cellSink) implement
// the aggregate half of it. Nothing implemented the other half, so a diagnostic
// that wanted a regression had to bin its rows and fit the bins in Go — which is
// how a clean-sheet family verdict came to rest on six bucket means, an
// estimator biased toward exactly the shape it was used to detect.
//
// This is deliberately not a third sink: it has no schema, no run id and no
// provenance, because it is scratch for one question rather than a banked
// figure. A number that survives long enough to be quoted belongs in the model
// CSV, which is versioned and diffed.
package backtest

import (
	"encoding/csv"
	"os"
)

// rowDump writes one CSV row per observation to the path in an env var. A nil
// *rowDump is usable and does nothing, exactly as the other sinks are, so the
// caller's write calls stay unconditional and the env var is checked once.
type rowDump struct {
	f      *os.File
	w      *csv.Writer
	fatalf func(string, ...any)
}

// newRowDump opens path and writes header. An unset path returns nil, which is
// the normal case: the dump exists for a question someone is actively asking.
//
// ⚠️ Every failure here is FATAL, which is the opposite of how the aggregate
// sinks behave, and deliberately so. Setting the env var *is* someone asking for
// these rows, and the consumer is a separate R process that cannot tell a
// missing file from a stale one. A swallowed `os.Create` error leaves the
// PREVIOUS run's dump on disk, the test still passes, `t.Logf` is invisible
// without `-v`, and R then fits the wrong population and reports it as the right
// one. A swallowed write or flush truncates it, which is the repo's own recorded
// hazard: "a killed sweep leaves a partial cells file that reads downstream as a
// complete sweep with fewer arms".
func newRowDump(path string, fatalf func(string, ...any), header ...string) *rowDump {
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		fatalf("row dump requested but not writable: %v", err)
		return nil
	}
	d := &rowDump{f: f, w: csv.NewWriter(f), fatalf: fatalf}
	if err := d.w.Write(header); err != nil {
		fatalf("row dump header not written: %v", err)
		return nil
	}
	return d
}

func (d *rowDump) write(fields ...string) {
	if d == nil {
		return
	}
	if err := d.w.Write(fields); err != nil {
		d.fatalf("row dump write failed: %v", err)
	}
}

func (d *rowDump) close() {
	if d == nil {
		return
	}
	d.w.Flush()
	// Flush reports through Error(), not a return value, so a full disk or a
	// short write is silent unless it is asked for by name.
	if err := d.w.Error(); err != nil {
		d.fatalf("row dump flush failed, so the file is truncated: %v", err)
	}
	if err := d.f.Close(); err != nil {
		d.fatalf("row dump close failed: %v", err)
	}
}
