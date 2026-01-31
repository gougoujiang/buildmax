---
name: implement
description: Takes a detailed design document (from the design skill) and writes the code. Implements modules, types, methods, and flows as specified; produces runnable code and tests. Use when the user has an approved design (e.g. design/NNN-design.md) and wants implementation, or when they ask to implement from a design or write code from a design doc.
---

# Implement from Design

## When to use

Apply this skill when:
- The user has a **design document** (e.g. from the design skill, `design/NNN-design.md`) and wants the code written
- They say "implement this design", "write the code from the design", or "code it up"
- The design has been reviewed and the user wants implementation to start

This skill consumes the **design** output (modules, structure, method design, flows) and optionally the **task spec** (`tasks/NNN.md`) for acceptance criteria and test requirements.

## Implementation workflow

1. **Input**
   - **Required**: Design document with Goal, Modules, Structure, Method design, How they work together, and Changes for review.
   - **Optional**: Task spec (`tasks/NNN.md`) for deliverables, out-of-scope, and acceptance criteria.

2. **Resolve order**
   - Implement in dependency order: interfaces and types first, then constructors, then methods that use them. Follow "How they work together" for call direction.
   - Create new packages/files as in the design **Structure**; modify existing files only where the design says **Modified**.

3. **Write code**
   - Match **Method design** exactly: receiver, name, signature, and responsibility. No extra public API unless the design adds it.
   - Follow project conventions: AGENTS.md (Go-only, `internal/` layout), existing style in the repo (formatting, naming, error handling).
   - Prefer interfaces for dependencies (LLM client, tools, etc.) so tests can inject mocks.

4. **Tests**
   - Add tests as specified in the task spec or design (e.g. unit tests with mock LLM and mock tool). No real API calls in tests.
   - Tick acceptance criteria: each "Done when" item should be covered by code or test.

5. **Verify**
   - Build: `go build ./...`
   - Tests: `go test ./...`
   - Confirm acceptance list: each checkbox can be checked off.

## Rules

- **Design is the contract.** Implement only what appears in the design and task spec. Do not add features or refactors that are out of scope.
- **Match signatures.** Method names, parameters, and return types must match the design Method design table.
- **One place per responsibility.** Put each type and method in the package the design assigns; do not duplicate logic across packages.
- **No new dependencies** unless the design or task explicitly introduces them (AGENTS.md: no new runtime deps).
- **Tests are required** where the task or design says so (e.g. "Unit test with mock X"). Prefer table-driven tests for multiple cases.

## Output

- **Primary**: Code changes — new or modified files under the paths in the design (e.g. `internal/agent/`, `internal/llm/`), plus tests. All acceptance criteria addressable.
- **Optional**: Short summary in chat (files touched, main types/methods added, test status) so the user can confirm.
