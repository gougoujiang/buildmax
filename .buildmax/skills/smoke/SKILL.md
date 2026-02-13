---
name: smoke
description: "Quick smoke test for BuildMax agent. Levels: 0, 1, 2."
---

# Smoke Test

Run a quick smoke test against the BuildMax agent to verify basic functionality.

## Usage

```
/smoke <level>
```

| Level | What it tests |
|-------|---------------|
| 0 | Agent can list its own tools |
| 1 | *(not yet implemented)* |
| 2 | *(not yet implemented)* |

Default level is **0** if omitted.

## Level 0 — Tool Awareness

1. Run `buildmax -p "What tools do you have?"` from the project root.
2. Verify the output contains these tool names:
   - Bash, Edit, Glob, Grep, Read, Skill, Task, TodoWrite, WebFetch, Write
3. **Pass** if all tool names appear. **Fail** if any are missing.

Report the result as:

```
SMOKE 0: PASS   (all <N> tools listed)
```

or

```
SMOKE 0: FAIL   (missing: <tool1>, <tool2>, ...)
```
