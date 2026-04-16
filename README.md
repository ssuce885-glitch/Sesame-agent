# Sesame

Sesame 是一个本地终端 agent runtime。

当前主线只有一个产品模型：

- 一个 workspace
- 一个总助手
- 一个 canonical session
- 一个当前 context head

不再提供多 session 切换，也不再暴露 daemon 历史选择。

## 当前能力

- 终端 TUI / REPL 对话
- 自动连接或拉起本地 runtime
- 工作区绑定的 canonical session
- shell、文件、搜索、补丁、任务、权限、skills 等工具
- task-backed 自动化派发
- workspace mailbox / cron / incident / automation runtime
- 权限中断后的恢复执行

## 启动

要求：

- Go `1.24+`
- 已配置模型，配置文件路径：`~/.sesame/config.json`

直接运行：

```bash
go run ./cmd/sesame
```

查看状态：

```bash
go run ./cmd/sesame --status
```

假模型 smoke test：

```bash
SESAME_MODEL_PROVIDER=fake SESAME_MODEL=fake-smoke SESAME_PERMISSION_PROFILE=trusted_local go run ./cmd/sesame
```

## 运行语义

- 启动时默认进入当前 workspace 的 canonical session
- TUI 关闭时会停掉当前前台 turn，不让 session 在后台偷偷继续跑
- 自动化优先拉起 task，由 task 负责结果、mailbox 投递和后续收口

## 目录

- `cmd/sesame`: CLI 入口
- `internal/`: runtime、daemon、engine、tools、storage
- `docs/superpowers/specs`: 当前保留的设计文档
- `docs/superpowers/plans`: 当前保留的实施计划

## 当前保留文档

- `docs/superpowers/specs/2026-04-16-runtime-subtraction-cleanup-design.md`
- `docs/superpowers/specs/2026-04-16-single-session-reopen-and-context-head-design.md`
- `docs/superpowers/plans/2026-04-16-runtime-subtraction-cleanup-implementation.md`
- `docs/superpowers/plans/2026-04-16-single-session-reopen-and-history-implementation.md`

## 后续规划

- 完成单 session 下的“重开上下文”和“加载以前的聊天历史”
- 继续把自动化链路收敛到 task-first runtime
- 增加外部接入面，例如 Discord，但不改变单总助手模型
- 继续删除和现状不一致的旧表面、旧文档、旧兼容逻辑
