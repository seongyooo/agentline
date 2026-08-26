# AgentView — Implementation Plan

## 1. Project Overview

AgentView is a terminal-based observability UI for AI coding agents.

The goal is **not** to create another code editor, Git client, or Claude Code clone.

The goal is to answer four questions at a glance:

1. What is the agent doing right now?
2. Where in the project is the agent working?
3. What is the current mission and overall progress?
4. Does the agent need my attention?

The primary initial target is Claude Code.

The architecture must remain extensible so that other coding agents such as Codex, Gemini CLI, or OpenHands can be supported later.

---

## 2. Product Philosophy

### Core principle

> Don't show the code. Show what the agent is doing.

AgentView should provide situational awareness rather than detailed implementation information.

Users already have:

- Claude Code for detailed interaction
- VS Code / IDEs for editing and inspecting code
- lazygit for detailed Git operations
- Claude Code statusline for token/cost information

AgentView should **not** duplicate these tools.

The product should feel closer to an **AI Agent Observatory** than a terminal IDE.

---

## 3. MVP UI

The initial UI should be intentionally minimal.

Conceptual layout:

```text
┌────────────────────────────────────────────────────────────┐
│ AGENTVIEW                           ● CLAUDE CODE  WORKING │
├───────────────────────────┬────────────────────────────────┤
│ PROJECT                   │ MISSION                        │
│                           │                                │
│ 📁 Assets                 │ Water Room Puzzle              │
│ ├─ 📁 Scripts         ●   │                                │
│ │  ├─ Core                │ ✓ Analyze system               │
│ │  ├─ Player              │ ✓ Implement valve              │
│ │  ├─ Puzzle          ●   │ ● Connect drainage             │
│ │  │  ├─ Valve.cs     ●   │ ○ Test                         │
│ │  │  └─ Drain.cs     ●   │                                │
│ │  └─ Rooms           ●   │ NOW                            │
│ ├─ 📁 Prefabs             │ Editing Drain.cs               │
│ ├─ 📁 Scenes              │                                │
│ └─ 📁 Materials           │ NEXT                           │
│                           │ Run tests                      │
├───────────────────────────┴────────────────────────────────┤
│ ACTIVITY                                                    │
│                                                            │
│ Reading WaterRoom.cs                                        │
│ Editing Drain.cs                                            │
│ Running tests                                               │
│                                                            │
├────────────────────────────────────────────────────────────┤
│ > Ask Claude Code...                          Ctrl+Enter    │
└────────────────────────────────────────────────────────────┘
```

This is a conceptual layout, not a strict pixel-perfect requirement.

The UI must adapt to terminal dimensions.

---

# 4. MVP Information Architecture

The UI should contain these major areas.

## 4.1 Header

Display:

- AgentView name
- Current agent
- Agent status

Example:

```text
AGENTVIEW                         ● CLAUDE CODE  WORKING
```

Possible states:

- WORKING
- WAITING
- DONE
- ERROR
- NEEDS INPUT

The status must be immediately recognizable.

---

## 4.2 Project Tree

Show the project's filesystem tree.

This is intentionally included.

However, this is **not** intended to replace VS Code's file explorer.

The tree should primarily communicate:

> Where is the agent currently working?

Example:

```text
📁 Assets
├─ 📁 Scripts
│  ├─ 📁 Core
│  ├─ 📁 Player
│  ├─ 📁 Puzzle       ●
│  │  ├─ Valve.cs     ●
│  │  └─ Drain.cs     ●
│  └─ 📁 Rooms        ●
├─ 📁 Prefabs
├─ 📁 Scenes
└─ 📁 Materials
```

Use visual indicators for activity.

Possible states:

- active
- recently modified
- recently accessed
- inactive

Do not rely exclusively on color.

Symbols should also communicate state.

The tree must remain readable for large projects.

Do not render every file in very large directories by default. Prefer sensible collapsing, filtering, or lazy expansion.

The project tree should support:

- expand/collapse
- scrolling
- selecting a directory/file
- highlighting active paths
- showing recent activity

---

# 5. Mission

Mission represents the user's high-level goal.

For MVP, do **not** attempt sophisticated AI reasoning.

Initially, the mission can be derived from the user's initial Claude Code prompt or manually configured.

Example:

```text
MISSION

Water Room Puzzle
```

Later versions may infer structured tasks:

```text
MISSION

Water Room Escape Puzzle

✓ Analyze water system
✓ Implement valve
● Connect drainage
○ Test escape condition
```

Do not implement advanced mission inference in the first MVP.

The architecture should allow it later.

### Mission design principle

