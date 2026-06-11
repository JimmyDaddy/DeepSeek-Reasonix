# Slash Subagent Lifecycle

Status: **Implemented for the TUI**, with desktop text fallback for
hand-entered `/subagents` management. Desktop still does not implement the
interactive `/subagents` runtime browser in this slice. Existing desktop
slash-skill behavior, including entries such as `/explore`, remains unchanged.

This document records the current contract after the TUI implementation.
`/subagents show` is intentionally absent - it never shipped because the TUI's
interactive detail screen (Enter on any run in the picker) serves the same
purpose. Desktop keeps `/subagents` out of command discovery, but hand-entered
management commands fall back to controller notice text until a first-class
desktop runtime UI exists.

## Scope

Slash subagents are user-invoked skills whose frontmatter resolves to
`runAs=subagent`, for example:

```text
/scout inspect this code path
```

There are two separate concepts:

- **Model subagents** are spawned by tools such as `task` and return through the
  parent model's tool result.
- **Slash subagents** are started directly by the user, have controller-owned
  lifecycle state, and can be inspected from the TUI with `/subagents`.

Current frontend scope:

- TUI: supported.
- HTTP/SSE: receives typed events and can use textual management notices.
- Desktop: existing slash skills remain available; `/subagents` stays hidden
  from command discovery, while hand-entered management commands use
  controller-generated notice text until a first-class desktop renderer exists.

## Controller Model

The controller owns retained slash-subagent state in `SubagentRun`.

```go
type SubagentRun struct {
    ID        string
    Skill     string
    Alias     string
    Prompt    string
    Task      string
    StartedAt time.Time
    EndedAt   time.Time
    State     event.SubagentState
    Answer    string
    Err       string
    Events    []SubagentEvent
}
```

The public snapshots are:

```go
func (c *Controller) ListSubagents() []SubagentSummary
func (c *Controller) SubagentDetail(id string) (SubagentDetail, bool)
func (c *Controller) CancelSubagent(id string)
func (c *Controller) ClearSubagents(state string)
```

`SubagentEvent` is a controller-owned detail log capped at
`maxSubagentEvents = 200`. It retains full display text for replaying the TUI
detail transcript, including reasoning, answer text, tool dispatches, tool
results, notices, usage, and terminal errors.

## Lifecycle

Inline skills still run through the normal guarded parent turn.

For `runAs=subagent` skills:

1. The controller allocates a stable subagent ID, alias, and `SubagentRun`.
2. It emits `TurnStarted` with `event.Subagent` metadata.
3. Read-only/background-capable subagents run in a child goroutine.
4. Writer-capable subagents fall back to a guarded foreground subagent turn.
5. Child events are tagged with the subagent ID and appended to the retained
   detail log.
6. Completion updates the run state to `completed`, `failed`, or `canceled`.
7. Successful runs append the slash command and final child answer to the parent
   transcript only through the controller finalization path.

If a parent foreground turn is running when a child finishes, the child
completion is queued and flushed at a controller safe point before the parent
`TurnDone` event is emitted. Child goroutines must not directly mutate the
parent session.

Background slash subagents inherit static controller context such as active goal
and plan-mode framing, but they do not consume parent-turn one-shot queues such
as pending memory updates or completed background-job summaries. Those queues
remain reserved for the next parent turn.

## Event Contract

Slash subagent lifecycle reuses existing event kinds. It does not add dedicated
subagent event kinds.

```go
type Subagent struct {
    ID    string
    Skill string
    Alias string
    State SubagentState
    Error string
}
```

When `Event.Subagent` is present, the event belongs to that slash-subagent
lifecycle or output stream. Parent reducers must not treat those events as
normal parent assistant text.

Important event meanings:

- `TurnStarted + Subagent`: child run started.
- `Reasoning/Text + Subagent`: child reasoning or answer stream for detail
  replay.
