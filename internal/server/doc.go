// Package server provides the HTTP server for BuildMax (backend for portal).
//
// Shared HTTP helpers (use these consistently; do not duplicate logic in handlers):
//
//   - Auth: auth.go — requireAuth, withWorkspaceAuth, userIDFromRequest. All workspace-scoped
//     handlers must use withWorkspaceAuth to validate JWT and workspace ownership before
//     touching stores.
//   - Response: response.go — writeJSON, writeJSONError, writeQuotaExceeded, writeInternalError.
//     Use these as the single way to send JSON responses.
//   - Query: query.go — parseLimitOffset for pagination query params.
//   - Paths: paths.go — workspacesDir(), persistentWorkspaceDir() for workspace file paths.
//   - Middleware: middleware.go — corsMiddleware, requestLoggingMiddleware, workerAuthMiddleware.
package server