Mission should answer:

> What larger thing is the agent trying to accomplish?

It should not simply repeat the current tool call.

---

# 6. NOW

This is one of the most important UI elements.

It answers:

> What is the agent doing right now?

Examples:

```text
NOW

Reading
WaterRoom.cs
```

```text
NOW

Editing
DrainSystem.cs
```

```text
NOW

Running
Unity tests
```

```text
NOW

Waiting for input
```

The current action should be visually prominent.

The UI should distinguish between:

- reading
- editing
- creating
- deleting
- running commands
- testing
- waiting
- thinking/processing
- completed
- error

Do not expose internal chain-of-thought.

"NOW" must only represent observable activity and concise summaries derived from it.

---

# 7. NEXT

NEXT should provide a lightweight indication of what is likely to happen next.

For MVP, it can be based on available observable information.

Do not attempt complex prediction.

If no reliable next action is known, display:

```text
NEXT

—
```

Never invent a future action.

Later versions may optionally infer a likely next action from the task context.

---

# 8. Activity

Show only recent activity.

Do **not** display the entire Claude Code conversation.

Keep approximately the last 5–10 meaningful events visible.

Example:

```text
ACTIVITY

14:32  Read     WaterRoom.cs
14:33  Edit     DrainSystem.cs
14:34  Edit     RoomManager.cs
14:35  Run      Unity tests
```

Activity should automatically update as new events arrive.

The user should eventually be able to:

- pause scrolling
- scroll backward
- resume live mode

Avoid filling the screen with noisy low-level events.

Prefer meaningful actions.

---

# 9. Claude Code Input

AgentView should eventually allow the user to interact with Claude Code directly.

The bottom area should provide:

```text
> Ask Claude Code...
```

The user should be able to enter a prompt and send it to the running Claude Code process.

However, this should **not** be implemented before the event/state architecture is working.

For the initial MVP, prioritize:

1. observing Claude Code
2. rendering state
3. filesystem visualization

Then implement direct input.

---

# 10. What AgentView Should NOT Become

Do NOT turn AgentView into:

- a full code editor
- a VS Code replacement
- a Git client
- a token/cost dashboard
- a full Claude conversation viewer
- a general terminal emulator
- a complex project management application

Avoid unnecessary UI.

Do not add features simply because they can be displayed in a TUI.

Every visible element should answer one of the core questions:

1. What?
2. Where?
3. Why?
4. What's next?
5. Do I need to intervene?

---

# 11. Architecture

Use a strict separation between event collection, state management, and rendering.

Recommended architecture:

```text
Claude Code
     │
     ↓
Event Collector
     │
     ↓
Agent Event Model
     │
     ↓
State Store
     │
     ├───────────────┐
     ↓               ↓
Project State     Agent State
     │               │
     └───────┬───────┘
             ↓
        TUI Renderer
```

The TUI must **not** directly depend on Claude Code-specific implementation details.

The application should be organized around stable internal interfaces.

---

# 12. Agent Adapter Architecture

Design the system so that Claude Code is only one possible agent backend.

Conceptually:

```text
Agent Adapter
├── Claude Code
├── Codex
├── Gemini CLI
└── OpenHands
```

Each adapter converts agent-specific events into a common internal event format.

Example internal event:

```json
{
  "type": "file_edit",
  "path": "Assets/Scripts/Puzzle/DrainSystem.cs",
  "timestamp": "...",
  "source": "claude-code"
}
```

Other examples:

```json
{
  "type": "file_read",
  "path": "Assets/Scripts/Rooms/WaterRoom.cs",
  "timestamp": "...",
  "source": "claude-code"
}
```

```json
{
  "type": "command_start",
  "command": "dotnet test",
  "timestamp": "...",
  "source": "claude-code"
}
```

```json
{
  "type": "agent_status",
  "status": "working",
  "timestamp": "...",
  "source": "claude-code"
}
```

The exact schema may evolve.

Keep the schema small, explicit, and extensible.

Do not leak provider-specific event formats into the rest of the application.

---

# 13. Event Collection

Use official/structured Claude Code mechanisms where possible.

Do **not** begin by scraping terminal text if structured events/hooks are available.

The collector should translate agent-specific events into normalized internal events.

The event layer should be testable independently from the TUI.

The event collector should handle:

- tool activity
- file reads
- file edits
- file creation/deletion
- command execution
- command completion
- agent status
- errors
- waiting/input-required states

Only collect information that is actually available.

Do not fabricate state.

---

# 14. State Model

Create a centralized application state.

Conceptually:

