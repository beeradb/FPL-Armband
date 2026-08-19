# Review — publishing armband as a container image

**Range reviewed:** `origin/main..build-the-serve-container-image`
(`b56a11e`, `5d97358`, `bc4cceb`, plus the fixes recorded below).

**What it is.** Three new files, nothing modified: a `Dockerfile` (two-stage,
distroless, non-root), a `.dockerignore`, and `.github/workflows/image.yml`,
which builds `linux/amd64` and pushes to GHCR with provenance and an SBOM.

The image is deliberately **generic** — the binary and nothing else. No
`config.json`, no cached FPL data, no overrides. Whatever runs it supplies
config as a mount and data as a volume, so neither the image nor this
repository carries any detail of a particular deployment.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| `fpl-security-review` | yes | the workflow holds `packages: write`, `id-token: write` and a registry token, and `.dockerignore` decides what can enter a published artifact |
| `fpl-code-review` | yes | the Dockerfile encodes assumptions about `config.Load`, `cache_dir` and flag ordering that only hold if they match the binary |
| `fpl-stats-review` | skipped | touches no scoring, no estimator, no cell |
| `fpl-findings-audit` | skipped | touches no recorded verdict; `AGENTS.md` and `docs/` unchanged |
| `fpl-run-review` | skipped | no live run, no config written |
| `fpl-season-maintenance` | skipped | none of the four hand-maintained lists |

Both reviewers' reports were treated as proposals and each was checked before
being applied or declined. Two of their claims were verified independently
against the registry and GitHub APIs rather than taken on trust: the two base
image digests, and the `actions/checkout` tag.

## Applied

1. **Shell injection through the branch name (the serious one).**
   `'${{ github.ref }}'` was interpolated directly into the `meta` step's
   `run:` block. GitHub substitutes before bash parses, a branch name may
   legally contain a single quote, and this workflow builds every branch — so
   naming a branch was enough to execute commands in a job that already had the
   GHCR push token on disk from the login step, plus `packages: write` and an
   OIDC minter in the environment. `ci.yml` states this exact rule and this
   workflow broke it. `REF`, `REPO` and `SHA` now arrive through `env:` and are
   quoted. This was reachable only by someone who can push a branch, but agents
   in this repo create branches, and `merge-gate` already treats a branch name
   as an unreviewed disclosure channel.
2. **Both base images pinned by digest.** `golang:1.26.5-alpine` and
   `distroless/static-debian12:nonroot` were mutable tags, so the image digest
   was not a function of the commit. Resolved independently:
   `golang@sha256:0178a641…`, `static-debian12@sha256:1b7b9f0f…`. The
   determinism comment now claims the **binary** only, which is what
   `-trimpath -buildid=` actually buys.
3. **`concurrency: group` made per-ref.** A single global group cancels *queued*
   runs (`cancel-in-progress: false` protects only the running one), so three
   branches pushed close together would leave the middle one with no image,
   reported as "cancelled". Concurrent sessions are normal here.
4. **`GOARCH` follows `TARGETARCH`.** It was hard-coded, which currently matches
   the single platform but would silently produce a manifest entry labelled
   `arm64` containing an amd64 binary the moment a platform is added.
5. **`USER` moved above `WORKDIR`**, so `/data` is created owned by 65532 on
   every builder rather than root-owned under the classic one.
6. **`cache-to` reduced to `mode=min`.** `mode=max` exports every intermediate
   layer into the repo-wide 10 GB LRU Actions cache, shared with the season
   archive `ci.yml` calls load-bearing; evicting that fails silently.
7. **Two comments corrected.** The `actions/checkout` pin said `v4.1.7`; the SHA
   is `v4.4.0`. The `WORKDIR` rationale is narrowed — see below.

## Declined

- **Adding a default `CMD ["serve"]`.** The finding is real: with no `CMD`, a
  pod whose `args:` were dropped or misspelled exits 0, so it restarts forever
  showing `Completed` with no crash signal. But the proposed fix is worse than
  the defect. `armband serve` with no `-config` has `config.Load` *write* a
  default config, and a default config has `entry_id: 0`, which makes the engine
  assume the standard budget and render a complete, legal-looking fifteen that
  belongs to nobody — the failure `internal/analysis/budget.go` has a comment
  block explaining must never happen. A container that exits 0 doing nothing
  beats one that serves a plausible fiction. Recorded in the Dockerfile so the
  next reader does not "fix" it.
- **Documenting in this repo how the image is reached over a network.**
  `serve` refuses a non-loopback bind and 403s a non-loopback `Host`, so it
  needs something in its own network namespace to rewrite `Host`. That is true
  and worth knowing, but *how a particular deployment does it* is deployment
  detail and does not belong here. The constraint itself is already documented
  where it is enforced, in `cmd/armband/serve.go`.
- **Narrowing `id-token: write`.** Correct in principle, but no federated trust
  exists to narrow against yet. Revisit when one is added; a subject condition
  should then pin `ref:refs/heads/main` rather than a wildcard.

## Could not be checked

- **There is no Docker daemon on this machine.** The image was never built,
  so no layer, digest or file ownership inside it was observed. What *was*
  executed natively: the exact `go build` the Dockerfile runs (18 MB static
  ELF, byte-identical across two runs), and a build from a faithfully
  reproduced build context after applying `.dockerignore` by hand.
- **BuildKit's symlink handling was reasoned, not demonstrated.** It matters
  because `reports` is a symlink out of the repository on developer machines.
  Two independent legs close it without needing that behaviour: `reports` is
  untracked, so CI's checkout has no such path at all, and `.dockerignore`
  excludes it regardless.
- **GitHub repository settings** — who holds push access, branch protection,
  and GHCR package visibility. The severity of the injection finding rests on
  push being restricted, which was assumed rather than verified.
- **`scripts/leakscan` cannot run locally** (it needs `LEAKSCAN_PATTERN`) and
  says so rather than reporting a pass. A manual scan of all three files for
  host, address and domain strings returned nothing; CI runs the real one.

## Judgement

Nothing here changes a scored quantity, an estimator or a recorded verdict, so
no measurement is affected and no detection threshold applies. The risk in this
change was entirely supply-chain and operational, which is where both reviewers
were pointed and where every applied fix landed.
