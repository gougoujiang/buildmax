# Design 074 - New ID solution

## Goal

Replace all entity ID generation with a single prefixed format `<prefix>_<body>` (body = 20-char base36 lowercase), remove ULID and the old `NewID()` API, and add `artifact_item_id` for artifact items.

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **internal/util** | Prefixed ID generation only. | `NewPrefixedID`, prefix constants, body encoding (lowercase base36, 20 chars). |
| **internal/storage/entity** | Entity persistence; uses util for all new IDs. | User, Workspace, Chat, ChatRun, Agent, Artifact, ArtifactItem creation. |
| **internal/model** | Domain structs and table mapping. | ArtifactItem + new `ArtifactItemID` field. |
| **internal/executor** | Run execution; generates artifact ID when reporting success. | Single call to `util.NewPrefixedID(util.PrefixArtifact)`. |

## Structure

**internal/util**

- `id.go` — Only prefixed ID: `NewPrefixedID(prefix string) string`; package-level prefix constants (`PrefixUser`, `PrefixWorkspace`, …); lowercase base36 body (20 chars) from crypto/rand. Remove `NewID`, `NewULID`, `encodeBase36` (replace with lowercase variant), and ULID import.
- `id_test.go` — Tests for `NewPrefixedID`: prefix present, body length 20, body charset `[a-z0-9]`, no collisions in a batch.

**internal/model**

- `models.go` — Add `ArtifactItemID string` to `ArtifactItem` with `gorm:"type:varchar(64);uniqueIndex;not null"` and `json:"artifact_item_id"`. Keep existing `ArtifactID` and `RelativePath`.

**internal/storage/entity**

- `user.go` — Replace `util.NewID()` with `util.NewPrefixedID(util.PrefixUser)`.
- `workspace.go` — Replace `util.NewID()` with `util.NewPrefixedID(util.PrefixWorkspace)` (both EnsureDefaultWorkspaceForUser and CreateWorkspace).
- `agent.go` — Replace `util.NewID()` with `util.NewPrefixedID(util.PrefixAgent)`.
- `chat.go` — Replace `util.NewULID()` for chatID and chatRunID with `util.NewPrefixedID(util.PrefixChat)` and `util.NewPrefixedID(util.PrefixChatRun)`.
- `chat_run.go` — Replace `util.NewULID()` with `util.NewPrefixedID(util.PrefixChatRun)`.
- `artifact.go` — `CreateArtifactWithItem`: when creating `ArtifactItem`, set `ArtifactItemID: util.NewPrefixedID(util.PrefixArtifactItem)`.
- `chat_run.go` — `OnRunComplete`: when creating `ArtifactItem`, set `ArtifactItemID: util.NewPrefixedID(util.PrefixArtifactItem)`.

**internal/executor**

- `executor.go` — Replace `util.NewULID()` with `util.NewPrefixedID(util.PrefixArtifact)` for artifact ID.

**Session / CLI**

- Leave session ID as UUID. No change to `internal/session` or `internal/cmd` for this task.

## Method design

| Location | Function / receiver | Signature | Responsibility |
|----------|---------------------|-----------|----------------|
| **util** | NewPrefixedID | `(prefix string) string` | Return `prefix + "_" + body` where body is 20 chars from `[a-z0-9]` (crypto-random, base36). Panic only on impossible rand failure. |
| **util** | (package constants) | `PrefixUser = "u"`, `PrefixWorkspace = "w"`, `PrefixAgent = "a"`, `PrefixChat = "c"`, `PrefixChatRun = "r"`, `PrefixArtifact = "ar"`, `PrefixArtifactItem = "f"` | Call sites pass these so prefix is consistent and typo-safe. |
| **util** | encodeBase36Lower | `(n *big.Int, length int) string` | Encode non-negative big.Int to base36 lowercase, zero-padded to `length`; used only by NewPrefixedID. |
| **entity.Store** | (existing) | CreateUser, CreateWorkspace, CreateAgent, CreateChat, CreateChatRun, CreateArtifactWithItem, OnRunComplete | No signature change; only the ID value source changes (NewPrefixedID with correct prefix). |

