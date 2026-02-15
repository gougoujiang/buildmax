# Design 028: Multiple model support

## Goal

Introduce a settings file (`settings.json`) under the application data directory that defines a list of LLM models; at startup use the first model from that list when present, otherwise fall back to env-based `LoadLLM()`. Expose a display name (entry `name` or `model`) for the TUI/prompt footer.

## Modules

| Module (package) | Responsibility | Changes |
|------------------|----------------|---------|
| **internal/config** | Configuration loading and paths | New types `Settings`, `ModelEntry`; `SettingsPath()`, `LoadSettings(path)`, `EffectiveLLM(path)`; keep `LoadLLM()` unchanged. |
| **internal/cmd** | Setup and run modes | `setupAgentAndSession` uses `EffectiveLLM("")` instead of `LoadLLM()`, sets `ModelName` from returned display name. |

No changes to `internal/llm`, `internal/agent`, or `internal/tui`; they continue to receive `config.LLM` and a model name string.

## Structure

**Settings file**

- Path: `DataDir()/settings.json` (e.g. `~/.buildmax/settings.json`). No file or directory creation in this task.
- JSON shape (snake_case keys per AGENTS.md §6.1):
  - Root object: `models` (array of model objects). Other top-level keys are ignored.
  - Each model object: `model` (required), `name` (optional), `api_url` (required), `api_key` (required).

**internal/config**

- **Types** (in `config.go`):
  - `Settings`: struct with `Models []ModelEntry`, JSON tag `models`.
  - `ModelEntry`: struct with `Model`, `Name`, `APIURL`, `APIKey`; JSON tags `model`, `name`, `api_url`, `api_key`.
- **Functions** (in `config.go`):
  - `SettingsPath() string` — returns `filepath.Join(DataDir(), "settings.json")`.
  - `LoadSettings(path string) Settings` — if `path == ""` use `SettingsPath()`. Read file; if missing, unreadable, or invalid JSON, return `Settings{}` (zero value; `Models` nil or empty). Otherwise return parsed struct. No panic; no creation of file/dir.
  - `EffectiveLLM(path string) (LLM, displayName string)` — load via `LoadSettings(path)`. If `len(settings.Models) == 0`, return `LoadLLM()` and `cfg.Model` as display name. Else use first entry: map to `LLM{Model: m.Model, BaseURL: m.APIURL, APIKey: m.APIKey}`, display name = `m.Name` if non-empty else `m.Model`. No validation of first entry beyond using it; invalid values surface at LLM call time.
- **Unchanged**: `LoadLLM()`, `LLM`, `DataDir()`, and all other existing symbols.

**internal/cmd/setup.go**

- In `setupAgentAndSession`: replace `cfg := config.LoadLLM()` with `cfg, modelName := config.EffectiveLLM("")`. Use `cfg` for API key check and `llm.NewClient(cfg)`; set `ModelName: modelName` in `setupResult`.

## Method design

| Package | Function | Signature | Responsibility |
|---------|----------|-----------|----------------|
| config | SettingsPath | `() string` | Return `filepath.Join(DataDir(), "settings.json")`. |
| config | LoadSettings | `(path string) Settings` | If path empty, set path = SettingsPath(). Read path; on missing file or parse error return empty Settings; else return parsed Settings. |
| config | EffectiveLLM | `(path string) (LLM, string)` | LoadSettings(path). If no models, return LoadLLM() and cfg.Model. Else return first model as LLM and display name (entry name or model id). |
| config | LoadLLM | (existing) | Unchanged; used by EffectiveLLM when falling back. |

**Data structures**

- `Settings`: `Models []ModelEntry` with `json:"models"`.
- `ModelEntry`: `Model string`, `Name string`, `APIURL string`, `APIKey string` with tags `model`, `name`, `api_url`, `api_key`.

## How they work together

**Startup flow**

1. `setupAgentAndSession` is called (from `runTUI` or `runPromptMode`).
2. It calls `cfg, modelName := config.EffectiveLLM("")`.
3. `EffectiveLLM("")` calls `LoadSettings(SettingsPath())`. If the file is missing or has no models, it returns `LoadLLM()` and `cfg.Model`. Otherwise it maps the first model entry to `LLM` and returns that plus the display name.
4. `setupAgentAndSession` uses `cfg` for the API key check and `llm.NewClient(cfg)`; passes `modelName` as `setupResult.ModelName`.
5. TUI and prompt mode use `setupResult.ModelName` as today (footer, etc.); no change to their code.

**Testing**

- Config tests: use a temp dir (or `t.TempDir()`) and optionally set `BUILDMAX_HOME` to it; write `settings.json` with valid models and call `LoadSettings(path)` or `EffectiveLLM(path)` with that path. Assert effective LLM and display name. For fallback: call `EffectiveLLM(path)` with a path to a missing file, or a file with `{}` or `{"models":[]}`; assert result equals `LoadLLM()` and model id. For invalid JSON, assert fallback (empty settings → EffectiveLLM returns env-based config). No real API calls.

**Dependencies**

- `config`: add `encoding/json`, `os` (already present). Read file with `os.ReadFile`; decode into `Settings`.
- `cmd`: already imports `config`; no new imports.

## Changes for review

- **Modified — internal/config/config.go**: Add `Settings`, `ModelEntry`; add `SettingsPath`, `LoadSettings`, `EffectiveLLM`. Leave `LoadLLM`, `LLM`, and all path helpers unchanged.
- **Modified — internal/cmd/setup.go**: In `setupAgentAndSession`, replace `config.LoadLLM()` with `config.EffectiveLLM("")`; use returned display name for `setupResult.ModelName`.
- **Modified — internal/config/config_test.go**: Add tests for `SettingsPath`; `LoadSettings` (valid file with one or more models, missing file, empty models, invalid JSON); `EffectiveLLM` (uses first model and display name when file has models; falls back to LoadLLM when no file or empty models). Use temp dir and/or temp file; no network.
- **Unchanged**: `internal/llm`, `internal/agent`, `internal/tui`, `cmd/buildmax`, all other packages.