- `ToolDispatch/ToolResult + Subagent`: child tool activity.
- `Message + Subagent`: final child answer for runtime/detail display.
- `Usage + Subagent`: child usage summary.
- `TurnDone + Subagent`: terminal child state; state must not remain `running`.

Notice strings are human-readable only. Frontends should not parse
`[subagent] ...` notice text for controller-originated state.

## `/subagents` Command

`/subagents` is the public task-center entrypoint for runtime slash-subagent
instances.

```text
/subagents
/subagents cancel <id-or-alias>
/subagents clear [completed|failed|canceled|all]
```

There is intentionally no `/subagents show` command. `show` never shipped, and
the TUI detail path is the supported inspection surface.

Reference resolution for `cancel` is controller-owned:

1. Exact subagent ID.
2. Unique alias, case-insensitive.

Ambiguous refs return a readable notice and do not pick an arbitrary run.

`/subagents` is no longer treated as a public text subcommand group in the TUI.
Bare `/subagents` opens the task center; explicit management commands such as
`cancel` and `clear` remain available for controller-backed, non-interactive
flows. `clear all` removes terminal retained runs only; running runs are
preserved. Clearing never mutates persisted chat history.

## TUI Behavior

Bare `/subagents` is intercepted by the TUI and opens an interactive runtime
browser backed by `ListSubagents()` and `SubagentDetail(id)`.

When users type `/subagents cancel ...` or `/subagents clear ...` in the TUI,
the command is delegated to the controller notice path instead of opening the
browser.

Live slash-subagent output in the main chat no longer uses a bottom-pinned
runtime panel. It is rendered as a structured block directly inside the main
transcript so child activity appears in conversational order with the parent
session.

Embedded live block behavior:

- A background-capable slash subagent inserts one transcript block when the
  controller emits its structured `Notice + Subagent` metadata.
- While the child is still running, the block defaults to a collapsed preview
  of recent reasoning / output lines.
- `Ctrl-O` toggles the most recently updated live block between collapsed and
  expanded state.
- When the child reaches a terminal state (`completed`, `failed`, `canceled`),
  the block auto-expands so the final answer or terminal error is shown in full.
- The main composer remains interactive while a background slash subagent runs;
  users can keep typing or start another foreground turn without waiting for the
  child to finish.

List view:

- Shows active and retained runs newest-first.
- `/` enters filter mode.
- While filtering, typed text narrows runs by alias, skill, or state.
- `Enter` leaves filter mode and keeps the current filter applied.
- `Backspace` edits the active filter; clearing the query restores the full
  retained list.
- `Up/Down` or `k/j` moves selection.
- `Enter` opens detail for the selected run.
- `c` cancels a running selected run.
- `r` refreshes the list.
- `Esc` leaves filter mode first when editing; otherwise `Esc` or `q` closes
  back to the main agent.

Detail view:

- Is an independent rendered screen, not scrollback appended under the main
  agent transcript.
- Is read-only and has no input box.
- Reuses the main subagent panel rendering path for reasoning blocks, answer
  markdown, tool calls, tool results, notices, usage, and terminal errors.
- Does not compress completed runs into a separate summary style; completed and
  running details replay the same event log.
- `Up/Down`, `j/k`, mouse wheel, `PgUp/PgDn`, `Home`, and `End` scroll inside
  the detail screen.
- `Esc` returns to the main agent without leaving rendered detail content in the
  main transcript.
- `Enter` is ignored in detail; users cannot reply from a subagent detail view.
- `Ctrl-D` keeps the normal quit behavior instead of being swallowed by the
  subagent browser.

The TUI must not re-queue Bubble Tea mouse messages from a view callback. Bubble
Tea already forwards mouse messages to `Update`; re-sending them can create an
infinite mouse-message loop and starve keyboard events.

## Desktop Behavior

Desktop does not implement the slash-subagent runtime browser in this slice.

The desktop app:

- Does not list `/subagents` in command autocomplete.
- Keeps pre-existing slash skills, including `runAs=subagent` skills such as
  `/explore`, visible in the slash menu.
