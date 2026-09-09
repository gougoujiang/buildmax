package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The ephemeral kind cluster a task owns, recorded in .local/ rather than an
// environment variable because each `mk` invocation is a fresh process: a task
// runs `kind up`, then `e2e kind`, then `kind down` as separate commands, and
// only a file carries the cluster's identity across them. .local/ is the
// gitignored home for a contributor's own state (see local.go).
//
// This is how a task gets an isolated cluster without touching the resident
// buildmaxdev one a person keeps for manual testing: create with
// BUILDMAX_KIND_EPHEMERAL=1, and every later kind command in this worktree reads
// the same identity back, then `kind down` removes both the cluster and this
// record so the worktree falls back to the default.
const ephemeralKindMarker = localDir + "/kind-ephemeral.env"

type ephemeralKind struct {
	cluster    string
	portalPort string
	tlsPort    string
}

// readEphemeralKind returns the recorded ephemeral cluster, or ok=false when
// this worktree has none (the common case, which targets buildmaxdev).
func readEphemeralKind() (ephemeralKind, bool) {
	data, err := os.ReadFile(ephemeralKindMarker)
	if err != nil {
		return ephemeralKind{}, false
	}
	var e ephemeralKind
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch key {
		case "cluster":
			e.cluster = value
		case "portal_port":
			e.portalPort = value
		case "tls_port":
			e.tlsPort = value
		}
	}
	// A half-written record is treated as none: better to fall back to the
	// default than to point half the commands at a cluster and half elsewhere.
	if e.cluster == "" || e.portalPort == "" || e.tlsPort == "" {
		return ephemeralKind{}, false
	}
	return e, true
}

func writeEphemeralKind(e ephemeralKind) error {
	if err := os.MkdirAll(localDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", localDir, err)
	}
	body := fmt.Sprintf("cluster=%s\nportal_port=%s\ntls_port=%s\n", e.cluster, e.portalPort, e.tlsPort)
	if err := os.WriteFile(ephemeralKindMarker, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", ephemeralKindMarker, err)
	}
	return nil
}

func clearEphemeralKind() error {
	if err := os.Remove(ephemeralKindMarker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", ephemeralKindMarker, err)
	}
	return nil
}

// allocateEphemeralKind picks a name nothing else uses and two free host ports,
// records them for the rest of this worktree's kind commands, and returns them.
// Called only by `kind up` under BUILDMAX_KIND_EPHEMERAL=1, and only when this
// worktree has no ephemeral cluster yet.
func allocateEphemeralKind() (ephemeralKind, error) {
	suffix, err := randomHex(4)
	if err != nil {
		return ephemeralKind{}, fmt.Errorf("choose an ephemeral kind cluster name: %w", err)
	}
	portal, err := freeTCPPort()
	if err != nil {
		return ephemeralKind{}, fmt.Errorf("choose a free portal port: %w", err)
	}
	tls, err := freeTCPPort()
	if err != nil {
		return ephemeralKind{}, fmt.Errorf("choose a free TLS port: %w", err)
	}
	e := ephemeralKind{
		cluster:    "buildmax-eph-" + suffix,
		portalPort: strconv.Itoa(portal),
		tlsPort:    strconv.Itoa(tls),
	}
	if err := writeEphemeralKind(e); err != nil {
		return ephemeralKind{}, err
	}
	return e, nil
}