## How they work together

**Data/control flow**

1. Any code that creates an entity (User, Workspace, Agent, Chat, ChatRun, Artifact, ArtifactItem) calls `util.NewPrefixedID(util.PrefixX)` with the appropriate constant and assigns the result to the entity’s ID field.
2. Executor generates one artifact ID per successful run via `util.NewPrefixedID(util.PrefixArtifact)` and passes it to artifact storage and to the run updater (ArtifactPayload).
3. ArtifactItem rows get a new stable ID via `util.NewPrefixedID(util.PrefixArtifactItem)` in both `CreateArtifactWithItem` and `OnRunComplete`.

**Dependencies**

- `internal/util` has no dependency on entity or executor; it only needs `crypto/rand`, `math/big`, and `github.com/google/uuid` (for 128 bits of entropy; we still use UUID internally to feed big.Int, then encode to lowercase base36).
- `internal/storage/entity` and `internal/executor` depend on `internal/util` for ID generation.
- Session and CLI continue to use UUID for session ID; they do not use util for IDs in this task.

**Key data structures**

- **Prefixed ID string**: `"<prefix>_<body>"`, e.g. `"w_"` + 20 chars. Created by util; stored in DB and exposed in API unchanged.
- **ArtifactItem**: gains `ArtifactItemID string`; set once at create time in entity layer.

**Body generation (internal to util)**

- Read 128 bits from `crypto/rand` (or use UUID v4 and its bytes). Interpret as big.Int.
- Encode to base36 using alphabet `[a-z0-9]` (0–9, then a–z). Take exactly 20 characters: if value needs fewer digits, left-pad with '0'; if larger, take the low 20 digits (or re-sample until value < 36^20). This keeps collision probability negligible.

## Artifact item schema change

- **Table** `artifact_item`: add column `artifact_item_id` VARCHAR(64) NOT NULL UNIQUE (or unique index). GORM: add field `ArtifactItemID string` with `gorm:"type:varchar(64);uniqueIndex;not null" json:"artifact_item_id"`.
- **Creation**: every insert of `ArtifactItem` must set `ArtifactItemID` to `util.NewPrefixedID(util.PrefixArtifactItem)`. Existing rows (if any) are out of scope for migration; new rows only.

## Session ID decision

- **Leave session ID as UUID.** CLI session (`internal/session`) is independent of server Chat; changing it would require session file format or CLI flags to accept the new format. Out of scope for 074.

## Changes for review

- **New (util)**: `NewPrefixedID(prefix string) string`; package constants `PrefixUser`, `PrefixWorkspace`, `PrefixAgent`, `PrefixChat`, `PrefixChatRun`, `PrefixArtifact`, `PrefixArtifactItem`; internal `encodeBase36Lower(n *big.Int, length int) string`.
- **Removed (util)**: `NewID()`, `NewULID()`, current `encodeBase36`; remove dependency `github.com/oklog/ulid/v2` from `go.mod`.
- **Modified (util/id.go)**: Single ID API; lowercase base36 body length 20; doc comment listing prefix semantics.
- **Modified (util/id_test.go)**: Replace Tests for NewID/NewULID with tests for NewPrefixedID (prefix, length, charset, uniqueness); drop ulid import.
- **Modified (internal/model/models.go)**: `ArtifactItem` gains `ArtifactItemID string` with gorm and json tags.
- **Modified (internal/storage/entity)**: user.go, workspace.go, agent.go, chat.go, chat_run.go, artifact.go — switch to `util.NewPrefixedID(util.PrefixX)` and set `ArtifactItemID` when creating ArtifactItem in CreateArtifactWithItem and OnRunComplete.
- **Modified (internal/executor/executor.go)**: Use `util.NewPrefixedID(util.PrefixArtifact)` for artifact ID.
- **Database**: New column `artifact_item_id` on `artifact_item` (GORM AutoMigrate or equivalent will add it; new rows only).
- **No change**: Session package, cmd/root (session-id remains UUID), API route shapes or parameter names.
