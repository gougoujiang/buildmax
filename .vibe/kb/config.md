# Configuration

## Purpose

The `internal/config` package provides environment-variable-based configuration for LLM settings and the application data directory. No config files are used yet — everything is read from environment variables.

## Key Types and Interfaces

| Name | Kind | Role |
|------|------|------|
| **LLM** | struct | Holds API key, base URL, and model name |

## How It Works

### LLM Configuration (`LoadLLM()`)

Reads three settings from environment variables:

| Setting | Env Var(s) | Default |
|---------|-----------|---------|
| API Key | `OPENROUTER_API_KEY` or `BUILDMAX_API_KEY` | (none — required for operation) |
| Base URL | `BUILDMAX_BASE_URL` | `https://openrouter.ai/api/v1` |
| Model | `BUILDMAX_MODEL` | `z-ai/glm-4.5-air:free` |

Priority: `OPENROUTER_API_KEY` takes precedence over `BUILDMAX_API_KEY`.

### Data Directory (`DataDir()`)

Returns the path to the application's data folder:

| Condition | Path |
|-----------|------|
| `HOME_DIR` env var set | `filepath.Clean($HOME_DIR)` |
| Otherwise | `~/.buildmax` |

The data directory stores:
- `sessions/` — Session JSON files and session list index
- `logs/` — Rotating log files

`DataDir()` does **not** create the directory — callers must `os.MkdirAll` as needed.

### Available Models (Constants)

```go
ModelGPT35Turbo      = "openai/gpt-3.5-turbo"       // NOT FREE
ModelGemma327bItFree = "google/gemma-3-27b-it:free"
ModelGLM45AirFree    = "z-ai/glm-4.5-air:free"      // current default
```

## Dependencies

- **Uses**: Go standard library only (`os`, `path/filepath`)
- **Used by**: `cmd/buildmax` (loads config at startup), `internal/llm` (Client uses LLM struct)

## Notes

- Setting `HOME_DIR` is useful for testing (`make.bat test` sets it to `./testing-sandbox`).
- Config files (YAML/Viper) are planned but not yet implemented.
- See also: [LLM Client](llm-client.md), [CLI](cli.md), [Project Overview](project-overview.md).
