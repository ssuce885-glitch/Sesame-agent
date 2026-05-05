# Sesame V2 联调验收清单

本文档用于把 V2 从“代码能跑”收口到“可稳定联调、可提交、可长期迭代”。每次大改后按顺序执行，先确保仓库基线，再验证运行链路。

## 0. 仓库基线

- 先阅读 `docs/v2-workspace-organization.zh-CN.md`，确认本轮改动属于哪个提交边界。
- 确认 V1 删除是预期结果，不在 V2 收口过程中恢复旧实现。
- 确认 `internal/v2/`、Web 新页面、配置加载新文件都纳入版本控制。
- 确认 `roles/` 和顶层 `skills/` 没有进入代码仓库；它们是 workspace 本地业务资产，应通过本地配置、模板或安装流程准备。
- 确认临时产物没有进入仓库：数据库、日志、构建产物、测试 workspace。
- 执行质量检查：

```bash
go test ./...
go vet ./...
staticcheck ./...
git diff --check
```

通过条件：

- 所有命令退出码为 0。
- `git status --short` 中只剩预期的正式改动。
- 没有未解释的 untracked 代码文件。

## 1. Fresh Workspace 启动

构建当前 V2 入口：

```bash
go build -o /tmp/sesame-v2 ./cmd/sesame
```

使用全新的数据目录启动：

```bash
rm -rf /home/sauce/project/Workspace/sesame-fresh/.sesame
/tmp/sesame-v2 \
  -workspace /home/sauce/project/Workspace/sesame-fresh \
  -data-dir /home/sauce/project/Workspace/sesame-fresh/.sesame
```

通过条件：

- TUI 能进入默认 session。
- daemon 日志写入 `<data-dir>/sesame-v2.log`。
- `GET /v2/status` 返回 200。
- 首次启动能自动创建或加载默认 session。

非交互 smoke 可先验证 daemon 和基础 API，不发模型请求：

```bash
tmp=$(mktemp -d /tmp/sesame-v2-smoke.XXXXXX)
addr=127.0.0.1:18421
mkdir -p "$tmp/workspace"
/tmp/sesame-v2 -daemon \
  -workspace "$tmp/workspace" \
  -data-dir "$tmp/workspace/.sesame" \
  -addr "$addr" >"$tmp/daemon.log" 2>&1 &
pid=$!

curl -fsS "http://$addr/v2/status"
curl -fsS "http://$addr/v2/roles"
kill "$pid"
rm -rf "$tmp"
```

建议 smoke 继续覆盖：

- `POST /v2/roles`
- `GET /v2/roles/{id}`
- `PUT /v2/roles/{id}`

## 2. Main Agent 对话链路

验证项：

- TUI 发送普通消息，用户消息立即显示。
- assistant 流式输出实时显示，不需要退出重进。
- 多轮连续 turn 能保持 session 历史。
- 中断后 turn 状态变为 `interrupted`。
- 重启 daemon 后 running turn 会恢复为 `interrupted`，不会卡住 active session。

需要观察的接口：

- `GET /v2/sessions/{id}`
- `GET /v2/sessions/{id}/timeline`
- `GET /v2/events?session_id=<id>`
- `POST /v2/turns/{id}/interrupt`

## 3. Web 实时渲染

验证项：

- Web 首屏能加载默认 session。
- 发送消息后不用刷新即可看到 user/assistant/tool/event block。
- 页面刷新后 timeline 与 TUI 一致。
- `context_compacted`、`context_microcompacted`、tool error、turn failure 都有可读展示。
- 网络断开或 SSE 重连时不会重复渲染同一批事件。

通过条件：

- Web 和 TUI 对同一 session 的最终消息一致。
- SSE `afterSeq` 能补齐刷新前后的事件。

## 4. Role 创建和编辑

验证项：

- `POST /v2/roles` 能创建角色。
- `PUT /v2/roles/{id}` 能更新角色。
- 写入路径为：
  - `roles/<role_id>/role.yaml`
  - `roles/<role_id>/prompt.md`
- 校验覆盖：
  - role id
  - name
  - system prompt
  - model
  - tool permissions
  - path permissions
  - skill list

通过条件：

- API、Web、role tool 三个入口读取到同一份角色定义。
- 无效 role 不会写入半成品文件。

## 5. Role 运行观测

验证项：

- main agent 能创建 specialist role task。
- task 状态按 `pending -> running -> completed/failed/interrupted` 推进。
- 可读取 task trace：
  - input
  - assistant output
  - tool call
  - tool result
  - error
  - final report
- running 中的 role task 也能被探查。

需要观察的接口：

- `GET /v2/tasks`
- `GET /v2/tasks/{id}`
- `GET /v2/tasks/{id}/trace`
- `POST /v2/tasks/{id}/cancel`
- `GET /v2/reports`

通过条件：

- main agent 不需要轮询等待 role 完成。
- role 完成后通过 report delivery 回到 main agent。
- 失败任务能看到明确错误和最后的 trace。
- 自动化事务链已由 `go test ./internal/v2/automation -run TestAutomationRoleReportTransaction` 覆盖：
  watcher `needs_agent` -> owner role task -> specialist session -> task trace -> delivered report -> main `report_batch` turn。
- automation / workflow 联调还需确认：
  - `POST /v2/automations` 与 `GET /v2/automations` 会返回 `workflow_id`。
  - `Automation.workflow_id` 为空时仍走 direct owner task。
  - `Automation.workflow_id` 非空且 watcher 返回 `needs_agent` 时，会触发同 workspace workflow，并把 `workflow_run_id` 写入 automation run。
  - workflow 不存在、workspace 不匹配或 workflow trigger service 缺失时，automation run 会记录 `error` 审计项。