- Routes hand-entered `/subagents` commands through the controller notice path,
  so bare `/subagents` lists retained runs in text and `cancel` / `clear`
  remain usable without the interactive browser.
- Does not expose `ListSubagents`, `SubagentDetail`, or `CancelSubagent` Wails
  bindings.
- Does not include a `subagent` field in `desktop/wire.go`.

Future desktop support should be added only with a first-class desktop runtime
surface and reducer separation equivalent to the TUI detail screen.

## Wire Contract

The HTTP/SSE wire layer serializes `event.Subagent` as:

```json
{
  "kind": "message",
  "text": "...",
  "subagent": {
    "id": "...",
    "skill": "scout",
    "alias": "Ellis",
    "state": "completed",
    "error": ""
  }
}
```

Desktop intentionally does not expose this field while desktop subagent UI is
disabled.

## Safety Rules

- Parent transcript merge order is deterministic and controller-owned.
- Child goroutines do not directly mutate the parent session.
- Successful child answers merge as a user slash command plus assistant answer.
- Failed and canceled child runs do not append a parent assistant answer.
- Background slash subagents must not drain parent-turn pending memory or
  completed-job context.
- `Cancel()` continues to target the foreground turn; `CancelSubagent(id)`
  targets a retained slash-subagent run.
- Writer-capable slash subagents must not run as unsafe background editors.
  Current behavior is foreground fallback through the guarded controller path.
- Tool-spawned model subagents keep existing `Tool.ParentID` behavior.

## Test Coverage

Controller:

- Successful, failed, and canceled subagent states.
- Full retained event text for detail replay.
- Final answer appended as a retained `Message` event when needed.
- Parent-turn interleaving and queued completion merge.
- Background slash subagents do not drain parent-turn pending memory or
  completed-job context.
- Clear/cancel ref handling and ambiguous alias handling.

TUI:

- Bare `/subagents` opens the picker without writing a notice into scrollback.
- Empty list still opens the picker.
- Explicit `/subagents clear ...` and `/subagents cancel ...` stay on the
  controller notice path instead of reopening the picker.
- Detail replay preserves reasoning, tool calls/results, answer text, usage,
  and terminal errors.
- Completed detail uses the same rendering path as running detail.
- Detail screen is height-bounded and scrollable.
- Keyboard and mouse scrolling work without starving `Esc` or `Ctrl-D`.
- Detail view is read-only and ignores `Enter`.
- Cancel affordance appears only for cancelable runs.

Desktop:

- Desktop command menu keeps existing slash skills while omitting `/subagents`
  management.
- Desktop slash-arg completion hides `/subagents`, and `/agents` has no
  management surface because the command is removed.
- Desktop submit path forwards hand-entered `/subagents` runtime-management
  commands to controller notice text instead of opening the TUI browser.

## Acceptance Criteria

- `/subagents show` is absent.
- TUI `/subagents` opens an interactive selectable runtime list.
- Selecting any run state, including `completed`, opens the independent detail
  screen.
- Detail replay does not lose styling for reasoning, tools, normal answer text,
  final answer, usage, or errors.
- Subagent detail has no reply input.
- `Esc` returns to the main agent and does not leave detail content in the main
  transcript.
- Desktop exposes no interactive `/subagents` runtime browser entrypoint until
  a dedicated desktop runtime UI is implemented.
- Desktop hand-entered `/subagents` still supports textual list / cancel / clear
  management through controller notices.

## Settings Shell Commands

Settings-defined slash commands are platform shell snippets. Reasonix appends
the raw slash arguments and executes the combined command with the platform
default shell:

- Windows: `cmd /c <command>`
- Other platforms: `sh -c <command>`

Reasonix preserves quoted user arguments when appending them, but it does not
translate POSIX shell syntax to Windows `cmd.exe` syntax. Cross-platform
commands should either use syntax accepted by both shells or be defined per
platform by the user's settings.
