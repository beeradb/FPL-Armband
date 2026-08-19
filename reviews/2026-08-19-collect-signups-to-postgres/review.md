# Review — collecting the landing page's signups, into Postgres

**Range reviewed:** `4691fae..47bebed` on `collect-signups-to-postgres`, one commit, plus a
merge of `origin/main` (`99ae9f2`) taken after the review and re-tested.

**What it is.** `POST /gate` validated an email address, set a cookie, redirected, and threw
the address away. It now records it. `internal/signup` is a one-method `Store` and a pgx
implementation; the landing page posts to an absolute `https://fplarmband.com/gate` so the
copy served by a local `armband serve` captures into the same list; `/gate` answers 204
rather than a 303 and refuses an unexpected `Origin`.

The deployment side lives in the private deployment repo and is **not** in this range: a
Postgres Deployment on node-scoped hostPath, a least-privileged role created at initdb, a
nightly `pg_dump`, a second upstream pool for `/gate`, and the host directories at 0700.
Reviewed in the same sitting, recorded here, applied nowhere yet.

**Not measurement work.** Nothing in `internal/analysis` moved, no constant was swept, no
figure is quoted. No `HOLD` or `POLICY` number is claimed or implied.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| `fpl-security-review` | yes | a public write path, a new store of personal data, a CORS exception, a database credential, and a secret the cluster did not previously hold |
| `fpl-code-review` | yes | the change's central risk is the silent no-op — the route's whole history is a form that reported success and sent nothing |
| `fpl-docs-review` | yes | `docs/architecture.md` and `AGENTS.md` changed, and the deployment README carries claims about the gate that this falsified |
| `fpl-stats-review` | no | nothing under `internal/analysis` or `internal/backtest` moved, and no measurement was made or quoted. There is no quantity here with a standard error |
| `fpl-findings-audit` | no | no verdict in `AGENTS.md` was added, moved or leaned on. The one sentence edited there is a security-surface statement, not a finding |
| `fpl-run-review` | no | no live run, no config written, no transfer recommended |

**A reviewer's report is a set of proposals.** Every finding below was reproduced before it
was acted on, and the ones declined are recorded with the reason rather than dropped.

## Findings, ranked by how misleading the state was

### 1. The no-store branch re-shipped the exact bug the change removes — APPLIED

`s.signups == nil` skipped the write and fell through to the cookie and 204. `landing.js`
reads 204 as "recorded" and navigates to `/app`, so the reader was told it worked and
nothing was written.

The comment justified the branch as "the local case", and the reviewer traced that the local
page posts to the live site and **never reaches the local handler at all** — so the branch's
only reachable instance was the one the comment did not name: a deployed pod with
`ARMBAND_SIGNUPS_DSN` misspelled or empty. Every submission 204s, the table stays empty,
nothing fails. `TestTheGateStillAnswersWithNoStore` actively pinned that as correct.

Now a 503, and the test is inverted: `TestTheGateRefusesWhenNoStoreIsConfigured` asserts the
status **and** that no gate cookie is set.

### 2. The CORS check withheld the receipt, not the effect — APPLIED

`allowLoopbackOrigin` declined to echo `Access-Control-Allow-Origin` for a foreign origin
and then did the insert anyway. CORS never stops a simple cross-origin POST from being sent,
and the fetch is deliberately built as a simple request (form encoding, no preflight), so
any page on the internet could have had every one of its visitors' browsers insert a row and
simply not be told so. The doc comment said such an origin was "a thing to refuse"; the code
did not refuse it.

Renamed `allowGateOrigin`, now returns a decision, and the handler answers 403 before the
write. The test asserts the **write did not happen**, not merely that a header was absent.

### 3. No upper bound on a stored address — APPLIED

`mail.ParseAddress` enforces no length limit. Measured by the reviewer: a 4000-character
address parses, survives the re-parse guard, and is stored twice — `email` and `email_key` —
on a hostPath the deployment shares with the FPL archive, the proxy cache and Traefik's
`acme.json`. Every distinct string is a fresh key, so nothing dedupes it. A filled disk there
takes certificate renewal down with the list.

Bounded at RFC 5321's 254 octets overall and 64 for the local part, in `Clean` **and** as a
`CHECK` in the schema — the Google sign-in flow will reach `Add` through a second call site,
and a bound enforced in one validator is one the next caller can forget.

### 4. `Clean` mangled a quoted local part while claiming to protect against exactly that — APPLIED

The comment said taking `parsed.Address` is what keeps a display name "and a comma" out of
the list. Reproduced: `"a,b"@example.com` parses, and `parsed.Address` returns the
**unquoted** `a,b@example.com` — a different address, not a legal addr-spec, and two
recipients to anything that builds a mail header from it. Same for an embedded space and an
embedded `@`.

`Clean` now re-parses its own output and refuses anything that does not survive unchanged.
Refused rather than repaired: such addresses are vanishingly rare, the person can supply
another, and storing a mangled address silently is worse than declining it.
`TestCleanRefusesAnAddressItWouldHaveToMangle` covers all three shapes.

### 5. The mirror pin was a containment check, not a use check — APPLIED

`TestTheSignupOriginIsSpelledOnceInEffect` asserted only that the URL **appeared** in
`landing.js`. It would have passed with the constant left assigned and dead beside a fetch
reverted to a relative `/gate` — the precise regression the absolute URL exists to prevent.
It also spelled `/gate` as a literal rather than using `routeGate`, so renaming the route
would have left the test green and every submission 404ing.

