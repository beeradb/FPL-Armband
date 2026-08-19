# Build the armband binary and ship it on a distroless base.
#
# The image is deliberately GENERIC: the binary and nothing else. No config.json,
# no cached FPL data, no overrides. A deployment supplies its own config as a
# mount and its own data as a volume, so this image says nothing about where or
# how it is run.
FROM golang:1.26.5-alpine AS build

# The module declares go 1.26.5. GOTOOLCHAIN=local turns a version mismatch into
# a build error rather than a silent download of a different compiler.
ENV GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .

# -trimpath strips the build path and -buildid= drops the last non-deterministic
# field, so two builds of one commit produce identical bytes and the digest means
# something.
RUN go build -trimpath -ldflags='-s -w -buildid=' -o /out/armband ./cmd/armband

# static-debian12 carries the CA bundle the live FPL API needs and nothing else:
# no shell, no package manager. Nothing in the tree imports "C", so CGO stays off
# and the binary is fully static.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/armband /usr/local/bin/armband

# cache_dir is relative to the working directory unless a config makes it
# absolute, and config.Load writes a default config when the file is missing --
# so the working directory has to be somewhere both can legitimately land.
WORKDIR /data
USER 65532:65532

# No CMD. The subcommand and its flags belong to whoever runs this, and armband
# requires global flags to precede the subcommand.
ENTRYPOINT ["/usr/local/bin/armband"]