```text
AgentState
├── agent
├── status
├── currentAction
├── currentFile
├── mission
├── nextAction
├── recentActivity
└── activeFiles

ProjectState
├── root
├── fileTree
└── activityByPath
```

The renderer should receive state and render it.

Avoid putting business logic directly inside UI components.

Prefer:

```text
Events
   ↓
Reducer / State Updater
   ↓
Application State
   ↓
Renderer
```

This makes the application easier to test and later support multiple agents.

---

# 15. Filesystem Activity

The project tree should react to agent activity.

For example:

```text
Edit:
Assets/Scripts/Puzzle/Drain.cs
```

should update:

```text
Puzzle       ●
└─ Drain.cs  ●
```

Recent activity should decay over time.

A file that was edited 30 minutes ago should not look equally active as the file currently being edited.

Consider activity levels such as:

- CURRENT
- RECENT
- MODIFIED
- INACTIVE

The exact visual representation can be decided during implementation.

### Important

Do not turn the project tree into a second IDE explorer.

The goal is to show **activity and context**, not provide every file-management feature.

---

# 16. Git Integration

Git integration is optional for the first MVP.

If implemented, use Git only to enhance project awareness.

Useful information:

- modified files
- untracked files
- current branch

Do **not** build a Git client.

Do **not** implement commit/push/pull UI in the MVP.

lazygit already solves that problem.

Git information should support the AgentView concept rather than become a separate feature area.

---

# 17. Claude Code Launcher / Process Model

AgentView will eventually need to manage or attach to a Claude Code process.

The preferred user experience is:

```bash
agentview
```

Then AgentView starts or connects to Claude Code and presents the observability UI.

However, process management should be implemented only after the event pipeline works.

Do not tightly couple process lifetime to the TUI before the architecture is stable.

The application should eventually support:

```text
AgentView
    │
    ├── Agent process
    ├── Event collector
    └── TUI
```

The exact process strategy should be chosen after verifying the available Claude Code integration mechanisms.

---

# 18. Claude Code Input

The input box should eventually allow:

```text
> Ask Claude Code...
```

Behavior:

1. User types a prompt.
2. AgentView sends it to Claude Code.
3. Claude Code continues execution.
4. Agent events flow back into the collector.
5. State updates.
6. TUI updates.

The input UI should not become a second chat interface.

Do not render the entire assistant response inside AgentView.

AgentView is for situational awareness.

---

# 19. Technology

Preferred stack:

- Go
- Bubble Tea
- Lip Gloss
- Bubbles

Reason:

- strong TUI ecosystem
- cross-platform
- easy single-binary distribution
- suitable for open-source CLI tooling
- good terminal rendering and input handling

Use idiomatic Go.

Keep dependencies minimal.

Avoid introducing a large framework unless necessary.

---

# 20. Responsive TUI

The application must work at different terminal sizes.

Large terminal:

```text
┌───────────────┬───────────────────────────┐
│ PROJECT       │ MISSION / NOW / NEXT      │
├───────────────┴───────────────────────────┤
│ ACTIVITY                                  │
├───────────────────────────────────────────┤
│ INPUT                                     │
└───────────────────────────────────────────┘
```

Small terminal:

```text
┌──────────────────────┐
│ STATUS               │
├──────────────────────┤
│ MISSION              │
├──────────────────────┤
│ NOW                  │
├──────────────────────┤
│ ACTIVITY             │
├──────────────────────┤
│ INPUT                │
└──────────────────────┘
```

Do not allow panels to become unreadably small.

Prioritize information in this order:

1. Status
2. NOW
3. Mission
4. Project activity
5. Activity log
6. Project tree details
7. NEXT

If space becomes constrained, collapse or reduce lower-priority information.

---

# 21. Visual Design

The UI should feel:

- minimal
- professional
- calm
- information-dense but readable
- modern
- developer-oriented

Avoid:

- excessive borders
- excessive colors
- huge ASCII art
- unnecessary animations
- excessive icons
- visual clutter

Color should communicate state, not decoration.

Suggested semantic states:

- working
- success
- warning
- error
- inactive

But the UI must remain understandable in monochrome terminals.

Use consistent spacing and alignment.

Panels should have clear visual hierarchy.

Important information should have strong contrast.

Do not use color as the only source of meaning.

---

# 22. Interaction

Keyboard-first interaction is preferred.

Suggested initial shortcuts:

```text
↑ / ↓       Navigate
← / →       Expand/collapse or change focus
Enter       Select
Tab         Move between panels
Esc         Cancel / close popup
q           Quit
?           Help
Ctrl+C      Exit
```

