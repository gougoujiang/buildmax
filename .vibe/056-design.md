# Design 056: Container image infra

## Goal

Provide a multi-stage Dockerfile for the BuildMax binary and a make target that builds the image and loads it into the local kind cluster, so the image is available for in-cluster workloads. Align with existing setup (kind cluster name, docs).

## Modules / Artifacts

| Artifact | Location | Responsibility |
|----------|----------|-----------------|
| Dockerfile.buildmax | Repo root | Build and runtime stages for the `buildmax` binary |
| make script | Repo root (`make`) | New `pub_images` command: docker build + kind load |
| DEVELOPEMENT.md | Repo root | Section on building and loading the BuildMax image |

No new Go packages or internal code; only repo-root files and script changes.

## Dockerfile.buildmax — Structure

- **File**: `Dockerfile.buildmax` at repo root. Build context: repo root (`.`).

**Stage 1 — build**

- **Base image**: `golang:1.26-alpine3.23`.
- **Workdir**: e.g. `/app`.
- **Copy**: Only what’s needed for `go build ./cmd/buildmax`: `go.mod` and `go.sum` first (for better layer caching), then `cmd/` and `internal/`.
- **Build**: `RUN go build -o /buildmax ./cmd/buildmax`. Go’s default `GOOS=linux` when running inside the container produces a Linux binary. Optional `CGO_ENABLED=0` for a static binary.
- **Output**: Single binary at `/buildmax`.

**Stage 2 — runtime**

- **Base image**: `alpine:3.23`.
- **Install** (if needed): `ca-certificates` for HTTPS (LLM, S3, etc.). Optional: add a non-root user and run as that user.
- **Copy**: Only the binary from the build stage: `COPY --from=0 /buildmax /usr/local/bin/buildmax`.
- **Entrypoint**: `ENTRYPOINT ["/usr/local/bin/buildmax"]` so `docker run <image> version` works as `buildmax version`.
- **Default cmd**: Can leave unset or `CMD ["version"]` for convenience; not required for acceptance.

**Platform**

- No `FROM --platform=` in the Dockerfile; let Docker use the default (on Mac ARM64 → linux/arm64). Optional `BUILDMAX_IMAGE_PLATFORM`: the make script will pass `--platform "$BUILDMAX_IMAGE_PLATFORM"` to `docker build` only when the env var is set.

## Make — pub_images

- **Command**: `./make pub_images` (single top-level command, no subcommand).
- **Behavior**:
  1. **Build image**
     - `docker build -f Dockerfile.buildmax -t buildmax:local .`
     - If `BUILDMAX_IMAGE_PLATFORM` is set (e.g. `linux/amd64`), add `--platform "$BUILDMAX_IMAGE_PLATFORM"`.
     - Run from `SCRIPT_DIR` (repo root), same as other make commands.
  2. **Load into kind**
     - Cluster name: use same default as `setup/setup.sh`, i.e. `BUILDMAX_KIND_CLUSTER` if set, else `buildmaxdev` (note: DEVELOPEMENT.md says "dev" but setup.sh uses `buildmaxdev`; use setup.sh as source of truth).
     - `kind load docker-image buildmax:local --name "$CLUSTER_NAME"`.
- **Prerequisites**: Docker and kind must be available; no automatic install. On failure, exit with a clear message (e.g. "docker build failed" or "kind load failed").
- **Usage**: Add to the make script’s `usage()` and case statement: `pub_images  Build BuildMax image and load into kind cluster`.

## Documentation — DEVELOPEMENT.md

- **Section**: Add a subsection under or after "Setup Kind, MinIO and MySQL", e.g. **"BuildMax container image"**.
- **Content**:
  - Dockerfile: `Dockerfile.buildmax` (for the binary; a future `Dockerfile.portal` will be for the Portal).
  - Build: `docker build -f Dockerfile.buildmax -t buildmax:local .`
  - Load into kind: `kind load docker-image buildmax:local --name buildmaxdev` (or mention `BUILDMAX_KIND_CLUSTER`).
  - One-step: `./make pub_images` does both.
  - Optional: if `BUILDMAX_IMAGE_PLATFORM` is set (e.g. `linux/amd64`), the make target uses it for `docker build --platform`.
- **Consistency**: Fix DEVELOPEMENT.md’s cluster name if it says "dev" — either make it "buildmaxdev" or state that the default is from `BUILDMAX_KIND_CLUSTER` (default `buildmaxdev`).

## How it works together

1. Developer runs `./make pub_images` from repo root.
2. Make runs `docker build -f Dockerfile.buildmax ...`; Docker builds the two-stage image (Go build in stage 1, binary copied to Alpine in stage 2), tags as `buildmax:local`.
3. Make runs `kind load docker-image buildmax:local --name buildmaxdev` (or `$BUILDMAX_KIND_CLUSTER`); kind imports the image into the cluster.
4. Any Pod/Job in that cluster can use `image: buildmax:local` and run the binary (e.g. `buildmax server` or `buildmax -p "..."`).
5. Docs in DEVELOPEMENT.md tell users how to build, load, and use the image and the make target.

## Changes for review

| Change | File / location |
|--------|------------------|
| **New** | `Dockerfile.buildmax` at repo root — multi-stage: golang Alpine build, Alpine runtime with `buildmax` and ca-certificates |
| **Edit** | `make` — add `pub_images` to usage; add `cmd_pub_images` (build + kind load, honor `BUILDMAX_IMAGE_PLATFORM` and `BUILDMAX_KIND_CLUSTER`); add `pub_images)` case |
| **Edit** | `DEVELOPEMENT.md` — add "BuildMax container image" section (Dockerfile name, docker build, kind load, `./make pub_images`, optional `BUILDMAX_IMAGE_PLATFORM`); align cluster name with setup.sh (buildmaxdev) |

No changes to Go code, `setup/setup.sh`, or `internal/`.
