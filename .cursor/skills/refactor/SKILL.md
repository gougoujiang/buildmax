---
name: refactor
description: Analyzes recent implementations and code structure for code smells and refactor opportunities. Produces a written refactor proposal with detailed explanations and writes it to a file. Use when the user asks for refactor review, code smell detection, refactor opportunities, or to propose refactors.
---

# Refactor Review

## When to use

Apply this skill when:
- The user asks for a **refactor review**, **code smell detection**, or **refactor opportunities**
- They want to look at **recent implementations** or **code structure** and get concrete proposals
- They ask to **list** or **write down** refactor proposals with detailed explanation

This skill does **not** implement the refactors; it analyzes, proposes, and writes the proposal document.

## Workflow

1. **Scope**
   - Determine what to analyze: recent changes (e.g. from a task or PR), a package, or paths the user specified.
   - If unspecified, focus on recently modified or added code under `internal/` and `cmd/`.

2. **Inspect**
   - Read the relevant files and call graphs.
   - Note: package boundaries, coupling, duplication, naming, error handling, test coverage, and alignment with AGENTS.md (Go-only, `internal/` layout).

3. **Detect smells**
   - Look for: long functions, large types, duplicated logic, unclear names, deep nesting, missing interfaces for testability, mixed concerns, dead or redundant code.
   - For each smell: note **where** (file/line or symbol) and **why** it’s a problem.

4. **Proposals**
   - For each refactor opportunity, write one **proposal** with:
     - **Title**: Short name (e.g. "Extract X", "Introduce interface Y").
     - **Location**: File(s) and symbols.
     - **Current state**: What the code does now and why it’s a smell.
     - **Proposed change**: What to do (extract, rename, split, introduce interface, etc.).
     - **Benefit**: Readability, testability, maintainability, or consistency.
     - **Priority** (optional): High / Medium / Low if ordering matters.

5. **Write proposal file**
   - Produce a single markdown file using the [proposal template](template.md).
   - Default path: `tasks/refactor-proposal-YYYYMMDD.md` or a path the user specifies.
   - Fill: scope, summary, list of proposals with the details above, and optional "Suggested order" or "Out of scope".

## Proposal file structure

Use [template.md](template.md). Summary:

| Section | Purpose |
|--------|---------|
| **Scope** | What was analyzed (paths, packages, or "recent changes"). |
| **Summary** | Brief overview of findings (e.g. "5 proposals, 2 high priority"). |
| **Proposals** | One subsection per proposal: title, location, current state, proposed change, benefit, priority. |
| **Suggested order** | Optional: in which order to apply refactors if they depend on each other. |
| **Out of scope** | Optional: refactors considered but excluded and why. |

## Rules

- **Propose, don’t implement.** The output is the proposal file only; no code changes unless the user asks to implement a specific proposal afterward.
- **Be specific.** Each proposal must point to concrete locations (file, function, or type) and describe the change in enough detail to implement later.
- **Explain the smell.** For each proposal, state why the current code is a problem (maintainability, testability, clarity, duplication, etc.).
- **Respect project conventions.** Proposals should align with AGENTS.md (Go-only, `internal/` layout, no new runtime deps) and existing style.
- **One file per run.** Write exactly one proposal document per invocation; use the template so the format is consistent.

## Output

- **Primary**: One markdown file (e.g. `tasks/refactor-proposal-YYYYMMDD.md`) containing scope, summary, and numbered proposals with location, current state, proposed change, benefit, and optional priority.
- **Optional**: Short summary in chat (number of proposals, file path, high-priority items) so the user knows where to look.
