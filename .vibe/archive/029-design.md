# Design 029: Use --model arg to specify model

## Goal

Add a `--model` root flag that selects a model from `settings.json` by model ID or display name; when set, use that entry for both prompt mode and TUI. When the selector is set but settings have no models or no matching entry, exit with a clear error.

## Modules

| Module (package) | Responsibility | Changes |
|------------------|----------------|---------|
| **internal/config** | Configuration loading and model resolution | New `EffectiveLLMWithSelector(path, modelSelector string) (LLM, string, error)`; `EffectiveLLM(path)` delegates to it with empty selector. |
| **internal/cmd** | Root flags, setup, and run modes | Root: add `--model` flag; pass value to `runPrintMode` / `runTUI`. `setupAgentAndSession(resumeID, modelSelector string)`; `runPrintMode(prompt, resumeID, modelSelector)`; `runTUI(resumeID, modelSelector)`. |

No changes to `internal/llm`, `internal/agent`, or `internal/tui`; they continue to receive `config.LLM` and display name via `setupResult`.

## Structure

**Model selector semantics**

- Selector is a string (the value of `--model`). Empty means “use default” (first model from settings or env fallback).
- Matching: iterate `settings.Models` in order; for each entry, if `entry.Model == selector` or `entry.Name == selector`, use that entry (first match wins). Exact string comparison; no case folding or trimming.
- When selector is non-empty: if settings are missing or `models` is empty or invalid, return a single error (e.g. "no models in settings; add settings or omit --model"). If settings have models but no entry matches, return error (e.g. "model not found: <selector>").

**internal/config**

- **New function**  
  `EffectiveLLMWithSelector(settingsPath string, modelSelector string) (LLM, string, error)`  
  - If `modelSelector == ""`: same behaviour as current `EffectiveLLM(path)` — `LoadSettings(path)`; if no models, return `LoadLLM()` and `cfg.Model` (no error); else first entry → LLM + display name. So when selector is empty, returned error is always nil.  
  - If `modelSelector != ""`: load settings. If `len(s.Models) == 0`, return error "no models in settings; add settings or omit --model". Else loop over `s.Models`; first entry where `m.Model == modelSelector` or `m.Name == modelSelector` → build LLM from that entry, display name = `m.Name` or `m.Model`, return. If no match, return error "model not found: ..." (include selector in message).
- **EffectiveLLM(path string) (LLM, string)**  
  Keep; implement as `cfg, name, _ := EffectiveLLMWithSelector(path, ""); return cfg, name` so existing callers and tests remain valid.

**internal/cmd**

- **root.go**: Add `root.Flags().String("model", "", "use model from settings by model id or name")`. In `runRoot`, read `model, _ := cmd.Flags().GetString("model")`, then call `runPrintMode(prompt, resumeID, model)` or `runTUI(resumeID, model)`.
- **setup.go**: `setupAgentAndSession(resumeID string, modelSelector string) (setupResult, error)`. Call `cfg, modelName, err := config.EffectiveLLMWithSelector("", modelSelector)`; if err != nil, return zero value and err. Rest unchanged (API key check, build client, tools, session, return result with `ModelName: modelName`).
- **print.go**: `runPrintMode(prompt string, resumeID string, modelSelector string) error`. Call `setupAgentAndSession(resumeID, modelSelector)`; on error print to stderr and return. Rest unchanged.
- **tui.go**: `runTUI(resumeID string, modelSelector string) error`. Call `setupAgentAndSession(resumeID, modelSelector)`; on error return. Rest unchanged.

## Method design

