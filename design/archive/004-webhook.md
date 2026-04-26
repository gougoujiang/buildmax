# Webhook configuration

Webhook allows external systems to trigger a Tier 2 run (one TaskRun) by sending an HTTP POST. No LLM is used at Tier 1 for webhook; the request is mapped directly to a new task run.

## Auth: per-workspace API keys

- Each workspace can have one or more **webhook API keys**. Create and revoke keys in the portal: **Workspace settings → Webhook API keys**.
- When calling the webhook endpoint, send the key in one of:
  - `Authorization: Bearer <key>`
  - `X-Webhook-Key: <key>`
- The server resolves the key to a workspace. The path `workspace_id` must match that workspace (401 invalid key, 403 if path does not match).
- Keys are stored as SHA256 hash; the plaintext is shown **once** when you create a key. Copy it immediately; it cannot be retrieved later.

## Endpoint

- **POST** `/api/workspaces/{workspace_id}/webhook`
- **Body**: JSON. The message (task input) is read from a configurable path (default `"message"`). Set via `BUILDMAX_WEBHOOK_MESSAGE_PATH` (e.g. `message`, `body.text`).
- **Example**:
  ```json
  { "message": "Summarize the README" }
  ```
  or with callback:
  ```json
  { "message": "Run tests", "callback_url": "https://your-server.com/callback" }
  ```
- **Response**: `202 Accepted` with `{ "task_id": "<task_run_id>" }`. Use the task_id to poll run status or to correlate with run-complete callbacks.

## Callback (optional)

- If the body includes `"callback_url": "https://..."`, it may be stored and invoked when the run completes (implementation may defer POST to a follow-up).
- **Callback payload** (documented shape): `{ "task_run_id": "...", "status": "SUCCEEDED" | "FAILED", "reply_summary": "..." }`.

## Env (optional)

- `BUILDMAX_WEBHOOK_MESSAGE_PATH`: JSON path for message in body (default `message`).
- `BUILDMAX_WEBHOOK_USER_ID`: User ID used as `CreatedBy` for webhook-created runs (default `webhook`).
