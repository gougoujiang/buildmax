package scheduler

import "log/slog"

// This package runs four independent background loops. Each tags its records
// with the loop that wrote them, so one filter selects a subsystem instead of
// matching a prefix that every loop spells differently.
//
// Built per call rather than held in a package var: a var would capture
// slog.Default() before infra/log replaces it at startup.
func componentLog(name string) *slog.Logger { return slog.With("component", name) }

func (s *Scheduler) log() *slog.Logger         { return componentLog("scheduler") }
func (c *CredentialCleaner) log() *slog.Logger { return componentLog("credential_cleaner") }
func (c *StaleRunReaper) log() *slog.Logger    { return componentLog("stale_run_reaper") }
func (a *AuditRetainer) log() *slog.Logger     { return componentLog("audit_retention") }