| Package | Function | Signature | Responsibility |
|---------|----------|-----------|----------------|
| config | EffectiveLLMWithSelector | `(settingsPath, modelSelector string) (LLM, string, error)` | Load settings from path ("" → SettingsPath()). Empty selector: first model or LoadLLM fallback, never error. Non-empty: require at least one model; find first entry with Model or Name == selector; return that LLM and display name or error. |
| config | EffectiveLLM | `(path string) (LLM, string)` | Call EffectiveLLMWithSelector(path, ""), return first two values (ignore error). |
| cmd | setupAgentAndSession | `(resumeID, modelSelector string) (setupResult, error)` | Call EffectiveLLMWithSelector("", modelSelector); on error return it. Else build client, tools, session as today; set ModelName from returned display name. |
| cmd | runPrintMode | `(prompt, resumeID, modelSelector string) error` | Call setupAgentAndSession(resumeID, modelSelector); on error print and return. Then run agent, save session, print reply as today. |
| cmd | runTUI | `(resumeID, modelSelector string) error` | Call setupAgentAndSession(resumeID, modelSelector); on error return. Then start TUI with result as today. |

## How they work together

**Startup flow with --model**

1. User runs `buildmax --model "Gemma 3 27B"` or `buildmax -p "hello" --model openai/gpt-3.5-turbo`.
2. `runRoot` reads the `--model` flag and calls `runTUI(resumeID, model)` or `runPrintMode(prompt, resumeID, model)`.
3. `runTUI` / `runPrintMode` call `setupAgentAndSession(resumeID, modelSelector)`.
4. `setupAgentAndSession` calls `config.EffectiveLLMWithSelector("", modelSelector)`. If selector is empty, behaviour is unchanged (first model or env). If selector is set and no models or no match, error is returned and propagated to user (stderr + exit for print, return error for TUI).
5. On success, setup result (including selected model’s display name) is used as today; TUI footer and prompt output show that name.

**Error messages**

- No models when selector set: e.g. `"no models in settings; add settings or omit --model"`.
- Selector set but no match: e.g. `"model not found: <selector>"`.

**Tests (config)**

- `EffectiveLLMWithSelector(path, "")`: with path to file that has models → same result as `EffectiveLLM(path)`; with path to missing/empty models → same as current fallback; never error.
- `EffectiveLLMWithSelector(path, "id-from-entry")`: file with one or more models, one entry has `model` equal to selector → returns that entry’s LLM and display name.
- `EffectiveLLMWithSelector(path, "display-name")`: entry has `name` equal to selector → returns that entry.
- `EffectiveLLMWithSelector(path, "nonexistent")`: file has models but no match → returns non-nil error, message contains selector.
- `EffectiveLLMWithSelector(path, "anything")` with path to missing file or file with `{}`/empty models → returns non-nil error (no models in settings).

Use temp dir and temp settings file; no network. Keep existing `EffectiveLLM` tests; they still pass via delegation.

## Changes for review

- **Modified — internal/config/config.go**: Add `EffectiveLLMWithSelector(settingsPath, modelSelector string) (LLM, string, error)`. Implement `EffectiveLLM(path)` by calling `EffectiveLLMWithSelector(path, "")` and returning the first two values (error is always nil when selector is empty).
- **Modified — internal/cmd/root.go**: Add `--model` string flag. In `runRoot`, get flag value and pass to `runPrintMode(prompt, resumeID, model)` and `runTUI(resumeID, model)`.
- **Modified — internal/cmd/setup.go**: Change `setupAgentAndSession(resumeID string)` to `setupAgentAndSession(resumeID string, modelSelector string) (setupResult, error)`. Use `EffectiveLLMWithSelector("", modelSelector)`; on error return it.
- **Modified — internal/cmd/print.go**: Change `runPrintMode(prompt, resumeID string)` to `runPrintMode(prompt, resumeID, modelSelector string)`; call `setupAgentAndSession(resumeID, modelSelector)`.
- **Modified — internal/cmd/tui.go**: Change `runTUI(resumeID string)` to `runTUI(resumeID string, modelSelector string)`; call `setupAgentAndSession(resumeID, modelSelector)`.
- **Modified — internal/config/config_test.go**: Add tests for `EffectiveLLMWithSelector`: empty selector (unchanged behaviour); match by model; match by name; no match (error); no settings + non-empty selector (error).
- **Unchanged**: `internal/llm`, `internal/agent`, `internal/tui`, `cmd/buildmax`, other packages.
