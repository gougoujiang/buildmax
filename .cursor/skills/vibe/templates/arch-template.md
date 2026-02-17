# Architecture Refactor Proposal — [Date or Scope Name]

## Scope

**Analyzed**

- Paths / scope: [e.g. `internal/`, full codebase, or "store + agent layer"]
- Criteria: [e.g. "package boundaries and dependencies" or "layering and module coupling"]

## Summary

- **Opportunities**: [count]
- **High impact**: [count]
- **Overview**: [One or two sentences on main structural findings.]

---

## Opportunities

### 1. [Short title, e.g. "Introduce clear store boundary"]

**Scope**: [Packages, layers, or subsystems involved — e.g. `internal/store`, `internal/agent`]

**Current state**

- [How the code is structured today: boundaries, dependencies, coupling.]
- [Why it's an architectural concern: unclear ownership, circular deps, wrong layering, etc.]

**Proposed change**

- [Target structure: new package, interface boundary, dependency direction, or split/merge.]
- [Concrete steps or target module layout if helpful.]

**Benefit**: [e.g. clearer boundaries, testability, scalability, reduced coupling]

**Impact**: High | Medium | Low

---

### 2. [Next opportunity]

**Scope**: …

**Current state** / **Proposed change** / **Benefit** / **Impact**

(Repeat for each opportunity.)

---

## Suggested order

(Optional. If refactors depend on each other, list the order to apply them.)

1. [Opportunity 1 title] — do first because …
2. [Opportunity 2 title] — then …

## Out of scope

(Optional. Structural changes considered but excluded and why.)

- [e.g. "Full rewrite of TUI layer — deferred to roadmap."]
