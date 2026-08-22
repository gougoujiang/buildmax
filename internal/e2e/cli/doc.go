// Package clie2e drives the built CLI binary end to end against a scripted
// model.
//
// It exists because the CLI is the one surface whose boundary — a real binary,
// its own BUILDMAX_HOME, the workspace, the permission gate, and the agent loop
// — is only assembled when the process runs. Everything here uses the real
// binary and a temporary home; nothing reaches a provider.
//
// The Marketplace tests add a second boundary: a server the binary does not
// share memory with, serving the real catalog routes over in-memory stores.
// Publishing and installing only mean anything when they cross it.
//
// See docs/design/end-to-end-testing.md §6.
package clie2e
