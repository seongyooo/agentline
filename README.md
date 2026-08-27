# AgentLine

[![test](https://github.com/seongyooo/agentline/actions/workflows/test.yml/badge.svg)](https://github.com/seongyooo/agentline/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/seongyooo/agentline.svg)](https://pkg.go.dev/github.com/seongyooo/agentline)
[![MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A terminal observability UI for AI coding agents.

AgentLine answers four questions at a glance:

1. What is the agent doing right now?
2. Where in the project is it working?
3. What is the current mission, and how far along is it?
4. Does it need your attention?

It is not an editor, a Git client, or a Claude Code clone. It shows **what the agent is
doing** — not the code it is doing it to.

---

## What it looks like

A real frame, rendered at 100×28 (colour stripped):

```text
AGENTLINE   asks first                                                             ● CLAUDE  WORKING
────────────────────────────────────────────────────────────────────────────────────────────────────
PROJECT ◂                              │ MISSION                                                    
                                       │ Add Git awareness to the header                            
Assets/                              ● │ ████████████░░░░░░░░░░░░ 2/4                               
├─ Scripts/                          ● │                                                            
│  ├─ Core/                            │ NOW                                                        
│  ├─ Player/                          │ Editing   3s                                               
│  ├─ Puzzle/                        ● │ .../Puzzle/DrainSystem.cs                                  
│  │  ├─ Valve.cs                      │                                                            
│  │  └─ DrainSystem.cs              ● │ NEXT                                                       
│  └─ Rooms/                         ◐ │ —                                                          
│     └─ WaterRoom.cs                ◐ │                                                            
├─ Prefabs/                            │                                                            
├─ Scenes/                             │ Opus 5 | Context: 31% used                                 
└─ Materials/                          │ 5h: 62% (reset 8/27 19:40) | 7d: 28% (reset 8/31 13:00)    
────────────────────────────────────────────────────────────────────────────────────────────────────
ACTIVITY                                                                                            
19:01  Working                                                                                      
19:02  Reading   .../Rooms/WaterRoom.cs                                                             
19:03  Running   Run the git package tests                                                          
19:04  Done      Run the git package tests                                                          
19:05  Editing   .../Puzzle/DrainSystem.cs                                                          
                                                                                                    
                                                                                                    
                                                                                                    
────────────────────────────────────────────────────────────────────────────────────────────────────
> Ask Claude Code...                                              enter inspect   tab focus   q quit
```

- **Header** — the agent, its status (`WORKING`, `WAITING`, `NEEDS INPUT`, `DONE`,
  `ERROR`) and the permission mode, coloured by how much the agent can do without asking.
- **PROJECT** — the filesystem tree, with activity markers (`●` current, `◐` fading) on
  the files and folders the agent has touched. Touched files are revealed automatically,
  and a file the agent has announced but not yet written is dimmed until it lands.
- **MISSION / NOW / NEXT / REPLY** — the goal derived from your prompt, the action in
  flight with its elapsed time and the agent's own description of it, and a scrollable
  box holding its answer. The bar under `MISSION` counts the agent's own task list, and
  appears only when it keeps one: AgentLine never estimates completion.
- **Session line** — the model, how full the context is, and the usage windows, written
  the way Claude Code's own status line writes them. Everything on it was reported by
  the agent; nothing is measured or estimated here.
- **ACTIVITY** — a timestamped log of recent observed actions, which ages out.
- **Prompt bar** — send a prompt to the session AgentLine owns, with the Git branch and
  the keys that apply.

The layout is responsive: it drops to a single column and sheds the lower-priority
sections (`NEXT`, then `REPLY`) before it will clip `NOW`.

---

## Install

No Go toolchain needed — these download a released binary.

**macOS and Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/seongyooo/agentline/main/install.sh | sh
```

**Windows**

```powershell
irm https://raw.githubusercontent.com/seongyooo/agentline/main/install.ps1 | iex
```

Or take the archive for your platform from the
[releases page](https://github.com/seongyooo/agentline/releases) and put the binary
somewhere on your `PATH`.

<details>
<summary>With Go, or from source</summary>

```sh
go install github.com/seongyooo/agentline/cmd/agentline@latest
```

Requires Go 1.26+ and puts `agentline` in `$(go env GOPATH)/bin`.

To work on it:

```sh
git clone https://github.com/seongyooo/agentline
cd agentline
go build ./cmd/agentline
```

</details>

---

## Usage

There are two ways to watch Claude Code, plus a way to try the interface for free.

### 1. Let AgentLine own the session (recommended)

```sh
agentline --run
```

AgentLine launches `claude` in streaming-JSON mode and keeps one process alive across
turns, so the prompt box is live and each prompt continues the same conversation.
Nothing has to be installed into the project, and nothing is left behind on exit.
Requires the `claude` executable on `PATH`.

### 2. Observe a Claude Code session you started yourself

AgentLine listens for Claude Code hooks on `127.0.0.1:8787` by default. Install the hook
settings into the project you want observed:

```sh
agentline --print-hooks          # merge the output into .claude/settings.json
agentline                        # then run Claude Code as usual
```

The settings are generated rather than documented, so the address can never drift from
what AgentLine is actually listening on. In this mode the prompt box is inert — the
session belongs to your terminal, not to AgentLine.

### 3. Try it without spending anything

```sh
go build -o fakeagent ./cmd/fakeagent
agentline --run --agent ./fakeagent --root /some/scratch/dir
```

`fakeagent` speaks the same streaming-JSON protocol on the same pipes, so this drives the
real adapter, the real process handling and the real control requests — only the thinking
is missing. It reads and writes the files it is pointed at for real, so the tree and the
activity log behave as they would during a session that costs money. Point it at a
scratch directory: it creates a file.

There is also `agentline --source mock`, which replays sample activity without launching
anything, but the prompt box is inert in that mode.

### Flags

| Flag | Default | Meaning |
|---|---|---|
| `--root` | detected from cwd | Project root |
| `--run` | `false` | Launch and own a Claude Code session |
| `--agent` | `claude` | Executable to run instead of `claude` |
| `--source` | `claude` | Which agent: `claude`, `codex`, or `mock` |
| `--addr` | `127.0.0.1:8787` | Address to receive hooks on |
| `--mission` | — | Pin `MISSION` instead of deriving it from prompts |
| `--print-hooks` | — | Print the hook settings to install, then exit |
| `--mock-interval` | `2s` | Delay between mock events |

Diagnostics go to `<user cache dir>/agentline/agentline.log`, never to the screen.

### Keys

Several panels scroll, so the arrow keys belong to one of them at a time. The focused
panel is marked `◂` in its heading.

| Key | Action |
|---|---|
| `tab` | Move focus between the tree, `REPLY`, `ACTIVITY` and the prompt |
| `↑` `↓` `pgup` `pgdn` | Scroll the focused panel |
| `→` `←` | Expand / collapse a folder (tree only) |
| `enter` | Describe the selected file or folder (tree only) |
| `esc` | Back out of a description or the prompt |
| `i` | Jump straight to the prompt |
| `shift+tab` | Cycle the permission mode |
| `ctrl+n` | Start a fresh session, clearing accumulated context |
| `q` / `ctrl+c` | Quit |

Click to move focus; the wheel scrolls whatever is under the pointer.

In the prompt box, `enter` sends and every other key is text. Typing `/` completes from
the commands the session announced; `/model` opens a picker.

---

## Supported agents

| Agent | How | Status |
|---|---|---|
| Claude Code | Owned session (stream-JSON), or hooks | Verified against real sessions |
| Codex | `codex exec --json` | Built to the published schema; **not yet run against a real session** |
| Gemini CLI | OTLP telemetry | Not implemented — see below |

```sh
agentline --run --source codex
```

Codex is a different shape to Claude Code: `codex exec` runs one turn and exits, so
AgentLine keeps the thread it reports and resumes it on the next prompt. The conversation
remembers, but there is no long-lived process. It reports a real exit code for commands
and says whether a patch added, updated or deleted each file, so those read more precisely
than the Claude adapter can manage.

The adapter is written against Codex's published event schema and is covered end to end by
a stand-in that speaks it, but nobody has yet pointed it at the real binary. Treat it as
untested until someone has.

**Gemini CLI** has no JSON or streaming output; its only structured channel is
OpenTelemetry over gRPC, which emits exactly the right events — `user_prompt`, `tool_call`,
`file.operation`. Receiving them means an OTLP/gRPC server and the protobuf definitions
that go with it, which is a large dependency for a project that currently has three. It is
feasible and mapped out, but not built.

Adding an agent means writing one adapter that translates its output into the normalized
event model. Nothing downstream of an adapter knows which agent it is watching.

---

## Architecture

```text
Claude Code ──stream-json──┐
Claude Code hooks ──HTTP───┼──► internal/agent/... ──► events.Event ──► state ──► tui
Codex ──────────────JSONL──┘        (adapters)          (normalized)   (reducer)
```

| Package | Responsibility |
|---|---|
| `cmd/agentline` | Flags, root detection, wiring, logging |
| `internal/agent` | The `Source` seam and a mock backend |
| `internal/agent/claude` | Claude Code adapter: hook server, owned session, translation |
| `internal/agent/codex` | Codex adapter: one process per turn, resumed by thread id |
| `internal/events` | The normalized, provider-neutral event model |
| `internal/state` | The reducer — the only place state changes |
| `internal/project` | Lazy filesystem scanner and tree model |
| `internal/git` | Branch and changed-file lookup |
| `internal/tui` | Bubble Tea model, layout, rendering |

The event model covers `file_read`, `file_edit`, `file_create`, `file_delete`,
`file_pending`, `command_start`, `command_end`, `agent_status`, `agent_error`,
`user_prompt`, `agent_reply`, `task_progress` and `session_info`. Adapters must not
populate a field they could not observe — AgentLine reports what it saw and says nothing
where it has nothing.

Owning a session also allows control requests back to the agent — the permission mode,
the model, and how full the context is. That wire format is not in the published SDK
documentation; it was read out of the CLI, so its exact shape is pinned by tests that
will say so if it ever drifts.

Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

---

## What AgentLine is not

It will not become an editor, a diff viewer, a Git client, a chat transcript, or a
dashboard of token spend. Those tools already exist and are better at their jobs.
`REPLY` shows a single line of the agent's answer — just enough to know the turn landed.

---

## Development

```sh
go test ./...
```

Some tests are opt-in because they need a real agent, a real terminal, or a large tree:

| Test | Enable with |
|---|---|
| Live Claude Code session and hooks | `LIVE_CLAUDE=<root>`, `LIVE_ADDR` |
| Observing a running session end-to-end | `OBSERVE_ROOT=<root>`, `OBSERVE_ADDR`, `OBSERVE_MISSION` |
| Frame-by-frame UI preview | `LIVE_PREVIEW=1` |
| Scanner benchmark on a real tree | `BENCH_ROOT=<root>` |

Everything else runs without an agent. The streaming adapter is covered against a
stand-in that speaks the protocol, so a change to it fails a test rather than quietly
breaking a session nobody is watching.

`IMPLEMENTATION.md` is the design document and the source of truth for scope.
`docs/hook-spike.md` records what Claude Code hooks actually deliver — measured, not
assumed.

---

## Roadmap

- **Needs You** — a distinct attention state for when the agent is genuinely blocked.
- **Deeper Git awareness** — beyond the current branch and changed files.
- **Intelligence layer** — inferring `NEXT` and summarising phases from observed events.
- **Multi-agent support** — a second adapter, which is the real test of the seam.

---

## Contributing

The rules that keep this project from turning into a terminal IDE:

1. Build one phase at a time; every phase should run.
2. Do not over-engineer — no abstraction without a second implementation in sight.
3. Keep provider-specific code inside its adapter.
4. Keep UI and business logic separate; the reducer owns state.
5. Preserve the product philosophy: situational awareness, not detail.
6. Do not duplicate tools that already exist.
7. Prefer observable facts to estimates. When AgentLine does not know, it says nothing.

Read `IMPLEMENTATION.md` before opening a PR, and keep `go test ./...` green. CI runs the
tests with the race detector on Linux, macOS and Windows — paths, process handling and
terminal widths all differ between them, so all three have to pass.

Issues and pull requests are welcome. If you are adding support for another agent, the
adapter seam is `internal/agent.Source`; `internal/agent/claude` is the worked example
and `docs/hook-spike.md` shows how its behaviour was established.