Now built from `signupOrigin + routeGate`, and it additionally asserts `fetch(GATE,` and that
the file makes exactly one `fetch` call.

### 6. The nightly backup masked a failed `pg_dump`, then deleted the good ones — APPLIED

`pg_dump | gzip > out` under `set -e`: the shell reads the **last** command's status, so a
failed dump left gzip's zero, wrote a ~20-byte valid gzip under today's date, and let `find`
prune the real dumps — with the job reporting success. Thirty nights of that and the only
copy of the one dataset that cannot be refetched is gone. The comment asserted the opposite
("`set -e` matters"), and `set -o pipefail` is unavailable because the image's `/bin/sh` is
dash.

Now `pg_dump --compress=gzip:9 -f` to a temp name, an atomic rename, and the prune only
after. No pipeline, so the status that matters is the one `set -e` sees.

### 7. The application connected to Postgres as the bootstrap superuser — APPLIED

`POSTGRES_USER: armband` creates the role via `initdb --username`, i.e. as cluster
superuser, and the app was given it. Theoretical today — every value in `Add` is a bind
parameter and the only other SQL is two compile-time constants — but it holds `DROP DATABASE`
and `COPY … TO PROGRAM` for a job needing INSERT and UPDATE on one table, and it converts any
future SQL defect at the announced second call site from "leak the list" into "own the pod".

`postgres` is now the superuser and stays in the database container; an initdb script creates
an ordinary `armband` LOGIN role owning the database and schema, and the app and the backup
job both connect as it. Two secret keys instead of one.

### 8. `/gate` held one of only two upstream connections for the length of a write — APPLIED

The sidecar's `max_conns=2` bounds pressure on the optimiser's mutex. The gate does not take
that mutex, but it now waits on Postgres rather than returning instantly, so two slow signups
could starve every uncached request behind them. Given its own upstream block.

### 9. "The only personal data this project holds" was false — APPLIED

Both `AGENTS.md` and `docs/architecture.md` said it. Counterexamples the reviewer checked
from the tree: `data/captures/` holds `bootstrap-static` payloads containing players' full
names, `config.json`'s `rest_players` names real footballers, and `internal/fpl` deserialises
the FPL account holder's own name. Reworded to what is true and load-bearing — the only
personal data this project **collects**, as against the published payloads it archives.

### 10. Four accuracy defects in the deployment README, three pre-existing — APPLIED

The new bullets sat next to them and made them visible: "Every GET re-runs the optimiser"
(the sidecar's own comment says `/` and `/app` no longer take the mutex); "the sidecar
refuses non-GET" (there is a `/gate` exception twenty lines later); the unbacked-state list
omitting the new database; and the layout row implying the `signups-secret` **script** is
gitignored when what is ignored is its output. Also: the retention window bounds the failure
the backup bullet claims to cover, now stated as "noticed within 30 days".

### 11. One falsified comment my own grep missed — APPLIED

`internal/webui/inline_test.go` still said the gate "sets its cookie and redirects".

## Declined, with reasons

- **A scripted client that omits `Origin` is still accepted** (security #2 residue). Nothing
  at that layer can distinguish it from a same-origin form post or `curl`, and anything that
  can omit a header can forge one. Refusing an absent `Origin` would break real clients and
  buy nothing. **The real answer to "is this a person who asked to be contacted" is a
  confirmation mail, which this change does not attempt.** Recorded as owed.
- **`origin == signupOrigin` is an exact match**, so adding `www.` or a staging hostname
  would 403 the gate there. Only `Host(`fplarmband.com`)` is routed today, so it is correct
  now. Declined as speculative; noted because the failure mode is a successful write reported
  to the reader as a failure.
- **`http://[::1]` with no port is not matched.** A functional gap for a local server on port
  80 only, not a security one.
- **The 405 path returns before the origin check**, so a cross-origin caller sees a network
  error rather than 405. Not on any reachable path.
- **No NetworkPolicy, and `sslmode=disable`.** Single node, both pods on one box, connection
  never touches a network. The manifest already flags that this dies at a second node.
- **`landing.html` still carries "✓ Check your inbox — invite is on the way."** The block is
  hidden and only ever shown carrying a failure message, so the string is unreachable as
  written — but it is a promise nothing sends. Deferred rather than declined: changing it
  invalidates nine committed visual-regression goldens, and that is worth doing with the copy
  decision rather than inside this change.

## Asserted, not verified

- **The manifests have never been applied, and `kubectl kustomize` has not been run** — there
  is no kubectl or kustomize on this machine. In particular, whether kustomize rewrites
  `secretKeyRef.name` inside the CronJob's pod template to the hash-suffixed Secret name is
  **unchecked**; if it does not, the backup job fails at container creation, which pairs
  badly with finding 6. Run it before applying.
- **uid 999 on the deployment node** is assumed to be unclaimed. Container uid 999 is host
  uid 999 under k3s, so a system account there would own the personal-data directory.
- **CI never executes the SQL.** `go test ./...` has no database, so the schema, the advisory
  lock and the `ON CONFLICT` branch skip. They were run here against a real Postgres 17,
  including eight concurrent `Open` calls against a dropped table — but that is one machine on
  one afternoon, and the skip messages say so loudly rather than reading as passes.
