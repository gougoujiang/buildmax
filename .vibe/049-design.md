# Design 049: Shorter ID Solution

## Goal

Replace `newUUID()` in the store with a new `internal/id` package that generates 22-character base62-encoded IDs from UUIDs. Update GORM model tags to accommodate both old and new ID lengths.

## Modules

### 1. `internal/id` (new package)

Provides a single public function `New()` that generates a short, globally unique ID.

### 2. `internal/store` (existing — modified)

Replace `newUUID()` calls with `id.New()`. Update GORM struct tags for column widths. Remove the old helper and the `github.com/google/uuid` import (no longer needed in this package).

## Structure

### `internal/id/id.go`

```go
package id

import (
    "encoding/hex"
    "math/big"

    "github.com/google/uuid"
)

const (
    alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
    base     = 62
    idLen    = 22 // 128-bit UUID fits in 22 base62 digits
)

// New generates a globally unique 22-character base62 ID.
// Internally creates a UUID v4, interprets its 128 bits as a big.Int,
// and encodes in base62.
func New() string
```

### `internal/id/id_test.go`

```go
package id

import "testing"

func TestNew(t *testing.T)           // length, charset, uniqueness
func TestEncode(t *testing.T)        // known input/output pairs
```

### `internal/store/store.go` (changes)

```go
// Remove:
//   import "github.com/google/uuid"
//   func newUUID() string { ... }

// Add:
//   import "buildmax/internal/id"

// Replace all newUUID() calls with id.New():
//   EnsureDefaultWorkspaceForUser — WorkspaceID: id.New()
//   CreateWorkspace               — WorkspaceID: id.New()
//   CreateProject                 — ProjectID:   id.New()
//   CreateTask                    — TaskID:      id.New()

// Update GORM tags — varchar(36) → varchar(64) on all public-ID / FK columns.
// session_id stays varchar(36).
```

## Method Design

### `id.New() string`

- **Receiver**: none (package-level function).
- **Parameters**: none.
- **Returns**: `string` — exactly 22 characters, chars from `[0-9A-Za-z]`.
- **Responsibility**: Generate a UUID v4, convert 16 raw bytes to a `*big.Int`, then repeatedly divmod by 62 to produce 22 base62 digits. Left-pad with `'0'` if the result is shorter than 22 chars. Return the string.
- **Algorithm**:
  1. `u := uuid.New()` — 16-byte array.
  2. `n := new(big.Int).SetBytes(u[:])` — interpret as unsigned 128-bit integer.
  3. Loop: `n.DivMod(n, big62, mod)` → map `mod` to `alphabet[mod]`, prepend to result.
  4. Pad left with `'0'` to length 22.
- **Why 22?**: `ceil(128 × log2 / log62) = ceil(21.49) = 22`. Max UUID `2^128 - 1` encodes to `"7n42DGM5Tflk9n8mt7Fhc7"` (22 chars). The minimum is `"0000000000000000000001"` (padded).

### `id.encode(n *big.Int) string` (unexported helper)

- **Receiver**: none.
- **Parameters**: `n *big.Int` — non-negative integer.
- **Returns**: `string` — base62 encoding of `n`, zero-padded to `idLen`.
- **Responsibility**: Pure encoding logic, separated for testability with known inputs.

## GORM Model Changes

All `varchar(36)` tags on public-ID and FK columns become `varchar(64)`. This is a non-destructive `ALTER TABLE ... MODIFY COLUMN` that GORM AutoMigrate handles automatically (widening a varchar is safe; no data loss).

| Struct | Field | Old tag | New tag |
|--------|-------|---------|---------|
| User | UserID | `varchar(36)` | `varchar(64)` |
| Workspace | WorkspaceID | `varchar(36)` | `varchar(64)` |
| Workspace | OwnerUserID | `varchar(36)` | `varchar(64)` |
| Project | ProjectID | `varchar(36)` | `varchar(64)` |
| Project | WorkspaceID | `varchar(36)` | `varchar(64)` |
| Task | TaskID | `varchar(36)` | `varchar(64)` |
| Task | WorkspaceID | `varchar(36)` | `varchar(64)` |
| Task | ProjectID | `varchar(36)` | `varchar(64)` |
| Task | CreatedBy | `varchar(36)` | `varchar(64)` |
| Task | SessionID | `varchar(36)` | **unchanged** |

## How They Work Together

1. On server startup, `store.New()` calls GORM `AutoMigrate`. GORM detects the wider `varchar(64)` tags and issues `ALTER TABLE ... MODIFY COLUMN` statements. Existing data is untouched.
2. When any `Create*` method runs, it calls `id.New()` instead of the old `newUUID()`. The returned 22-char string is stored in the `varchar(64)` column.
3. All downstream code (API handlers, JWT claims, portal) already treats IDs as opaque strings — no changes needed.
4. Old rows with 36-char UUIDs continue to work. Lookups use `WHERE workspace_id = ?` etc., matching by string equality regardless of length.

## Changes for Review

| Action | Package / File | What |
|--------|---------------|------|
| **New** | `internal/id/id.go` | `New()`, `encode()`, constants |
| **New** | `internal/id/id_test.go` | `TestNew`, `TestEncode` |
| **Edit** | `internal/store/store.go` | Remove `newUUID()` + uuid import; add `id` import; replace 4 call sites; update 9 GORM varchar tags |
