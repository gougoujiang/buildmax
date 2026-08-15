# Install

> **Audience:** users · **Status:** current

BuildMax ships three binaries. `buildmax` is the local CLI/TUI and is all you
need to start; `buildmax-server` and `buildmax-worker` are for the team
deployment described in [deploy/](../deploy/overview.md).

Before choosing an install path, check the
[support matrix](support.md) for the current alpha platform and deployment
boundaries.

| Component | Release targets | Distribution |
|---|---|---|
| CLI, server, worker | Linux amd64/arm64, macOS amd64/arm64, Windows amd64 | Release archives |
| Server and worker container | Linux amd64/arm64 | GHCR image |
| Portal | Linux amd64/arm64 | GHCR image, or build from source |
| Desktop | macOS and Windows development builds | Build from source; unsigned |

## Release Archive

Download an archive for your platform from
[Releases](https://github.com/gougoujiang/buildmax/releases). Each one contains
all three binaries plus `config-examples/`.

```bash
tar xzf buildmax_<version>_<os>_<arch>.tar.gz
sha256sum -c checksums.txt          # verify before trusting the binaries
sudo mv buildmax buildmax-server buildmax-worker /usr/local/bin/
buildmax version
```

Each release also publishes an SPDX SBOM for every archive. For releases made
after the repository became public, verify GitHub's build provenance
attestation with:

```bash
gh attestation verify --owner gougoujiang buildmax_<version>_<os>_<arch>.tar.gz
```

Windows archives use `.zip`. Checksums prove the downloaded bytes match the
release; the attestation proves GitHub Actions built those bytes from this
repository.

## Go Toolchain

For the CLI alone:

```bash
go install github.com/gougoujiang/buildmax/cmd/buildmax@latest
```

A binary installed this way reports the module version it was built from —
`buildmax version` prints `0.1.0-alpha`, not `dev`. It carries no commit hash,
because `go install` records no VCS stamp; release archives and `./make build`
report both.

## Container

One image carries all three binaries; a second serves the Portal:

```bash
docker pull ghcr.io/gougoujiang/buildmax:<version>
docker pull ghcr.io/gougoujiang/buildmax-portal:<version>
```

Both are published per release tag and carry the same version. Alpha releases
deliberately do not move `latest`, so name the version you want. Running the
Portal image takes one environment variable — see
[deploy/overview.md](../deploy/overview.md#portal).

To run the whole team stack rather than one image, use the
[Compose quickstart](../deploy/compose.md), which builds from source until a
release publishes these tags.

## From Source

Requires Go (version in `go.mod`), plus Node only if you also want the
frontends:

```bash
git clone https://github.com/gougoujiang/buildmax.git
cd buildmax
./make build          # CLI, server, worker, shared GUI, desktop app → bin/
```

`./make build cli` builds just the CLI. On Windows use `make.bat` with the same
commands. See [CONTRIBUTING.md](../../CONTRIBUTING.md) for the full task list.

## Desktop App

The desktop app is not published as a binary — launching it on macOS requires
code signing and notarization. Build it locally with `./make build`, which
produces it alongside the other binaries.

## Next

Configure a model with `buildmax init` and run your first task:
[quickstart.md](quickstart.md).
