package signup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCleanTakesTheAddressAndNotTheDecoration pins what reaches the list.
//
// mail.ParseAddress accepts a display name, so the parsed Address is what must be stored:
// keeping the raw string would put a name nobody asked for, and a comma, into a column
// something will eventually build a mail header from.
func TestCleanTakesTheAddressAndNotTheDecoration(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"someone@example.com", "someone@example.com"},
		{"  someone@example.com  ", "someone@example.com"},
		{"Someone <someone@example.com>", "someone@example.com"},
		// Case is KEPT. The local part is case-sensitive by the spec, so the thing
		// that sends mail should use what it was given; Key is where case stops
		// mattering, and only for deciding uniqueness.
		{"Someone@Example.COM", "Someone@Example.COM"},
		{"a+tag@example.co.uk", "a+tag@example.co.uk"},
	} {
		got, err := Clean(tc.in)
		if err != nil {
			t.Errorf("Clean(%q) refused a real address: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestCleanRefusesWhatIsNotAnAddress pins the shape check, and the sentinel the handler
// tells a bad address apart from a failed write by.
func TestCleanRefusesWhatIsNotAnAddress(t *testing.T) {
	for _, in := range []string{"", "   ", "not an address", "@example.com", "someone@"} {
		if _, err := Clean(in); !errors.Is(err, ErrNotAnAddress) {
			t.Errorf("Clean(%q) returned %v, want ErrNotAnAddress", in, err)
		}
	}
}

// TestCleanRefusesAnAddressItWouldHaveToMangle pins the round-trip check.
//
// These are LEGAL addresses, and mail.ParseAddress accepts every one of them — then
// returns an Address with the quoting stripped, which is a different address, is not a
// legal addr-spec, and reads as two recipients once anything builds a mail header from it.
// Storing that quietly would corrupt the row on the way in, so the gate declines instead.
//
// Without the second parse in Clean, every case here returns a mangled string and no
// error, which is why this test exists rather than a comment saying it was considered.
func TestCleanRefusesAnAddressItWouldHaveToMangle(t *testing.T) {
	for _, in := range []string{
		`"a,b"@example.com`,
		`"a b"@example.com`,
		`"a@b"@example.com`,
	} {
		got, err := Clean(in)
		if !errors.Is(err, ErrNotAnAddress) {
			t.Errorf("Clean(%q) = %q, %v — want ErrNotAnAddress, because the parsed "+
				"form does not survive being parsed again", in, got, err)
		}
	}
}

// TestCleanRefusesAnAddressTooLongToBeReal pins the length bound.
//
// mail.ParseAddress enforces none, and /gate is the one write path this server exposes to
// the internet. Without this, a scripted caller stores a fresh multi-kilobyte row per
// request — never deduplicated, because every distinct string is a new key — on the same
// disk as the FPL archive and Traefik's certificates.
//
// The bounds are RFC 5321's: 254 octets overall, 64 for the local part. Both cases are
// here because a legal 64-octet local part with an enormous domain passes the second
// check and must fail the first.
func TestCleanRefusesAnAddressTooLongToBeReal(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }

	// 254 exactly is legal and must be kept, so the bound is not off by one.
	ok := long(maxLocalPart) + "@" + long(maxAddress-maxLocalPart-1-len(".invalid")) + ".invalid"
	if len(ok) != maxAddress {
		t.Fatalf("the fixture is %d octets, not %d — fix the test, not the code",
			len(ok), maxAddress)
	}
	if _, err := Clean(ok); err != nil {
		t.Errorf("Clean refused a legal %d-octet address: %v", maxAddress, err)
	}

	for name, in := range map[string]string{
		"one octet over the total":  long(maxLocalPart) + "@" + long(maxAddress-maxLocalPart) + ".invalid",
		"local part over the bound": long(maxLocalPart+1) + "@example.com",
		"the abuse case":            long(4000) + "@example.com",
		"a vast domain":             "a@" + long(4000) + ".example.com",
	} {
		if got, err := Clean(in); !errors.Is(err, ErrNotAnAddress) {
			t.Errorf("%s: Clean returned %d octets and %v, want ErrNotAnAddress",
				name, len(got), err)
		}
	}
}

// TestKeyDecidesUniquenessCaseInsensitively pins the deduplication rule.
//
// Two spellings of one mailbox must collide, because the list is a list of people rather
// than of routing tokens, and mailing the same person twice is the failure this prevents.
func TestKeyDecidesUniquenessCaseInsensitively(t *testing.T) {
	if Key("Someone@Example.COM") != Key("someone@example.com") {
		t.Error("two spellings of one address produce different keys, so the same " +
			"person can enter the list twice")
	}
	if Key("someone@example.com") == Key("someone.else@example.com") {
		t.Error("two different addresses collide on one key")
	}
}

// TestThePostgresStoreRecordsAndDeduplicates is the only test that needs a real database.
//
// It SKIPS without one rather than failing, following this repo's existing convention for
// tests that need something outside the process — the live FPL API tests do the same. Run
// it with a throwaway Postgres:
//
//	docker run --rm -e POSTGRES_PASSWORD=x -p 5432:5432 postgres:17
//	ARMBAND_SIGNUPS_TEST_DSN=postgres://postgres:x@127.0.0.1:5432/postgres go test ./internal/signup/
//
// A skipped test proves nothing, which is why it says so loudly in the skip message: this
// is the ONLY coverage of the SQL, and CI has no database.
func TestThePostgresStoreRecordsAndDeduplicates(t *testing.T) {
	dsn := os.Getenv("ARMBAND_SIGNUPS_TEST_DSN")
	if dsn == "" {
		t.Skip("ARMBAND_SIGNUPS_TEST_DSN is unset — the SQL in this package is " +
			"therefore UNTESTED in this run, including the schema and the " +
			"ON CONFLICT branch that upgrades a typed address to a verified one")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer store.Close()

	// A table per run, so a developer's database does not accumulate rows and two runs
	// cannot see each other's.
	if _, err := store.pool.Exec(ctx, "DELETE FROM signups WHERE email_key LIKE $1",
		"%@signup-test.invalid"); err != nil {
		t.Fatalf("clearing prior test rows: %v", err)
	}

	const addr = "Someone@Signup-Test.invalid"
	if err := store.Add(ctx, Record{Email: addr, Source: SourceForm}); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	// The same person again, spelled differently. This must not error and must not
	// produce a second row.
	if err := store.Add(ctx, Record{Email: "someone@signup-test.invalid",
		Source: SourceForm}); err != nil {
		t.Fatalf("repeat Add: %v", err)
	}

	var rows int
	var email, source string
	var verified bool
	if err := store.pool.QueryRow(ctx,
		`SELECT count(*) OVER (), email, source, verified FROM signups
		 WHERE email_key = $1`, Key(addr)).Scan(&rows, &email, &source, &verified); err != nil {
		t.Fatalf("reading the row back: %v", err)
	}
	if rows != 1 {
		t.Errorf("the same address produced %d rows, want 1", rows)
	}
	if email != addr {
		t.Errorf("stored email is %q, want the first spelling %q", email, addr)
	}
	if verified {
		t.Error("a typed address came back verified")
	}

	// Now the upgrade path: the same person signs in with Google. The row must become
	// verified and must say so through the source column, because the evidence behind
	// the address changed from asserted to proved.
	if err := store.Add(ctx, Record{Email: "someone@signup-test.invalid",
		Source: SourceGoogle, Verified: true}); err != nil {
		t.Fatalf("verified Add: %v", err)
	}
	if err := store.pool.QueryRow(ctx,
		`SELECT count(*) OVER (), source, verified FROM signups
		 WHERE email_key = $1`, Key(addr)).Scan(&rows, &source, &verified); err != nil {
		t.Fatalf("reading the upgraded row: %v", err)
	}
	if rows != 1 {
		t.Errorf("the verified submission produced %d rows, want 1", rows)
	}
	if !verified || source != string(SourceGoogle) {
		t.Errorf("after a verified submission the row is source=%q verified=%v, "+
			"want %q and true", source, verified, SourceGoogle)
	}

	// And the reverse must NOT happen: a typed address cannot un-verify a row an
	// identity provider stood behind.
	if err := store.Add(ctx, Record{Email: addr, Source: SourceForm}); err != nil {
		t.Fatalf("post-verification Add: %v", err)
	}
	if err := store.pool.QueryRow(ctx,
		`SELECT source, verified FROM signups WHERE email_key = $1`,
		Key(addr)).Scan(&source, &verified); err != nil {
		t.Fatalf("reading the row after a typed repeat: %v", err)
	}
	if !verified || source != string(SourceGoogle) {
		t.Errorf("a typed repeat downgraded a verified row to source=%q verified=%v",
			source, verified)
	}
}

// TestConcurrentStartupsAgreeOnTheSchema is the multi-pod test.
//
// Concurrent CREATE TABLE IF NOT EXISTS is NOT safe in Postgres: the existence check and
// the create race, and the loser fails on a duplicate key in a system catalogue rather
// than discovering the table it asked about. That is why applySchema takes an advisory
// lock, and this is the test that the lock is doing the job — without it this fails only
// sometimes, which is the worst way for it to fail.
//
// It drops the table first, because the race only exists on the create.
func TestConcurrentStartupsAgreeOnTheSchema(t *testing.T) {
	dsn := os.Getenv("ARMBAND_SIGNUPS_TEST_DSN")
	if dsn == "" {
		t.Skip("ARMBAND_SIGNUPS_TEST_DSN is unset — the concurrent-startup path that " +
			"multiple pods depend on is therefore UNTESTED in this run")
	}
	// ⚠️ This test DROPS THE TABLE, because the race it exercises is on the create. That
	// makes the DSN it is pointed at load-bearing in a way no other test's is, and
	// ARMBAND_SIGNUPS_TEST_DSN is one token away from the production ARMBAND_SIGNUPS_DSN
	// — a name somebody debugging the live database has just been looking at. Exporting
	// the wrong one and running `go test ./...` would drop the whole signup list, and
	// with no read side and no restore path in this package, nothing here would notice.
	//
	// So: loopback only. The deployed database is reached at a cluster-internal name, so
	// this refuses the exact slip it is aimed at.
	if !isLoopbackDSN(t, dsn) {
		t.Fatalf("ARMBAND_SIGNUPS_TEST_DSN points at a non-loopback host, and this "+
			"test DROPS the signups table. Refusing to run it against %q — point "+
			"it at a throwaway database on 127.0.0.1.", redactDSN(dsn))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	setup, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	if _, err := setup.pool.Exec(ctx, "DROP TABLE IF EXISTS signups"); err != nil {
		setup.Close()
		t.Fatalf("dropping the table: %v", err)
	}
	setup.Close()

	// Eight at once, well above the replica count anyone would run, because a race
	// this narrow needs pressure to show up at all.
	const pods = 8
	errs := make(chan error, pods)
	start := make(chan struct{})
	for range pods {
		go func() {
			// A shared start signal, so the goroutines contend rather than
			// arriving in sequence and each finding the table already there.
			<-start
			store, err := Open(ctx, dsn)
			if err == nil {
				store.Close()
			}
			errs <- err
		}()
	}
	close(start)
	for range pods {
		if err := <-errs; err != nil {
			t.Errorf("a concurrent startup failed, so a rollout with more than one "+
				"replica would CrashLoop: %v", err)
		}
	}
}

// isLoopbackDSN reports whether a connection string names a host on this machine.
//
// It parses with pgx's own parser rather than net/url, so it agrees with what the store
// will actually connect to — a DSN may be key/value form, and the host may come from
// PGHOST rather than from the string at all.
func isLoopbackDSN(t *testing.T, dsn string) bool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// Unparseable means Open would refuse it too. Answering false keeps the
		// destructive path shut on anything not positively known to be local.
		return false
	}
	host := cfg.ConnConfig.Host
	// A unix socket path is by definition on this machine.
	if strings.HasPrefix(host, "/") {
		return true
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// redactDSN keeps a password out of a test failure message, which lands in CI output.
func redactDSN(dsn string) string {
	if cfg, err := pgxpool.ParseConfig(dsn); err == nil {
		return fmt.Sprintf("%s:%d/%s", cfg.ConnConfig.Host, cfg.ConnConfig.Port,
			cfg.ConnConfig.Database)
	}
	return "an unparseable connection string"
}

// TestOpenRefusesAnUnreachableDatabase pins the direction startup fails in.
//
// A configured database that cannot be reached is a deployment fault, and starting anyway
// would mean dropping submissions while answering success. The pod restarting is the
// correct outcome; the sidecar's cache keeps serving readers meanwhile.
//
// Bounded well under the store's own 30s retry window by a context deadline, so this test
// costs a second rather than half a minute.
func TestOpenRefusesAnUnreachableDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Port 1 on loopback: nothing listens there, and the connection is refused rather
	// than hanging, so this does not depend on a network timeout.
	if _, err := Open(ctx, "postgres://x:y@127.0.0.1:1/nothing"); err == nil {
		t.Error("Open accepted a database it could not reach, so a misconfigured " +
			"deployment would start and silently drop every signup")
	}
}
