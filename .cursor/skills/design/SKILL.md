---
name: design-system
description: Produces system design for a given task or requirement: modules, package structure, type and method design, and how components work together. Outputs a detailed design document for user review before implementation. Use when the user has a task spec or requirement and wants a design (modules, structure, APIs, flows) to review, or when they ask for system design or technical design.
---

# Design System

## When to use

Apply this skill when:
- The user has a task/requirement (e.g. from the interview skill or `tasks/NNN.md`) and wants a **design** to review before coding
- They ask for "system design", "technical design", "module design", or "how should this be structured"
- They want to see **what changes** (new packages, types, methods, flows) the requirement implies

This skill complements the interview skill: interview produces *what* to build and scope; design produces *how* it is structured and how parts interact.

## Design workflow

1. **Input**  
   Use the task spec or requirement as the source of truth. If given only a title or short description, infer scope from the codebase and project docs (AGENTS.md, existing `internal/` layout).

2. **Inspect codebase**  
   From the repo:
   - Existing packages under `internal/` and `cmd/`
   - Current types, interfaces, and call patterns
   - Where the new work fits and what it will extend or replace

3. **Produce design**  
   Output a single design document (e.g. `design/NNN-design.md` or in chat) using the [design template](template.md). Fill: modules, structure, method design, and how they work together.

4. **Summarize changes**  
   End with a concrete "Changes for review" section: new/edited packages, files, types, and methods so the user can approve before implementation.

## Design document structure

Use the structure in [template.md](template.md). Summary:

| Section | Purpose |
|--------|---------|
| **Goal** | One sentence: what this design achieves (from the task). |
| **Modules** | Packages/components: name, responsibility, and what they own (e.g. `internal/agent`: agent loop, tool interface). |
| **Structure** | Directory layout, new or changed files, main types and interfaces (names and roles). |
| **Method design** | Key functions/APIs: receiver, name, signature, and responsibility. No full code; enough to implement without guessing. |
| **How they work together** | Data/control flow: who calls whom, key data structures passed, and any sequences (e.g. "TUI sends message → agent.Process → LLM → tools → agent returns reply"). |
| **Changes for review** | Bullet list: new dirs/files, new or modified types and methods, so the user can review the delta. |

## Design rules

- **Align with task scope.** Only design what is in scope; call out "out of scope" where the task says so.
- **Respect project layout.** Follow AGENTS.md and existing `internal/` conventions (Go-only, no new runtime deps, package boundaries).
- **Interfaces over concrete types** where a component is injected or mocked (e.g. LLM client, tools).
- **One place per responsibility.** Assign each type/method a clear owner package; avoid cross-package duplication.
- **Method design = contract.** Signatures and responsibilities must be specific enough to implement and test (e.g. "Process(ctx, userMessage) (reply string, err error)").

## Output

- **Primary**: One markdown design document (e.g. `design/002-design.md`) with modules, structure, method design, flows, and "Changes for review".
- **Optional**: Short summary in chat (modules + main APIs + flow) so the user can confirm before implementation.
