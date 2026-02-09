# Design NNN - [Short Name]

## Goal

[One sentence: what this design achieves, from the task.]

## Modules

| Module (package) | Responsibility | Owns |
|------------------|----------------|------|
| **[e.g. internal/agent]** | [What this package does.] | [Types, interfaces, key files.] |
| **[e.g. internal/llm]** | [What this package does.] | [Types, interfaces, key files.] |

## Structure

**Directory / files**

- `path/to/package/` — [purpose]
  - `file.go` — [purpose]
  - `other.go` — [purpose]

**Main types and interfaces**

- **TypeName** (package): [role and key fields or behavior.]
- **InterfaceName** (package): [contract in one line; main methods.]

## Method design

| Receiver | Method | Signature | Responsibility |
|----------|--------|-----------|-----------------|
| **Agent** | Process | `(ctx, userMessage) (reply string, err error)` | [One line.] |
| **Client** | ChatWithTools | `(ctx, messages, tools) (content, toolCalls, err)` | [One line.] |

(Add rows for each key function. No full code; enough to implement.)

## How they work together

**Data/control flow**

1. [Step: who calls whom, with what data.]
2. [Step.]
3. [Step.]

**Dependencies**

- [Package A] depends on [Package B] for [what].
- [Package B] has no dependency on [Package A].

**Key data structures**

- [Struct or message type]: [who creates it, who consumes it, role in flow.]

## Changes for review

- **New**: [path or type/method name] — [one-line description.]
- **New**: [path or type/method name] — [one-line description.]
- **Modified**: [path or type/method name] — [what changes.]
- [Further bullets so the user can approve the delta.]
