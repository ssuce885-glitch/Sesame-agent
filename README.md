# Sesame

Sesame 是一个本地终端 agent runtime。

## 当前主线

- 一个 workspace
- 一个总助手
- 一个 canonical session
- 一个 canonical session 下的多 context head，按入口 binding 选择当前 head
- 后台自动化统一走 task，不再把 turn 当成后台主语义
- 运行态状态默认收进 workspace 的 `.sesame/`

不再提供多 session 切换，也不暴露 daemon 历史选择。

## 当前能力

- TUI / REPL 对话
- 自动连接或拉起本地 runtime
- workspace 绑定的 canonical session
- shell、文件、搜索、补丁、任务、权限、skills 等工具
- workspace mailbox / cron / incident / automation runtime
- task-backed 自动化派发
- 单 session 下的 context history 查看、history load、reopen
- 不做意图识别或 per-turn profile 路由
- 模型默认看到当前 runtime 可用的全部内置工具，执行时再由权限和 schema 约束兜底

当前命令面里和 context head 相关的入口：

- `/history`
- `/history load <head_id>`
- `/reopen`

语义：

- `reopen` 会新建一个空 lineage 的 context head
- `history load` 会新建一个 parent 指向历史 head 的 context head
- 后续 timeline 和模型上下文只读取当前 binding 的 current head lineage
- 终端默认 binding 是 `terminal:default`
- 后续外部入口（例如 Discord channel/thread）应使用各自 binding，共享 daemon，但默认不共享聊天上下文

## 启动

要求：

- Go `1.24+`
- 已配置模型，配置文件路径：`~/.sesame/config.json`

运行态默认路径：

- 全局配置仍在 `~/.sesame/`
- 当前 workspace 的 session、history、task、mailbox、automation runtime 默认在 `<workspace>/.sesame/`

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

## 当前行为

- 启动时默认进入当前 workspace 的 canonical session
- 当前 workspace 默认使用 `<workspace>/.sesame/sesame.db`
- task / todo 默认写入 `<workspace>/.sesame/sesame.db`
- task 输出日志默认写入 `<workspace>/.sesame/tasks/`
- workspace 背景状态以 workspace 维度观察，不再依赖“选中哪个 session”
- 自动化优先拉起 task，由 task 负责结果、mailbox 投递和后续收口
- 不再按 `web_lookup`、`automation`、`scheduled_report` 之类模式裁剪模型工具箱
- 权限 profile 仍影响执行审批，但不再影响模型“能不能看到工具”
- CLI / TUI 退出后，daemon 目前默认仍然保持运行

## 代码目录

- `cmd/sesame`: CLI 入口
- `internal/`: runtime、daemon、engine、tools、storage
- `docs/superpowers/specs`: 保留的主线设计文档

## 保留文档

- `docs/superpowers/specs/2026-04-15-runtime-spine-refactor-design.md`

## 后续重点

- 继续删除和现状不一致的旧表面、旧兼容逻辑、旧文档
- 把 daemon / session 生命周期再收紧，减少“退出界面但后台还活着”的不透明感
- 增加外部接入面，例如 Discord，但不改变单总助手模型
