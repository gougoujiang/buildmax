# Install

> **Audience:** users · **Status:** current

BuildMax ships three binaries. `buildmax` is the local CLI/TUI and is all you
need to start; `buildmax-server` and `buildmax-worker` are for the team
deployment described in [deploy/](../deploy/overview.md).

| Component | Release targets | Distribution |
|---|---|---|
| CLI, server, worker | Linux amd64/arm64, macOS amd64/arm64, Windows amd64 | Release archives |
| Server and worker container | Linux amd64/arm64 | GHCR image |
| Portal | Modern browsers | Build from source |
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

The image carries all three binaries:

```bash
docker pull ghcr.io/gougoujiang/buildmax:latest
```

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
