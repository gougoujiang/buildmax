# Smell Proposal - [Date or Scope Name]

## Scope

**Analyzed**

- Paths / packages: [e.g. `internal/tui/`, `internal/agent/` or "recent changes from task 013"]
- Criteria: [e.g. "all files modified in last N commits" or "package X and its callers"]

## Summary

- **Proposals**: [count]
- **High priority**: [count]
- **Overview**: [One or two sentences on main findings.]

---

## Proposals

### 1. [Short title, e.g. "Extract message formatting"]

**Location**: `path/to/file.go` — [function/type name or line range]

**Current state**

- [What the code does now.]
- [Why it's a smell: duplication, length, mixed concern, unclear name, etc.]

**Proposed change**

- [What to do: extract function, rename, introduce interface, split type, etc.]
- [Concrete steps or target structure if helpful.]

**Benefit**: [e.g. readability, testability, maintainability, consistency]

**Priority**: High | Medium | Low

---

### 2. [Next proposal]

**Location**: …

**Current state** / **Proposed change** / **Benefit** / **Priority**

(Repeat for each proposal.)

---

## Suggested order

(Optional. If refactors depend on each other, list the order to apply them.)

1. [Proposal 1 title] — do first because …
2. [Proposal 2 title] — then …

## Out of scope

(Optional. Refactors considered but excluded and why.)

- [e.g. "Large TUI refactor — deferred to separate task."]
