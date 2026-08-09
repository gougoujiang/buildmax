# BuildMax Documentation

This directory is the documentation set for BuildMax. It is organized by the
question a reader is trying to answer.

| If you want to know… | Go to | What it contains |
|---|---|---|
| What the project is and how to run it | [../README.md](../README.md) | Product overview, components, build and run instructions |
| What we are building next | [../ROADMAP.md](../ROADMAP.md) | Active near-term roadmap and priority order |
| How the system works today | [architecture/](architecture/) | Per-subsystem reference for the current codebase |
| Why it is built this way | [design/](design/) | Current design documents behind the roadmap |
| How it used to work | [archive/](archive/) | Superseded designs, kept for history and rationale |
| How to contribute | [../CONTRIBUTING.md](../CONTRIBUTING.md) | Development checks, boundaries, pull request guidance |
| How to report a vulnerability | [../SECURITY.md](../SECURITY.md) | Disclosure process and operator responsibilities |

## Document Types

**Architecture reference** (`architecture/`) describes the system as it is
implemented right now. Each document covers one package or subsystem and should
be updated in the same change that moves a package boundary or alters a
runtime contract.

**Design documents** (`design/`) are numbered records of a decision or a planned
piece of work: the problem, the options considered, the chosen approach, and the
implementation phases. A design document is written before or alongside the
work, and it keeps its number for life so that code comments and other documents
can cite it stably (for example, `docs/design/032-sandbox-and-execution-boundaries.md`).

**Archive** (`archive/`) holds design documents that no longer describe the
current direction. They are never edited to match new behavior; they are moved
here so the reasoning behind a past decision remains inspectable.

## Conventions

- Design documents are named `NNN-kebab-case-title.md`. Numbers are allocated
  once and never reused, including after a document is archived.
- A design document that is completed, superseded, or no longer describes the
  active direction moves to `archive/` rather than being deleted or rewritten.
- When behavior described in `architecture/` changes, update that document in
  the same pull request. When direction changes, add or archive a design
  document instead.
- Documentation is written in English so it is readable by all contributors.
- Cite documents by repository-relative path so links survive being moved
  between contexts.
