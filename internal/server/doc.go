// Package server provides the HTTP server for BuildMax (backend for portal).
//
// The server is split by API kind:
//
//   - server/auth — unauthenticated entry: POST /api/otp/request, POST /api/login.
//   - server/portal — authenticated user API: agents, tasks, artifacts,
//     conversations, stream, files, upload, usage.
//   - server/worker — worker API: GET/PATCH /api/worker/task-runs/{id}, POST .../stream.
//     Uses Bearer or X-Worker-Token auth.
//
// This package wires the three above (auth.Register, portal.Register, worker.Register),
// holds Config and the stream hub, and serves healthz, openapi, swagger and middleware.
package server
