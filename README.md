# go-agent

## Start the daemon

```bash
set AGENTD_DATA_DIR=%CD%\data
set ANTHROPIC_API_KEY=your-key
go run ./cmd/agentd
```

If you want to use an OpenAI-compatible streaming backend, set:

```bash
set AGENTD_MODEL_PROVIDER=openai_compatible
set OPENAI_API_KEY=your-key
set OPENAI_BASE_URL=https://your-provider.example.com
set AGENTD_MODEL=gpt-4.1-mini
go run ./cmd/agentd
```

## Create a session

```bash
curl -X POST http://127.0.0.1:4317/v1/sessions ^
  -H "Content-Type: application/json" ^
  -d "{\"workspace_root\":\"D:/work/demo\"}"
```

## Submit a turn

```bash
curl -X POST http://127.0.0.1:4317/v1/sessions/<session-id>/turns ^
  -H "Content-Type: application/json" ^
  -d "{\"client_turn_id\":\"turn-1\",\"message\":\"inspect this workspace\"}"
```

## Subscribe to SSE

```bash
curl -N http://127.0.0.1:4317/v1/sessions/<session-id>/events
```
