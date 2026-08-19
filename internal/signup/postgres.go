package signup

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is the store the deployment runs.
//
// # Why Postgres and not a file, and not SQLite
//
// A file was the right size for the data and the wrong size for the plan: more than one
// pod will write this list, and two processes appending to one node-scoped file is a
// corrupted list rather than a slow one.
//
// SQLite would have been the usual answer for a single node and is not available cheaply
// here. The Dockerfile's guarantee is a fully static distroless binary — CGO_ENABLED=0,
// nothing in the tree importing C — which rules out the cgo driver outright, and the pure
// Go one is a transpiled SQLite tree in a module with two direct dependencies. pgx speaks
// the wire protocol in pure Go, so the static build is unaffected. The constraint that
// usually argues for SQLite argues against it here.
type Postgres struct {
	pool *pgxpool.Pool
}

// schema is the whole thing. One table, created on the way up.
//
// # Why DDL at startup rather than a migration tool
//
// One table with no history does not need a migration runner, a versions table, or a
// second binary in the image. When the second table arrives this should become a numbered
// migration set — the point at which "IF NOT EXISTS" stops being honest is the point at
// which a column needs changing, and that is a different problem with a different answer.
//
// email_key exists as a stored column rather than an expression index because ON CONFLICT
// against an expression index is a spelling nobody remembers, and because the normalising
// rule then lives in exactly one place: Key, in Go, tested there.
// The CHECKs are not belt and braces over Clean's bounds — they are the bounds that survive
// a second call site. The Google sign-in flow will reach Add without going through the
// landing page's form, and a length rule enforced only in one validator is one the next
// caller can forget. This is the row that cannot be written, whoever asks.
//
// BOTH of RFC 5321's numbers are here, 254 overall and 64 for the local part, because a
// schema carrying only the first would bound that second call site at 254 and silently not
// at 64 — which is the half a validator is most likely to be the only holder of. The local
// part is taken off email_key rather than email: they have the same length, and email_key
// is the column with the index on it.
const schema = `
CREATE TABLE IF NOT EXISTS signups (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email        text        NOT NULL CHECK (length(email) <= 254),
    email_key    text        NOT NULL UNIQUE
                             CHECK (length(email_key) <= 254)
                             CHECK (length(split_part(email_key, '@', 1)) <= 64),
    source       text        NOT NULL CHECK (length(source) <= 32),
    verified     boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);`

// migrationLock is an arbitrary fixed key for the advisory lock the schema runs under.
//
// It is needed because more than one pod starts at once during a rollout, and concurrent
// CREATE TABLE IF NOT EXISTS is NOT safe in Postgres: the existence check and the create
// race, and the loser fails on a duplicate key in a system catalogue rather than
// discovering the table it asked about. That failure is rare, looks like a database fault,
// and appears only under exactly the condition this deployment is heading for.
const migrationLock = 0x5F504C5F // "_PL_"

// Open connects and makes sure the table is there.
//
// # Why this fails startup rather than degrading
//
// A configured database that cannot be reached is a deployment fault, and the honest
// answer is to refuse to start: the pod restarts, and the sidecar's node-scoped cache goes
// on serving readers the page while it does, which is what that cache is for. The
// alternative — start anyway and drop submissions — is the silent-failure shape the
// landing page's whole gate was rewritten to eliminate.
//
// The retry exists because "unreachable" and "not up yet" look identical from here, and a
// rollout that restarts the database alongside the app would otherwise be a coin flip.
func Open(ctx context.Context, dsn string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("reading the database URL: %w", err)
	}
	// Small on purpose. Inbound concurrency is already bounded hard upstream — the
	// sidecar caps the app at two connections and rate-limits the gate per IP — so a
	// larger pool cannot be used, and idle Postgres connections are not free.
	cfg.MaxConns = 4
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to the database: %w", err)
	}

	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

// migrate applies the schema, waiting for the database to accept connections at all.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	const (
		wait = 30 * time.Second
		gap  = time.Second
	)
	deadline := time.Now().Add(wait)
	for attempt := 1; ; attempt++ {
		err := applySchema(ctx, pool)
		if err == nil {
			return nil
		}
		// The context being done is the caller giving up — a Ctrl-C during startup —
		// and retrying against it would spin until the deadline saying nothing useful.
		if ctx.Err() != nil {
			return fmt.Errorf("preparing the signups table: %w", ctx.Err())
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("preparing the signups table, after %s and %d attempts: %w",
				wait, attempt, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("preparing the signups table: %w", ctx.Err())
		case <-time.After(gap):
		}
	}
}

// applySchema takes the advisory lock and creates the table.
func applySchema(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback on the error paths. A committed transaction makes this a no-op, so it is
	// the whole cleanup rather than half of it.
	defer func() { _ = tx.Rollback(ctx) }()

	// The transaction-scoped lock releases on commit or rollback, including the rollback
	// a crashed pod's connection produces. The session-scoped spelling would strand the
	// lock and wedge every other pod's startup.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(migrationLock)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, schema); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Add records one submission.
//
// # What a repeat submission does
//
// Nothing visible, which is the point: a person who signs up twice is not an error to
// report to them. The row's last_seen_at moves, so a repeat is still evidence.
//
// The one case where a repeat CHANGES anything is an unverified row meeting a verified
// submission — somebody typed their address and later signed in with Google. Then the
// verified evidence replaces the asserted evidence, and the source column follows it, so
// the row says how the address came to be trusted rather than how it was first seen. The
// reverse never happens: a typed address cannot un-verify a row Google stood behind.
func (p *Postgres) Add(ctx context.Context, r Record) error {
	const q = `
INSERT INTO signups (email, email_key, source, verified)
VALUES ($1, $2, $3, $4)
ON CONFLICT (email_key) DO UPDATE SET
    last_seen_at = now(),
    verified     = signups.verified OR EXCLUDED.verified,
    source       = CASE WHEN EXCLUDED.verified AND NOT signups.verified
                        THEN EXCLUDED.source ELSE signups.source END,
    email        = CASE WHEN EXCLUDED.verified AND NOT signups.verified
                        THEN EXCLUDED.email ELSE signups.email END`

	if _, err := p.pool.Exec(ctx, q, r.Email, Key(r.Email), string(r.Source), r.Verified); err != nil {
		// The address is deliberately absent from this message. An error string reaches
		// the log, and "we only wrote it to stderr" is still having written it down.
		return fmt.Errorf("recording a signup: %w", err)
	}
	return nil
}

// Close releases the pool.
func (p *Postgres) Close() { p.pool.Close() }
