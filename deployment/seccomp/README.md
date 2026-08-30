# Worker seccomp profile

`worker-bwrap.json` is a `securityContext.seccompProfile: {type: Localhost}`
profile for the worker Job pod (`internal/infra/k8s/job.go`). It exists
because Kubernetes' `RuntimeDefault` seccomp profile blocks bubblewrap from
creating the unprivileged user/mount/pid namespace it needs, confirmed
against a real pod running the worker's exact `PodSecurityContext` (non-root,
`Capabilities: {drop: [ALL]}`, `RuntimeDefault`, read-only root filesystem):
`bwrap` failed with `Creating new namespace failed: Operation not permitted`.
See [`docs/design/agent-sandbox-policy.md`](../../docs/design/agent-sandbox-policy.md)
and [`docs/current-state.md`](../../docs/current-state.md) for how this fits
the broader sandbox effort.

## Provenance

Base: Docker's own default seccomp profile,
[`daemon/pkg/oci/fixtures/default.json`](https://github.com/moby/moby/blob/master/daemon/pkg/oci/fixtures/default.json)
at blob `8d4d21145eef22ced811b84244da6f6f1b5e8704`, © Docker/Moby project,
licensed Apache License 2.0 — the same profile `RuntimeDefault` is modeled
on, deny-by-default (`SCMP_ACT_ERRNO`) with ~375 syscalls allowed. Used as
the base rather than writing an allowlist from scratch, for the same reason
[`sandbox-boundaries.md`](../../docs/design/sandbox-boundaries.md) shells out
to `bwrap`/`sandbox-exec` rather than reimplementing them: a widely-deployed,
audited default is safer than a bespoke one.

## What changed, and why

Root cause, confirmed with `strace` against a real pod: bubblewrap needs
`unshare`, `setns`, `mount`, `umount2`, `pivot_root`, `clone`, and `clone3` to
build its sandbox, and Docker's own default profile gates most of these
behind `"includes": {"caps": ["CAP_SYS_ADMIN"]}` — a rule that is *dropped
entirely* from the compiled filter when the container's capability bounding
set is empty, which the worker pod's `Capabilities: {drop: [ALL]}` makes it.
This profile removes that gate for exactly those seven syscalls, allowing
them unconditionally, and leaves every other entry from the base profile
untouched. The kernel's own privilege model still enforces the real boundary:
`unshare(CLONE_NEWUSER)` needs no capability at all by design, and the process
only gains capabilities *inside* the namespace it just created for itself —
this profile just stops seccomp from pre-emptively blocking the syscall
before the kernel gets to make that decision.

Verified end to end against a real pod carrying the worker's exact
`PodSecurityContext`: a full `bwrap --ro-bind / / --ro-bind /proc /proc --dev
/dev --tmpfs /tmp --bind <ws> <ws> --die-with-parent --unshare-pid
--unshare-ipc --unshare-uts -- <command>` run — the same argv
`internal/infra/sandbox/bwrap_linux.go` builds — confining a real command:
read inside the bound workspace succeeded, a write outside it was denied with
`Read-only file system`.

## The other half of the fix: `--ro-bind /proc /proc`, not `--proc /proc`

Allowing these syscalls was necessary but not sufficient. With `--unshare-pid`
and a *freshly mounted* `/proc` (`--proc /proc`, bubblewrap's default), the
kernel additionally refused the mount with `EPERM` and logged `VFS: Mount too
revealing` — a real, deliberate Linux VFS protection
(`SB_I_USERNS_VISIBLE` / `mnt_already_visible()` in `fs/namespace.c`), not a
seccomp restriction and not fixable by this profile: it stops a nested,
less-privileged mount namespace from mounting a fresh procfs that would
un-hide paths a container runtime already masked in an ancestor mount
namespace. `internal/infra/sandbox/bwrap_linux.go` re-binds the parent's
`/proc` read-only instead. The accepted cost: a sandboxed process sees the
host container's process list under `/proc`, not an isolated one — the
trade-off for being able to run bubblewrap inside a container's own PID
namespace at all. This is not specific to Kubernetes' seccomp defaults; the
same failure reproduced with seccomp fully disabled and the pod running as
real root, so it would reproduce on any container runtime that masks `/proc`
paths in the way containerd and dockerd both do by default.

## Distribution

Kubernetes' `Localhost` seccomp profile type requires the JSON file to exist
on every node under `<kubelet-root-dir>/seccomp/` (default
`/var/lib/kubelet/seccomp/`) before a pod naming it can start — there is no
way to inline the profile in the pod spec. The reference deployment ships it
via a `DaemonSet` that copies this file from a `ConfigMap` onto that path on
every node; see `deployment/buildmax-deploy.yaml`.
