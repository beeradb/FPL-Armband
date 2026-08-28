# Build the armband binary and ship it on a distroless base.
#
# The image is deliberately GENERIC: the binary and nothing else. No config.json,
# no cached FPL data, no overrides. A deployment supplies its own config as a
# mount and its own data as a volume, so this image says nothing about where or
# how it is run.
#
# Both bases are pinned by DIGEST, not tag. golang:1.26.7-alpine is rebuilt on
# Alpine security updates and distroless :nonroot is republished on every CA
# bundle refresh, so a tag would let the shipped runtime change without this
# file recording that it had -- the same rule the workflow applies to actions,
# and the base image is the larger surface of the two.
FROM golang@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468 AS build

# The module declares go 1.26.7. GOTOOLCHAIN=local turns a version mismatch into
# a build error rather than a silent download of a different compiler. (The
# golang image already sets this; restated so the guarantee does not depend on
# the base keeping it.)
#
# ⚠️ THAT COUPLING IS REAL AND IT BITES. Bumping go.mod without moving this digest
# fails the image build with `go.mod requires go >= X (running Y; GOTOOLCHAIN=local)`
# — which is the guarantee working, not a defect. It happened on the 1.26.5 -> 1.26.7
# bump that took seven reachable CVEs to zero: go.mod moved, this digest did not, and
# both the image build and the new Trivy scan went red until it did. **Move them in
# the same commit.**
ENV GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux

# GOARCH follows BuildKit's per-platform TARGETARCH rather than being fixed, so
# adding a platform to the workflow cannot produce a manifest entry labelled
# arm64 that contains an amd64 binary. That failure surfaces at runtime as
# "exec format error" on a node the scheduler thought was compatible.
ARG TARGETARCH
ENV GOARCH=${TARGETARCH}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .

# -trimpath strips the build path and -buildid= drops the last non-deterministic
# field, so two builds of one commit produce an identical BINARY. That is a
# claim about the binary only: the image digest additionally depends on the
# pinned bases above.
RUN go build -trimpath -ldflags='-s -w -buildid=' -o /out/armband ./cmd/armband

# static-debian12 carries the CA bundle the live FPL API needs and nothing else:
# no shell, no package manager. Nothing in the tree imports "C", so CGO stays off
# and the binary is fully static.
FROM gcr.io/distroless/static-debian12@sha256:1b7b9f0f0e0a1d2155f531db587cc48ec26aaf97ab64364225f5bf18a054e66a
COPY --from=build /out/armband /usr/local/bin/armband

# USER before WORKDIR, so /data is created owned by that uid on every builder
# rather than root-owned under the classic one.
USER 65532:65532
WORKDIR /data

# No CMD. armband requires global flags to precede the subcommand, so the whole
# command line belongs to the caller.
#
# Deliberately no default subcommand either: `armband serve` with no -config
# would have config.Load WRITE a default config, and a default config has
# entry_id 0, which makes the engine assume the standard budget and render a
# complete, legal-looking squad that belongs to nobody. A container that exits 0
# doing nothing is a better failure than one that serves a plausible fiction.
ENTRYPOINT ["/usr/local/bin/armband"]
