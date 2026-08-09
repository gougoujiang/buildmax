// Package server provides the HTTP server for BuildMax.
//
// Route handling lives in the handlers sub-package:
//
//   - server/handlers — all HTTP handlers in one package, grouped by domain file:
//     auth.go (OTP/login + JWT middleware), teams.go, agents.go, usage.go,
//     issues.go, workflows.go, conversations.go, tasks.go, artifacts.go,
//     files.go, stream.go, ws.go, webhook_keys.go, inbound_webhook.go, worker.go.
//
// This package wires handlers.NewHandler, holds server Config, and serves
// healthz, openapi, swagger, CORS, and request-logging middleware.
package server