Do not finalize the entire keymap until the basic UI exists.

The input field must support normal text editing.

Mouse support can be added later if useful.

---

# 23. Development Phases

Implement in the following order.

## Phase 1 — Project Skeleton

Create:

- Go module
- application entry point
- TUI skeleton
- state model
- event model
- basic logging/error handling

The program should launch successfully.

### Deliverable

A running terminal application with mock state.

---

## Phase 2 — Static TUI

Implement the layout using mock data.

Do **not** connect Claude Code yet.

Use mock state such as:

```text
Mission: Water Room Puzzle
Now: Editing DrainSystem.cs
Status: Working
```

Goal:

Make the UI visually polished and responsive.

Test multiple terminal sizes.

### Deliverable

A polished static TUI that demonstrates the intended product.

---

## Phase 3 — Filesystem Tree

Implement:

- project root detection
- filesystem scanning
- tree rendering
- expand/collapse
- scrolling
- active file highlighting

Do not implement advanced Git functionality yet.

### Deliverable

AgentView can display a real project structure.

---

## Phase 4 — Event Infrastructure

Implement:

- normalized event schema
- event dispatcher
- state reducer/updater
- mock event source

Example:

```text
mock event
    ↓
event dispatcher
    ↓
state update
    ↓
TUI
```

### Deliverable

The TUI reacts correctly to simulated events without Claude Code.

This is an important architectural checkpoint.

---

## Phase 5 — Claude Code Adapter

Implement the first Claude Code integration.

Use official/structured integration mechanisms where available.

Convert Claude events/hooks into the normalized event model.

Test independently from the UI.

Example:

```text
Claude event
    ↓
Claude Adapter
    ↓
Normalized Event
    ↓
State Store
    ↓
TUI update
```

### Deliverable

Real Claude Code activity appears in AgentView.

---

## Phase 6 — Live State

Connect real events to:

- NOW
- STATUS
- ACTIVITY
- active files
- project tree
- waiting/error states

At this point the application should already be genuinely useful.

### Deliverable

A developer can run Claude Code and observe its work through AgentView.

---

## Phase 7 — Claude Code Launcher / Input

Allow:

```bash
agentview
```

to launch/manage Claude Code.

Implement the input box:

```text
> Ask Claude Code...
```

The user should be able to send prompts without leaving AgentView.

Do this only after the observation pipeline is stable.

### Deliverable

AgentView becomes a practical entry point for Claude Code.

---

## Phase 8 — Mission

Add basic mission extraction from the initial user prompt.

For MVP, use the prompt itself or a lightweight deterministic extraction.

Do not use an additional LLM call yet.

Later, design an optional intelligence layer for:

- task decomposition
- progress estimation
- next-step inference
- mission summarization

### Deliverable

AgentView can display the high-level purpose of the current agent session.

---

## Phase 9 — Git Awareness

Optionally add:

- modified files
- untracked files
- branch

Keep it lightweight.

### Deliverable

Git state enhances project awareness without becoming a Git UI.

---

# 24. Future Intelligence Layer

Do not implement this in the first MVP.

Eventually, AgentView may transform low-level events:

```text
Read WaterRoom.cs
Edit DrainSystem.cs
Edit RoomManager.cs
Run tests
```

into higher-level context:

```text
MISSION
Water Room Escape Puzzle

PROGRESS
68%

CURRENT PHASE
Connecting drainage system

NEXT
Verify escape condition
```

This may require an optional LLM-based analysis layer.

If implemented, it must:

- be optional
- clearly separate inferred information from observed information
- never pretend an inference is a fact
- minimize unnecessary API cost
- work without an LLM whenever possible

Use explicit labels such as:

```text
INFERRED
```

when appropriate.

---

# 25. Needs You / Attention State

This is a high-priority future feature.

AgentView should make moments requiring human attention visually obvious.

Examples:

```text
⚠ NEEDS YOU

Claude Code is waiting for permission.
```

```text
⚠ NEEDS YOU

A command failed.
```

```text
✓ DONE

Mission completed.
```

The purpose is:

> The developer should not have to constantly watch the terminal.

This feature should eventually allow the user to leave AgentView running and only return when something important happens.

For MVP, implement only if the underlying agent events reliably expose these states.

---

# 26. Multi-Agent Support

Do not implement initially.

The architecture should support it later.

Potential agents:

- Claude Code
- Codex
- Gemini CLI
- OpenHands
- Aider
- other compatible coding agents

The common interface should remain:

```text
Agent Adapter
      ↓
Normalized Events
      ↓
Common State Model
      ↓
AgentView TUI
```

