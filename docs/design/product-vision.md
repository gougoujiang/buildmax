# Agent-Mediated AI-Native Workspace

**Product direction · current**
*Long-range design reference for the BuildMax AI-native workspace.*

---

## 1. High-Level Idea

An **AI-native workspace** where:

- Users express **intent**, not tool operations.
- AI agents operate on a **versioned, text-based workspace**.
- All state is stored as structured text (Markdown, CSV, JSON, YAML).
- Every change is reversible.
- Git acts as the hidden state engine.
- Containers provide isolated execution environments.

**Paradigm shift:**

| Traditional | AI-Native |
|-------------|-----------|
| Human → UI → Tool → Data | Human → Agent → Workspace → Versioned State |

Work shifts from **operating software** to **expressing goals**.

---

## 2. Key Concepts

### 2.1 Text as a First-Class Citizen

All core artifacts are text-based and machine-readable:

- **Reports** → Markdown  
- **Data** → CSV / JSON  
- **Plans** → Markdown  
- **Config** → YAML  
- **Logs** → Text  

**Rationale:** Diffable, versionable, explainable, AI-native.

### 2.2 Agent-Mediated Interaction

**Agent** = LLM + Tools + Workspace + Control Loop.

**Core loop:** Observe → Plan → Act → Observe

The agent:

- Reads workspace state
- Modifies files
- Executes code
- Generates artifacts
- Commits changes

The user does **not** interact with files directly.

### 2.3 Workspace Model

| Term | Definition |
|------|------------|
| **Workspace** | Persistent context container |
| **Project** | Executable work unit |
| **Task** | Single agent execution |

**Hierarchy:**

```
Workspace
├── Project
│   ├── Tasks
│   ├── Artifacts
│   ├── Commits
│   └── Container Runtime
└── Memory (long-term context)
```

### 2.4 Versioned Reality (Git as Infrastructure)

Git is **internal only** — hidden from the user.

For every task, the system:

- Creates a snapshot
- Records changes
- Generates a semantic change summary
- Supports restore

**Principles:** Write access by default; reversible by design.

**User sees:** Activity timeline, what changed, restore option.  
**User does not see:** commit, branch, merge, diff.

### 2.5 Container Runtime

Each project runs inside an isolated container with:

- Controlled execution
- Restricted command policy
- Resource limits

The agent operates only within the workspace boundary.

---

## 3. Work Model

### 3.1 Traditional Model

The user:

- Opens multiple tools
- Exports/imports data
- Runs manual steps
- Copies results
- Formats reports

**Work** = Tool operations.

### 3.2 AI-Native Model

**User:** States a goal.

**Agent:**

- Gathers context
- Executes operations
- Generates artifacts
- Commits state
- Explains impact

**Work** = Intent → State transformation.

### 3.3 Example: Sales Analysis Flow

**User says:** “Prepare this month’s sales analysis.”

**System:**

1. Loads workspace context  
2. Imports CSV data  
3. Cleans data  
4. Runs analysis  
5. Generates Markdown report  
6. Creates charts  
7. Records change  
8. Returns summary  

**User sees:** Key metrics, insights, timeline entry, restore option.

---

## 4. Design Principles

1. **Intent first** — Goals over operations.  
2. **Text is the primary representation** — All core state is text.  
3. **State is versioned** — Every change is tracked and reversible.  
4. **Mechanisms hidden, meaning visible** — Git/containers are implementation details.  
5. **Write access by default, reversible by design** — Users can act freely and undo.  
6. **Workspace is the agent’s body** — The agent lives in and acts on the workspace.  
7. **UI is visualization, not state** — The UI reflects workspace state; it does not own it.

---

## 5. Core Architecture Summary

```
User
  ↓
Conversation Agent (Intent → Plan)
  ↓
Operator Agent (Execute in Container)
  ↓
Workspace (Text-Based State)
  ↓
Git State Engine (Hidden)
  ↓
Timeline + Semantic Change Narrative
```

---

## 6. Mental Model for Users

Users should feel:

- “I describe what I want.”
- “The system understands my work.”
- “It remembers context.”
- “I can always go back.”
- “I don’t need to manage tools.”

They should **not** feel:

- “I am operating software.”
- “I am editing files.”
- “I am managing versions.”

---

## 7. UI Wireframe — Landing Page

Landing experience: single prompt area and recent activity.

```
┌──────────────────────────────────────────────────────────────┐
│  🧠 Nexus                                    ⚙️  Profile    │
│  Workspace: Sales Team                                        │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│              What would you like to accomplish?              │
│                                                              │
│      ┌──────────────────────────────────────────────────┐   │
│      │ Help me prepare this month's sales analysis       │   │
│      └──────────────────────────────────────────────────┘   │
│                                                              │
│                         [  Run  ]                            │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│  Recent Activity                                             │
│                                                              │
│  • Generated sales report (Today 10:42 AM)                   │
│  • Updated February revenue data                             │
│  • Created pricing draft                                     │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

---

*End of reference. Prototype name: Nexus (or replace as needed).*
