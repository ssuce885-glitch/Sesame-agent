# V2 后端 Runtime Manifest

更新时间：2026-05-04

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
internal/v2/contextsvc     context preview and ContextBlock application service
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
- role `tool_policy` 第一轮接线：
  - `allowed_tools` / `denied_tools` 兼容保留。
  - `tool_policy.<tool>` 已支持 `allowed`、`timeout_seconds`、`max_output_bytes`、`allowed_commands`、`allowed_paths`、`denied_paths`。
  - runtime 已统一使用同一套 tool visibility / execution gate。
- tool metadata 第一轮接线：
  - `ToolDefinition.capabilities` / `risk`
  - `ToolResult` envelope 字段
  - `tool_policy_explain` runtime explain 工具
- specialist task runner
- shell/file/task output bounds
- task trace
- reports and queued report_batch delivery
- memory/project state APIs
- read-only context preview API: `GET /v2/context/preview?session_id=<id>`
- ContextBlock index API:
  - `GET /v2/context/blocks?workspace_root=<path>`
  - `POST /v2/context/blocks`
  - `PUT /v2/context/blocks/{id}`
  - `DELETE /v2/context/blocks/{id}`
- context preview and ContextBlock update semantics covered in `internal/v2/contextsvc`
- Workflow / WorkflowRun 最小可审计模型：
  - `GET /v2/workflows?workspace_root=<path>`
  - `POST /v2/workflows`
  - `GET /v2/workflows/{id}`
  - `PUT /v2/workflows/{id}`
  - `POST /v2/workflows/{id}/trigger`
  - `GET /v2/workflow_runs?workspace_root=<path>&workflow_id=<id>&state=<state>`
  - `POST /v2/workflow_runs`
  - `GET /v2/workflow_runs/{id}`
  - `PUT /v2/workflow_runs/{id}`
  - `GET /v2/approvals?workspace_root=<path>&workflow_run_id=<id>&state=<state>&limit=<n>`
  - `POST /v2/approvals`
  - `GET /v2/approvals/{id}`
  - `PUT /v2/approvals/{id}`
  - 手动 trigger 现已接入 workflow executor MVP，支持 `trigger == manual` 下的 `role_task` 串行执行，以及 `approval` step 进入 `waiting_approval` 并创建 Approval 审计记录。
- automation watcher dispatch：
  - `Automation.workflow_id` 为空时，保持原有 `needs_agent -> owner role task` 路径。
  - `Automation.workflow_id` 非空时，`needs_agent` 会触发同 workspace 的 workflow run，并在 automation run 中记录 `workflow_run_id` 与 `workflow:<state>` 审计状态。
- TUI timeline/reports

## 当前已知后续项

- `internal/v2/app` handler tests 已覆盖 role CRUD 和 context preview；仍可继续扩展到任务、自动化等更多路由。
- Automation 的真实业务联调仍需跑 `reddit_monitor` 场景。
- Workflow executor 目前已覆盖手动 trigger，以及 automation watcher 通过 `workflow_id` 触发的 `role_task` / `approval -> waiting_approval` 暂停；approval 决策后的 resume/cancel 编排仍留在后续阶段。
- Workflow async run 在 `queued` 启动窗口如果进程崩溃，后续仍需补 workflow-run supervisor/recovery 机制，避免 run 长时间停在非终态。
- shell/command cancel 在 Unix 下会对进程组发送 `SIGKILL`；非 Unix 当前仍是 best-effort 取消主进程，文档和验收不应假设同等强度的子进程清理保证。
- Web build 的大 chunk 警告属于前端提交边界，不归本组处理。

## Role `tool_policy` Schema

当前 role 文件可继续使用原有粗粒度字段：

```yaml
allowed_tools:
  - file_read
denied_tools:
  - shell
allowed_paths:
  - docs/*
denied_paths:
  - secrets/*
```

第一轮新增细粒度 `tool_policy`：

```yaml
tool_policy:
  shell:
    allowed: true
    allowed_commands:
      - go test
    timeout_seconds: 30
    max_output_bytes: 8192
  file_write:
    allowed_paths:
      - docs/reports/*
    denied_paths:
      - docs/reports/private/*
```

输入仍兼容布尔简写：

```yaml
tool_policy:
  shell: false
```

语义说明：

- `allowed_tools` / `denied_tools` 仍然生效，作为兼容层保留。
- `tool_policy.<tool>.allowed=false` 可显式关闭单个工具。
- role/tool path policy 使用 slash path glob 语义（等价于 `path.Match`）；`*` 不跨目录，`docs/*` 不会匹配 `docs/a/b.md`，且不支持递归 `**`。
- role create/update 与 role 读取阶段都会校验 glob；若 pattern 非法，返回 `ErrInvalidRole`。runtime 若遇到非法 pattern，也会保守 deny 并返回原因。
- file tools 会同时应用 role 级 `allowed_paths` / `denied_paths` 和 tool 级 path policy。
- `.git`、`.sesame`、`.env*` 为 protected path，并按大小写不敏感处理；`file_read` 直接拒读，`glob` 不返回匹配，`grep` 会跳过扫描，若显式把 `path` 指到 protected file/dir 则返回拒绝。
- 上述 protected path 与 role path policy 都会同时检查显示路径和 symlink 解析后的 realpath；`file_write` 若目标不存在，会按最近存在 parent 的 realpath 继续检查，避免通过 symlink parent 写入 protected 或未授权区域；`file_edit` 只编辑既有文件，因此按既有文件 realpath 检查。
- `tool_policy` 对外序列化统一输出 object 形式（如 `shell: { allowed: false }`）；输入仍兼容 `shell: false`。
- 当启用 `tool_policy.shell.allowed_commands` 时，shell 必须命中允许的完整命令/前缀，且后续参数不能包含 shell 控制或展开语义，例如 `;`、`&&`、`||`、`|`、backtick、`$` / `$(`、`>`、`<`、换行等。