## 6. 上下文和长期项目能力

验证项：

- 普通短对话不会每轮压缩。
- 接近阈值时触发自动 compact。
- 大 tool result 触发 microcompact，但数据库原文不被改写。
- Project State 按阈值更新，不每轮调用模型。
- `GET /v2/context/preview?session_id=<id>` 能返回 prompt preview、已注入 context blocks、可用 memory/report blocks。
- `GET/POST/PUT/DELETE /v2/context/blocks` 能创建和维护 workspace-scoped context index，不改写原始 messages/reports/memories/project_state。
- compact 后继续多轮对话不丢目标、决策、打开事项。

通过条件：

- timeline 能看到压缩事件。
- Web Context 页面能看到 Context Inspector，且 Project State 显示为 included，Memory/Report 显示为 available 或后续策略选中状态。
- compact summary 会进入后续 model context。
- snapshot/raw segment 可追溯。

## 7. SQLite 和并发稳定性

## 7. Workflow 审计模型

验证项：

- `POST /v2/workflows` 能创建 workflow 定义，服务端生成 `id` 并绑定当前 workspace。
- `GET /v2/workflows?workspace_root=<path>` 能列出当前 workspace 的 workflow。
- `PUT /v2/workflows/{id}` 能更新定义，但不会改写 `id`、`workspace_root`、`created_at`。
- `POST /v2/workflows/{id}/trigger` 能创建并执行 workflow run；只接受当前 daemon workspace 下、`trigger` 为空或 `manual` 的 workflow。
- `POST /v2/workflow_runs` 必须引用已存在 workflow，并继承该 workflow 的 workspace。
- `GET /v2/workflow_runs?workspace_root=<path>&workflow_id=<id>&state=<state>` 能按 workflow 和状态过滤 run 审计记录。
- `PUT /v2/workflow_runs/{id}` 能更新状态、task/report/approval/trace 字段，但不会换绑 workflow 或 workspace。
- `POST /v2/approvals` 必须引用已存在 workflow run，并继承该 run 的 workspace。
- `GET /v2/approvals?workspace_root=<path>&workflow_run_id=<id>&state=<state>&limit=<n>` 能按 run 和状态过滤审批记录。
- `GET /v2/approvals/{id}` 和 `PUT /v2/approvals/{id}` 必须遵守当前 daemon workspace boundary。
- automation watcher 命中 `needs_agent` 且绑定了 `workflow_id` 时，workflow run 的 `trigger_ref` 形如 `automation:<automation_id>:<dedupe_key>`，并能回溯到对应 automation run。

通过条件：

- `trigger` MVP 串行执行 `role_task` step，并直接复用现有 `tasks.Manager`、specialist session、report delivery。
- `steps` 同时兼容 JSON 数组和 `{ "steps": [...] }` 对象包装；step 允许 `kind/type == role_task` 和 `approval`。
- run 状态至少覆盖 `queued -> running -> waiting_approval -> completed/failed/interrupted`，且 `trace`、`task_ids`、`report_ids`、`approval_ids` 会回写到 `WorkflowRun`。
- executor 遇到 `approval` step 时会创建 `pending` Approval，写入 `approval_requested` trace，把 run 停在 `waiting_approval`，本轮不继续执行后续 step。
- automation run 在 workflow 路径下会写入 `workflow_run_id`，`status` 记录为 `workflow:<state>`；旧的 direct task 路径继续使用 `task_id`。
- 客户端传入的 `id` 和跨 workspace 字段不会覆盖服务端边界。
- 后续执行器接入时可以从 run 记录追溯 trigger、task、report、approval 和 trace。
- `examples/workflows/` 已提供首批官方模板和 `README.zh-CN.md`，可直接复用到 Web Console / API 录入，但不会自动加载到 runtime。

Web Console 验收项：

- Sidebar 出现 `Workflows` 导航，能进入独立 workflow 工作台页面。
- 页面左侧能列出当前 workspace 的 workflow，至少显示名称、最近状态、trigger、owner role、更新时间和 steps 摘要。
- 页面右侧能创建或编辑 workflow，保存前至少要求 `name` 和 `steps` 非空；`steps` 支持用 owner role + prompt 生成 `role_task` JSON 模板。
- 选中 workflow 后能查看最近 20 次运行记录，且 `completed` / `failed` / `interrupted` 都能看到 `state`、`task_ids`、`report_ids`，以及 `trace` 的结构化事件视图（`event/state/kind/task_id/approval_id/message/time`）。
- manual workflow 可通过 Web Console 触发 `POST /v2/workflows/{id}/trigger`，默认 `trigger_ref=manual:web`，触发后运行记录能刷新并显示。

## 8. SQLite 和并发稳定性

验证项：

- 连续发起多个 turn 不出现 `database is locked`。
- role task 和 main agent 同时写事件/消息不会互相阻塞过久。
- daemon 重启后不会留下永久 running task。
- cancel queued/running task 都能收敛到终态。
- 若任务涉及 shell 子进程，Unix 环境下应验证进程组被一起终止；非 Unix 当前只要求主进程 best-effort 取消，不假设同等强度的子进程清理。

通过条件：

- 压测或重复手工操作中没有 SQLITE_BUSY 泄漏到用户界面。
- 所有 running 状态都能恢复、取消或完成。

## 9. 发布前判定

V2 可以进入稳定迭代的最低条件：

- 仓库基线检查全绿。
- Fresh workspace 从零启动通过。
- TUI 和 Web 对话链路都通过。
- Role 创建、编辑、执行、trace、report 链路通过。
- 上下文压缩至少有一次人工或测试触发验证。
- 已知失败路径都有可读错误，而不是静默卡住。