Never make the UI depend on a single provider's terminology if avoidable.

---

# 27. Testing

Every major component should be testable independently.

Especially:

- event parsing
- event normalization
- state transitions
- filesystem tree
- activity decay
- terminal resizing
- process lifecycle

The TUI should not require Claude Code to be running in order to run unit tests.

Use mock events for testing.

Example:

```text
file_read
file_edit
file_create
file_delete
command_start
command_end
agent_working
agent_waiting
agent_done
agent_error
```

Test state transitions such as:

```text
working → waiting
working → done
working → error
waiting → working
```

---

# 28. Error Handling

AgentView must fail gracefully.

Examples:

- Claude Code unavailable
- Claude process exits
- hook/event malformed
- project directory inaccessible
- terminal too small
- event source disconnected

The TUI should remain usable where possible.

Do not crash the entire application because a single event is malformed.

Log diagnostic information separately from the main UI.

---

# 29. Performance

AgentView should be lightweight.

Avoid:

- repeatedly rescanning the entire project every frame
- excessive filesystem polling
- unnecessary subprocess creation
- excessive rendering
- storing unlimited activity history in memory

Use event-driven updates where possible.

For filesystem activity, prefer targeted updates rather than full-tree rescans.

Large repositories must remain usable.

---

# 30. Security and Privacy

AgentView may observe:

- file paths
- commands
- prompts
- agent activity

Do not transmit any of this information externally by default.

The MVP should operate locally.

If a future LLM-based intelligence layer is added, clearly disclose what information is sent externally.

Do not log sensitive prompts or source code unnecessarily.

---

# 31. Documentation

Create a concise README containing:

- what AgentView is
- why it exists
- screenshots/GIF when available
- installation
- usage
- supported agents
- architecture overview
- roadmap
- contribution instructions

Do not write the README as marketing copy before the product actually works.

---

# 32. Open Source Design

Keep the repository clean and approachable.

Recommended structure:

```text
agentview/
├── cmd/
│   └── agentview/
├── internal/
│   ├── agent/
│   ├── events/
│   ├── state/
│   ├── project/
│   └── tui/
├── tests/
├── docs/
├── go.mod
├── go.sum
├── README.md
├── LICENSE
└── IMPLEMENTATION.md
```

The exact structure may change if a better idiomatic Go layout is appropriate.

Do not create unnecessary abstractions.

---

# 33. MVP Definition of Done

The MVP is successful when a developer can:

1. Start AgentView in a project.
2. Start/use Claude Code through it.
3. See whether Claude is working, waiting, done, or needs input.
4. See the file Claude is currently interacting with.
5. See recent agent activity.
6. See the project's filesystem structure.
7. See which parts of the project are currently active.
8. Send a prompt to Claude Code.
9. Resize the terminal without breaking the UI.

Most importantly:

> A developer should understand what the AI is doing within approximately 1–2 seconds of looking at the screen.

That is the primary UX goal.

---

# 34. Development Rules for Claude Code

When implementing this project:

### Rule 1 — Do not build everything at once

Complete one phase at a time.

After each phase:

1. build
2. run tests
3. manually verify
4. review the architecture
5. only then continue

### Rule 2 — Do not over-engineer

Prefer the simplest implementation that satisfies the current phase.

Do not implement speculative abstractions.

### Rule 3 — Keep provider-specific code isolated

Claude Code-specific behavior belongs in the Claude adapter.

The TUI should know nothing about Claude-specific event formats.

### Rule 4 — Keep UI and business logic separate

Do not put event processing, filesystem scanning, or state mutation directly inside rendering code.

### Rule 5 — Preserve the product philosophy

Whenever adding a feature, ask:

> Does this improve the developer's understanding of what the AI agent is doing?

If not, do not add it to the MVP.

### Rule 6 — Do not duplicate existing tools

Do not add:

- code editing
- full Git management
- token/cost analytics
- full chat transcript

These are outside AgentView's core purpose.

### Rule 7 — Prefer observable facts

Do not invent what the agent is doing.

If something is inferred, distinguish it from observed state.

---

# 35. Final Product Principle

AgentView is not:

> "A prettier Claude Code."

AgentView is:

> **A situational-awareness layer for AI coding agents.**

The core information hierarchy is:

```text
MISSION
   ↓
WHERE
   ↓
NOW
   ↓
NEXT
   ↓
ACTIVITY
   ↓
NEEDS YOU
```

Everything else is secondary.

The user should be able to glance at AgentView and immediately understand:

> **"What is my AI doing, where is it working, what is it trying to accomplish, and do I need to intervene?"**

That is the product.
