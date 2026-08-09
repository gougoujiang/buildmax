# Install

> **Audience:** users · **Status:** current

BuildMax ships three binaries. `buildmax` is the local CLI/TUI and is all you
need to start; `buildmax-server` and `buildmax-worker` are for the team
deployment described in [deploy/](../deploy/overview.md).

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

## Go Toolchain

For the CLI alone:

```bash
go install github.com/gougoujiang/buildmax/cmd/buildmax@latest
```

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

Configure a model and run your first task: [quickstart.md](quickstart.md).
