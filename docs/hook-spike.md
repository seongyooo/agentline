# Spike: what Claude Code hooks actually deliver

Phase 5 assumes AgentView can observe Claude Code through structured events
rather than by scraping terminal text. The whole architecture rests on that
assumption, so it was tested before the adapter was written.

**Result: the assumption holds.** Hooks deliver more than the plan expected,
including the prompt text needed for MISSION. Four gaps matter for the adapter.

## Method

Two headless sessions (`claude -p`) ran in a throwaway project whose
`.claude/settings.json` registered capture hooks on nine events, using both
transports at once — a `command` hook writing stdin to a file, and an `http`
hook posting to a local server. Captures were compared for content and order.

- Environment: Claude Code 2.1.226, Windows 11
- Scenario 1 — read a file, edit it, run a command: 10 command / 6 http records
- Scenario 2 — run a failing command, attempt a denied write: 7 command records

## Mapping to the normalized event model

| AgentView event | Hook source | Field |
|---|---|---|
| `file_read` | `PostToolUse` where `tool_name` is `Read` | `tool_input.file_path` |
| `file_edit` | `PostToolUse` where `tool_name` is `Edit` | `tool_input.file_path` |
| `file_create` | `PostToolUse` where `tool_name` is `Write` | `tool_input.file_path` |
| `command_start` | `PreToolUse` on a shell tool | `tool_input.command` |
| `command_end` | `PostToolUse` on a shell tool | `duration_ms` |
| `agent_error` | `PostToolUseFailure` | `error`, `is_interrupt` |
| `agent_status: working` | `UserPromptSubmit`, any `PreToolUse` | — |
| `agent_status: done` | `Stop` | `last_assistant_message` |
| mission (Phase 8) | `UserPromptSubmit` | `prompt` |

`tool_use_id` appears on every tool event and correlates `PreToolUse` with its
`PostToolUse`, which is what lets a command's start and end be paired.

## Gaps the adapter has to handle

### 1. There is no exit code

`PostToolUse.tool_response` for a shell command carries `stdout`, `stderr` and
`interrupted` — no exit status. A failing command does not produce
`PostToolUse` at all; it produces `PostToolUseFailure`, with the code buried in
a human-readable string:

```json
{ "error": "Exit code 3\n3", "is_interrupt": false }
```

So success and failure are reliably distinguishable *by which event fires*, but
the numeric code is not a structured field. The adapter should set
`command_end` from the event type and leave `ExitCode` nil rather than parse
that string. `is_interrupt` separates a user interruption from a real failure.

### 2. The shell tool is not always called `Bash`

On Windows the tool arrives as `PowerShell`, not `Bash`. Matching the literal
string `Bash` would silently drop every command event on Windows — the kind of
bug that looks like "the feature just doesn't work here". The adapter must
treat the shell as a set of tool names.

### 3. Paths are absolute

`tool_input.file_path` is a full OS path with native separators:

```
C:\Users\...\spike-project\notes.md
```

The event model and the project tree both use root-relative slash paths, so the
adapter must relativize against the project root and normalize separators.
Paths outside the root have to be dropped or marked, not silently mangled.

### 4. "Needs you" is unverified

The plan's §25 attention state depends on `Notification` with
`notification_type: "permission_prompt"`. In scenario 2 a write was denied for
lack of permission and **neither `Notification` nor `PermissionRequest`
fired** — the sequence was `PreToolUse(Write)` then straight to `Stop`.

This is expected: headless mode auto-denies instead of prompting, so there is
no prompt to notify about. It means the signal is untested, not that it is
absent. Verifying it requires an interactive session, and §25 should stay
out of the MVP until it has been observed directly.

## Transport: use HTTP

Both transports worked. HTTP is the better fit:

- The TUI is already a running process; it can listen directly, with no file
  tailing, polling, or watcher in between.
- Delivery arrived in correct order, with `tool_use_id` intact.
- A `command` hook would still need some way to reach the running TUI, which
  means inventing a second channel anyway.

Two constraints come with it:

- **Respond immediately.** Hooks run inline with the agent's tool calls, so a
  slow handler adds latency to the agent's work. The handler should return
  `200` with an empty body — the "no decision" response — and do its work off
  the request path.
- **The port must be agreed in advance,** since the hook config has to name a
  URL before AgentView starts. A fixed default port with a flag to override is
  enough for the MVP; installing the hook config into a project is Phase 7
  launcher work.

## Incidental findings

- `SessionStart` reports the reason as `source` (e.g. `"startup"`), not
  `session_start_reason` as the documentation states.
- Every payload carries `session_id`, `cwd`, `transcript_path` and
  `permission_mode`; tool events add `effort` and `prompt_id`.
- `Stop` includes `last_assistant_message`, which AgentView must **not**
  render — the plan is explicit that it is not a conversation viewer.
