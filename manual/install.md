# Install BuildMax

BuildMax ships three binaries. `buildmax` is the local CLI/TUI and is all you
need to start; `buildmax-server` and `buildmax-worker` are for a space
deployment.

Before choosing an install path, check the [support matrix](support.md) for the
current alpha platform and deployment boundaries.

| Component | Release targets | Distribution |
|---|---|---|
| CLI, server, worker | Linux amd64/arm64, macOS amd64/arm64, Windows amd64 | Release archives |
| Server and worker container | Linux amd64/arm64 | GHCR image |
| Portal | Linux amd64/arm64 | GHCR image, or build from source |
| Desktop | macOS arm64 (`.dmg`), Windows amd64 (`.exe`) | Release download; unsigned |

## Release archive

Download an archive for your platform from
[Releases](https://github.com/icloudbb/buildmax/releases). Each one contains
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
gh attestation verify --owner icloudbb buildmax_<version>_<os>_<arch>.tar.gz
```

Windows archives use `.zip`. Checksums prove the downloaded bytes match the
release; the attestation proves GitHub Actions built those bytes from this
repository.

## Go toolchain

For the CLI alone:

```bash
go install github.com/icloudbb/buildmax/cmd/buildmax@latest
```

A binary installed this way reports the module version it was built from —
`buildmax version` prints `0.1.0-alpha`, not `dev`. It carries no commit hash,
because `go install` records no VCS stamp; release archives report both.

## Container

One image carries all three binaries; a second serves the Portal:

```bash
docker pull ghcr.io/icloudbb/buildmax:<version>
docker pull ghcr.io/icloudbb/buildmax-portal:<version>
```

Both are published per release tag and carry the same version. Alpha releases
deliberately do not move `latest`, so name the version you want.

## From source

Requires Go (version in `go.mod`), plus Node only if you also want the
frontends:

```bash
git clone https://github.com/icloudbb/buildmax.git
cd buildmax
./make build          # CLI, server, worker, shared GUI, desktop app → bin/
```

`./make build cli` builds just the CLI. On Windows use `make.bat` with the same
commands.

## Desktop app

Download the desktop build for your platform from
[Releases](https://github.com/icloudbb/buildmax/releases): a `.dmg` on macOS, a
self-contained `.exe` on Windows. Each carries a `.sha256` beside it; verify the
download before running it.

```bash
shasum -a 256 -c buildmax-desktop_<version>_darwin_arm64.dmg.sha256   # macOS
```

The bundles are **not signed or notarized** during alpha, so the operating
system holds a fresh download until you clear it once:

- **macOS:** mount the `.dmg`, drag `BuildMax.app` to `/Applications`, then
  remove the download quarantine so Gatekeeper opens it:

  ```bash
  xattr -dr com.apple.quarantine /Applications/BuildMax.app
  ```

  Or right-click the app and choose **Open** the first time to approve it.
- **Windows:** SmartScreen warns on the unsigned binary. Choose **More info →
  Run anyway** the first time.

Signing and notarization are planned; until then this is the trade for a
download that runs.

### Build from source instead

An app you build yourself carries no download to be cleared, and Wails signs the
bundle ad-hoc as it packages it. `./make build desktop` produces it in `bin/`
in about a minute, and `./make run desktop` starts it. `./make build` builds it
alongside everything else.

## Next

Configure a model with `buildmax init` and run your first task:
[Quickstart](quickstart.md).
