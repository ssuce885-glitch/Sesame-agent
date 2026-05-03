# Sesame V2 联调验收清单

本文档用于把 V2 从“代码能跑”收口到“可稳定联调、可提交、可长期迭代”。每次大改后按顺序执行，先确保仓库基线，再验证运行链路。

## 0. 仓库基线

- 先阅读 `docs/v2-workspace-organization.zh-CN.md`，确认本轮改动属于哪个提交边界。
- 确认 V1 删除是预期结果，不在 V2 收口过程中恢复旧实现。
- 确认 `internal/v2/`、Web 新页面、配置加载新文件都纳入版本控制。
- 确认角色引用的 `skills/<name>/SKILL.md` 已纳入版本控制，fresh clone 不依赖本机私有技能目录。
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

## 6. 上下文和长期项目能力

验证项：

- 普通短对话不会每轮压缩。
- 接近阈值时触发自动 compact。
- 大 tool result 触发 microcompact，但数据库原文不被改写。
- Project State 按阈值更新，不每轮调用模型。
- compact 后继续多轮对话不丢目标、决策、打开事项。

通过条件：

- timeline 能看到压缩事件。
- compact summary 会进入后续 model context。
- snapshot/raw segment 可追溯。

## 7. SQLite 和并发稳定性

验证项：

- 连续发起多个 turn 不出现 `database is locked`。
- role task 和 main agent 同时写事件/消息不会互相阻塞过久。
- daemon 重启后不会留下永久 running task。
- cancel queued/running task 都能收敛到终态。

通过条件：

- 压测或重复手工操作中没有 SQLITE_BUSY 泄漏到用户界面。
- 所有 running 状态都能恢复、取消或完成。

## 8. 发布前判定

V2 可以进入稳定迭代的最低条件：

- 仓库基线检查全绿。
- Fresh workspace 从零启动通过。
- TUI 和 Web 对话链路都通过。
- Role 创建、编辑、执行、trace、report 链路通过。
- 上下文压缩至少有一次人工或测试触发验证。
- 已知失败路径都有可读错误，而不是静默卡住。
