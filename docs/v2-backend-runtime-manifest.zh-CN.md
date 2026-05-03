# V2 后端 Runtime Manifest

更新时间：2026-05-03

## 结论

V2 后端 runtime 已形成独立包组，可作为第二个提交边界审查。

验证结果：

```bash
go test ./internal/v2/...
```

通过。

临时标记检查：

```bash
rg -n "TODO|FIXME|HACK|temporary|placeholder|sample|panic\\(" internal/v2
```

结果：没有命中。

## 包结构

```text
internal/v2/agent          agent turn loop, context budget, compaction, project state refresh
internal/v2/app            daemon wiring and HTTP routes
internal/v2/automation     watcher execution and automation dispatch service
internal/v2/client         TUI HTTP client
internal/v2/contracts      behavior-free V2 DTOs and service interfaces
internal/v2/memory         workspace memory service
internal/v2/observability  in-process metrics collector
internal/v2/reports        task report creation and queued report_batch delivery
internal/v2/roles          role filesystem service and role snapshots
internal/v2/schema         SQLite schema migrations
internal/v2/session        session queue manager and stable session IDs
internal/v2/store          SQLite repositories
internal/v2/tasks          task manager, runners, output sink, task trace
internal/v2/tools          model-facing tools and role/tool policy gates
internal/v2/tui            terminal UI model, rendering, stream handling
```

## 建议提交

提交信息：

```text
add v2 runtime backend
```

建议包含：

- `internal/v2/**`

不要混入：

- V1 删除
- Web Console 迁移
- `cmd/sesame` 入口切换
- workspace roles/skills
- 文档整理

## 当前已覆盖能力

- daemon HTTP server
- session queue
- append-only messages/events
- agent turn loop
- context micro-compaction and threshold compaction
- project state refresh
- role CRUD
- role tools
- role prompt/model/budget/policy wiring
- specialist task runner
- shell/file/task output bounds
- task trace
- reports and queued report_batch delivery
- memory/project state APIs
- automation watcher dispatch
- TUI timeline/reports

## 当前已知后续项

- `internal/v2/app` handler tests 可继续扩展到更多路由。
- Automation 的真实业务联调仍需跑 `reddit_monitor` 场景。
- Web build 的大 chunk 警告属于前端提交边界，不归本组处理。
