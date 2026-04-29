# Sesame TUI & Setup Flow — Architecture & Technical Reference

> Generated 2026-04-29. Covers `internal/cli/`.

## Overview

| Property | Detail |
|----------|--------|
| Language | Go |
| TUI Framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Widgets | [Bubbles](https://github.com/charmbracelet/bubbles) `textarea`, `viewport` |
| Plain-text fallback | `internal/cli/render/` (Unicode box-drawing) |
| Setup flow | Interactive terminal wizard (`internal/cli/setupflow/`) |
| Daemon lifecycle | `internal/cli/daemon/manager.go` |

**Key architectural trait**: The TUI has its own mirror type system (`tui/` types are independent copies of `internal/types`), connected via `tuiClientAdapter` that converts data at the boundary.

---

## File Inventory

```
internal/cli/
├── app.go                            # CLI entry point + App struct
├── options.go                        # Flag parsing (all commands)
├── automation.go                     # Script-command JSON output
├── skills.go                         # Skill subcommand handler
├── setup.go                          # Glue → setupflow.Run()
├── console_encoding.go               # UTF-8 console (shared)
├── console_encoding_other.go         # No-op on Linux/macOS
├── console_encoding_windows.go       # Windows UTF-8 code page
├── client/
│   └── client.go                     # HTTP client to daemon
├── daemon/
│   └── manager.go                    # Daemon lifecycle (launch/stop/poll)
├── render/
│   ├── renderer.go                   # Plain-text REPL renderer
│   ├── cron.go                       # Cron list/detail formatting
│   └── tool_display.go              # Tool argument → human-readable display (shared)
├── repl/
│   ├── repl.go                       # Main REPL runner (TUI vs line-by-line)
│   ├── tui.go                        # Bubble Tea entry point
│   ├── tui_adapter.go                # Type conversion layer (client → TUI types)
│   └── tui/
│       ├── model.go                  # Model struct (all state)
│       ├── types.go                  # Mirror types (Event, Turn, etc.)
│       ├── init.go                   # Constructor + Init() commands
│       ├── update.go                 # Update() dispatcher + View() assembly
│       ├── stream.go                 # SSE Streamer (batching, reconnect)
│       ├── cmdqueue.go               # tea.Cmd constructors + applyEvent()
│       ├── commands.go               # Slash command handlers
│       ├── entries.go                # Entry management (append/upsert/stream)
│       ├── layouter.go               # Header/body/footer layout
│       ├── style.go                  # Lip Gloss styles (NightOwl palette)
│       ├── view_chat.go              # Chat view rendering
│       ├── view_cron.go              # Cron view rendering
│       ├── view_reports.go           # Reports view rendering
│       ├── view_subagents.go         # Subagents/runtime view rendering
│       └── update_test.go            # Tests
└── setupflow/
    ├── flow.go                       # Main flow orchestrator
    ├── state.go                      # Flow state + vendor presets
    ├── mapping.go                    # Vendor → provider mapping
    ├── input.go                      # Input primitives (arrow choose, text, secret, bool)
    ├── discord.go                    # Discord integration setup
    ├── vision.go                     # Vision provider setup
    └── config_writer.go              # Config file writer
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `sesame` (default) | REPL/TUI interactive mode |
| `sesame --status` | Print daemon status and exit |
| `sesame --print "msg"` | Submit one prompt and exit |
| `sesame --version` | Print version |
| `sesame daemon` | Run daemon process |
| `sesame setup` | Setup wizard |
| `sesame configure` | Same as setup |
| `sesame skill list/inspect/install/remove` | Skill management |
| `sesame automation apply/list/get/pause/resume/remove/install/reinstall/watcher` | Automation management |
| `sesame trigger emit/heartbeat/watch` | Trigger management |

### Default flags
| Flag | Description |
|------|-------------|
| `--workspace` | Workspace root (default: cwd) |
| `--data-dir` | Data directory override |
| `--model` | Model override |
| `--permission-mode` | Permission profile override |

---

## TUI Architecture

### Model (State)

```go
Model {
    // Dependencies
    ctx, client       // context + RuntimeClient (HTTP to daemon)
    catalog           // Skill catalog
    
    // Session
    sessionID         // Active session
    lastSeq           // Latest event sequence number
    status            // Daemon StatusInfo
    
    // Layout
    width, height     // Terminal dimensions
    viewport          // Bubbles viewport.Model
    textarea          // Bubbles textarea.Model
    ready             // Whether initial load is complete
    
    // Content
    entries           // []Entry (chat log)
    activeView        // ViewChat | ViewSubagents | ViewReports | ViewCron
    streaming         // Active streaming entry
    streamingBatcher  // Coalesced delta text buffer
    
    // View caches
    reports, cronJobs, runtimeGraph
    reportingOverview
    
    // Auto-refresh
    lastActivityTime, lastRefreshTime
}
```

### Rendering Pipeline

```
Model.Update(msg) → Model changes
    → Model.View() calls:
        renderHeader()  → title bar + tab bar + metadata
        renderBody()    → renderViewportContent() → per-view renderer
        renderFooter()  → hint + report push bar + textarea
    → Viewport content string set via viewport.SetContent()
```

**Key pattern**: On any state change, the **entire viewport content is regenerated** as a single string. No incremental updates. The viewport auto-scrolls for chat, stays at top for other views.

### Views

| View | Constant | Tab Label | Rendered by |
|------|----------|-----------|-------------|
| Chat | `ViewChat` | Chat | `renderChatContent()` |
| Runtime | `ViewSubagents` | Subagents | `renderSubagentsContent()` |
| Reports | `ViewReports` | Reports | `renderReportsContent()` |
| Cron | `ViewCron` | Cron | `renderCronContent()` |

### Key Bindings

| Key | Action |
|-----|--------|
| `Ctrl+C` | Quit |
| `Ctrl+D` | Quit (textarea empty only) |
| `Esc` | Interrupt running turn |
| `Tab` | Next view |
| `Shift+Tab` | Previous view |
| `Enter` | Submit prompt or slash command |
| `Alt+Enter` | Newline in textarea |
| `Up/Down` | Scroll viewport |
| `PgUp/PgDown` | Half-page scroll |
| `Home/End` | Jump to top/bottom |

### Slash Commands

```
/help             Show help
/clear            Start new context
/exit             Quit
/status           Show daemon status
/skills           List installed skills
/tools            List available tools
/history          List context history
/history load <id> Load past context
/reopen           Reopen current context
/reports          Show reports
/chat             Switch to Chat view
/agents           Switch to Subagents view
/subagents        Switch to Subagents view
/cron list        List cron jobs
/cron inspect <id> Job detail
/cron pause <id>  Pause a cron
/cron resume <id> Resume a cron
/cron remove <id> Delete a cron
```

---

## SSE Streaming (TUI)

The TUI's event stream has a **33ms batching window** for `assistant.delta` events:

```
SSE stream → tuiClientAdapter → tui.Event channel
    → Streamer.forwardEvents() loop
        → if delta: buffer in streamingBatcher
        → else: emit as tuiStreamEventMsg immediately
    → Every 33ms: flush batched deltas as single tuiStreamEventMsg
    → On stream close/error: reconnect after 1s
```

This avoids excessive TUI repaints from high-frequency delta events.

---

## Style System (NightOwl-inspired)

All in `style.go`. Constants:

```go
colorBg          = "17"    // #00001a deep background
colorSurface     = "18"    // #000022
colorBorder      = "24"    // #004466
colorText        = "15"    // white
colorTextMuted   = "248"   // #a8a8a8
colorTeal        = "43"    // teal (brand)
colorCoral       = "210"   // coral (user)
colorMint        = "121"   // mint (assistant)
colorAmber       = "215"   // amber (tools)
colorYellow      = "11"    // yellow (notices)
colorRed         = "9"     // red (errors)
```

All styles use Lip Gloss chainable method calls: `StyleTitle`, `StyleTabActive/StyleTabInactive`, `StyleEntryUser/Assistant/Tool/Notice/Error`, `StyleToolPanel`, `StyleSurfacePanel`, etc.

---

## Setup Flow

### Entry points
- `sesame setup` / `sesame configure` (explicit)
- Implicit on first run when `MissingSetupFields()` returns non-empty

### Flow Diagram

```
Run()
  ├── Ensure config files exist
  ├── MissingSetupFields() → if none + not explicit → return
  └── chooseHomeSection()
        ├── [1] Model Setup (Required)
        │     └── collectModelSetupFlowState()
        │           ├── Vendor: Anthropic | OpenAI | MiniMax | Volcengine | Fake | Custom
        │           ├── API key (secret input)
        │           ├── Model name
        │           ├── Base URL
        │           ├── Permission profile (default: trusted_local)
        │           └── Listen address (default: 127.0.0.1:4317)
        ├── [2] Third-Party Integrations
        │     └── runIntegrationsMenu()
        │           ├── Discord (bot token, guild, channel, allowed users)
        │           └── Vision (provider, key, model)
        └── [3] Save and Exit / Continue Startup
              └── config.MergeAndWriteUserConfig()
```

### Discord Setup Fields
- Bot token (inline or env var)
- Gateway intents, message content intent
- Guild ID, channel ID
- Allowed user IDs (required, comma-separated)
- Advanced: require mention, post ack, reply timeout, max input chars, attachments mode

### Input Primitives
- `chooseArrowOption()` — numbered list with default
- `readTextInput()` — label + default value
- `readSecretInput()` — masked (pre-filled shown as `your-key`)
- `readBoolChoice()` — Enabled/Disabled
- `readIntInput()` — number with validation
- `readRequiredCommaSeparatedList()` — reprompts until non-empty

### Vendors
| Vendor | Provider | Default Model |
|--------|----------|---------------|
| Anthropic | `anthropic` | `claude-sonnet-4-6` |
| OpenAI-compatible | `openai_compatible` | `gpt-4o` |
| MiniMax | `openai_compatible` | `minimax-m1` |
| Volcengine | `openai_compatible` | `deepseek-v4-pro` |
| Fake | `fake` | — |
| Custom Anthropic | `anthropic` | user-provided |
| Custom OpenAI | `openai_compatible` | user-provided |

---

## Plain-Text REPL Fallback

When stdin/stdout are not terminals (piped input, IDE terminal), the TUI is skipped:

- Unicode box-drawing: `╭ ╰ │ ✦ ◇ ◉ ◌ ◆ ◈`
- Prompt: `sesame ❯ `
- Line-by-line `bufio.Scanner` input
- All output written through `render.Renderer` to `io.Writer`
- Shared `render.ToolDisplay` for tool argument formatting

---

## TUI ↔ Daemon Connection

```
1. EnsureDaemon() → daemon.Manager.EnsureRunning()
     ├── Check GET /v1/status (poll until ready)
     └── If not running: find sesame binary, spawn `sesame daemon`

2. EnsureSession() → POST /v1/session/ensure
     └── Gets/creates session for workspace

3. StreamEvents() → GET /v1/session/events?after=<seq>&binding=<binding>
     └── SSE connection, reconnects on failure

4. SubmitTurn() → POST /v1/session/turns

5. InterruptTurn() → POST /v1/session/interrupt
```

### Auto-refresh
Every 5 seconds, the TUI reloads reports and (if stale or on Subagents view) the runtime graph and reporting overview.

### Session Headers
Every request sends:
- `X-Sesame-Context-Binding` (from `sessionbinding.DefaultContextBinding`)
- `X-Sesame-Session-Role` (from `SessionRoleMainParent`)
