# go-agent

`go-agent` is a local Go daemon for agent runtime experiments. The runtime design borrows from the Claude Code reference project at `E:\project\claude-code-2.1.88`, but this repository stays intentionally smaller and focused on a deployable HTTP/SSE runtime core.

## Run a Fake Smoke Instance

```bash
set AGENTD_DATA_DIR=%CD%\data-smoke
set AGENTD_MODEL_PROVIDER=fake
set AGENTD_MODEL=fake-smoke
set AGENTD_PERMISSION_PROFILE=trusted_local
go run ./cmd/agentd
```

This is the quickest way to verify local daemon startup. The status endpoint will be available at `http://127.0.0.1:4317/v1/status`.

## Run Against a Responses API Provider

```bash
set AGENTD_DATA_DIR=%CD%\data
set AGENTD_MODEL_PROVIDER=openai_compatible
set AGENTD_MODEL=gpt-4.1-mini
set OPENAI_API_KEY=your-key
set OPENAI_BASE_URL=https://your-provider.example.com
set AGENTD_PROVIDER_CACHE_PROFILE=ark_responses
set AGENTD_CACHE_EXPIRY_SECONDS=86400
set AGENTD_MICROCOMPACT_BYTES_THRESHOLD=4096
set AGENTD_PERMISSION_PROFILE=trusted_local
go run ./cmd/agentd
```

For Ark Responses native cache, keep `AGENTD_PROVIDER_CACHE_PROFILE=ark_responses` so the daemon can use provider-native cache directives. If you set `AGENTD_PROVIDER_CACHE_PROFILE=none`, the daemon falls back to local-only compaction and does not try to use provider cache.

`trusted_local` allows these local tools:

- `file_read`
- `glob`
- `grep`
- `file_write`
- `shell_command`

If you want a safer default, omit `AGENTD_PERMISSION_PROFILE` and the daemon will stay on `read_only`.

## Run Against Anthropic

```bash
set AGENTD_DATA_DIR=%CD%\data
set AGENTD_MODEL_PROVIDER=anthropic
set AGENTD_MODEL=claude-sonnet-4-5
set ANTHROPIC_API_KEY=your-key
go run ./cmd/agentd
```

## Create a Session

```bash
curl -X POST http://127.0.0.1:4317/v1/sessions ^
  -H "Content-Type: application/json" ^
  -d "{\"workspace_root\":\"D:/work/demo\"}"
```

Creating a new session does not automatically change the selected task.

## List Sessions And Discover Default Focus

```bash
curl http://127.0.0.1:4317/v1/sessions
```

The response includes `selected_session_id`. On restart, the daemon keeps that selected session when possible; if older data has no saved selection yet, the daemon falls back once to the most recently updated session and persists that choice.

## Explicitly Switch Tasks

```bash
curl -X POST http://127.0.0.1:4317/v1/sessions/<session-id>/select
```

The selected session persists across daemon restarts.

## Submit a Turn

```bash
curl -X POST http://127.0.0.1:4317/v1/sessions/<session-id>/turns ^
  -H "Content-Type: application/json" ^
  -d "{\"client_turn_id\":\"turn-1\",\"message\":\"inspect this workspace\"}"
```

## Subscribe to SSE

```bash
curl -N http://127.0.0.1:4317/v1/sessions/<session-id>/events
```

## Check Runtime Status

```bash
curl http://127.0.0.1:4317/v1/status
```

The status payload reports the active provider, model, and permission profile without leaking secrets.
