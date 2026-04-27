# Sesame

[English](README.md) | 简体中文

Sesame 是一个本地优先的个人助理 runtime，用于管理 workspace 范围内的工具、角色、自动化、报告和记忆。

它把对话、工具执行、后台任务、自动化、报告和运行时状态收拢到同一个以 workspace 为边界的运行链路里，让你可以在同一份上下文里提出需求、执行工作、查看结果并继续处理。

## 为什么是 Sesame

- 本地优先。Workspace 状态保存在你的机器上，而不是远端 SaaS 控制面。
- 单一运行主线。交互式对话、后台执行、报告和自动化共享同一套 runtime 模型。
- 以 workspace 为边界。上下文历史、任务、报告、角色和运行时数据都绑定在 workspace 上。
- 终端原生。日常工作用 CLI / TUI，需要更强可视化时使用 Web Console。
- 文件化角色。Specialist role 存放在 `roles/<role_id>/` 下，可以作为 workspace 资产管理。

## 功能特性

- 面向交互式 agent 工作流的 CLI 和 TUI 入口
- 自动管理 workspace 运行时状态的本地 daemon
- 支持 load、reopen 和分支式操作的上下文历史
- 内置 shell、文件、补丁、搜索和任务委托等工具
- 基于 task 的 specialist delegation，并通过 report delivery 回到主对话
- 带 skill gating 的 role automations：watcher asset -> owner role task -> main agent report -> policy loop
- role-owned automation source files、watcher contract validation 和 runtime inspection
- 用于 chat、runtime、roles 和 usage 的 Web Console
- 可选 Discord ingress，用于远程接入同一套 workspace runtime

## 快速开始

### 环境要求

- Go `1.24+`
- 在 `~/.sesame/config.json` 中配置模型提供方
- 当前主要在 Linux / WSL 环境下测试

### 1. 克隆仓库并进入一个 workspace

```bash
git clone <your-fork-or-repo-url>
cd Sesame-agent
mkdir -p /path/to/workspace
```

### 2. 启动 Sesame

在仓库根目录执行，并显式指定要使用的 workspace：

```bash
go run ./cmd/sesame --workspace /path/to/workspace
```

首次使用请先完成 setup：

```bash
go run ./cmd/sesame setup
```

后续如果需要重新打开配置向导：

```bash
go run ./cmd/sesame configure
```

`configure` 会打开一个共享配置主页，包含两个入口：
- `Model Setup`（必需）
- `Third-Party Integrations`（可选）

Discord 配置位于 `Third-Party Integrations` 下。启动时只要求完成 `Model Setup`，Discord 可稍后再配置。

启用 Discord 时，`Allowed User IDs` 为必填项。配置流程会拒绝空值，避免 bot 意外接受所有用户消息，或因空白名单导致所有用户都被静默拒绝。

当配置缺失时，直接执行 `sesame` 启动会自动进入 setup。

或者查看 daemon / runtime 状态：

```bash
go run ./cmd/sesame --workspace /path/to/workspace --status
```

### 3. 打开 Console

当本地 daemon 运行后，在浏览器中打开：

```text
http://127.0.0.1:4317/
```

### 4. 开始使用

常用聊天命令：

- `/history`
- `/history load <head_id>`
- `/reopen`

## 配置

Sesame 主要使用两个存储位置：

- 全局配置和共享本地状态：`~/.sesame/`
- workspace 运行时状态：`<workspace>/.sesame/`

模型提供方配置文件位置：

```text
~/.sesame/config.json
```

你也可以随时通过 `sesame configure` 回到共享配置主页（`Model Setup` 与 `Third-Party Integrations`）。

## 工作原理

Sesame 正在收敛到一套更明确的 runtime 模型，核心对象包括：

- `workspace`：运行时状态的 aggregate root
- `session`：与用户交互的主绑定
- `context head`：历史、重载、重开和分支的边界
- `task`：后台执行的主干语义
- `report`：子任务 / 后台任务回到主线的交付物
- `role`：specialist 行为所使用的文件化执行配置

典型执行链路：

```text
用户请求
  -> 主 parent session
  -> 工具调用或 task 创建
  -> runtime 执行
  -> report delivery / task result
  -> 主 parent 向用户回复
```

Specialist role 可以在内部使用 session 或 context handle 作为实现细节，但公开模型应当是 workspace runtime orchestration，而不是多 agent 聊天室。

## 自动化模型

Simple automation 使用一条明确的 runtime 链路：

```text
role watcher script
  -> runtime dispatch lock
  -> owner role task
  -> main agent report delivery
  -> policy-driven resume / pause / escalation
```

watcher 只负责检测。当 watcher 上报 `needs_agent` 时，Sesame 会暂停该 watcher 的本轮调度，为这次信号只派发一个任务给 owning role，等待 task 结果，把 report 交付给 main agent，然后根据 automation policy 决定恢复、暂停或升级。

Automation 创建被显式 gating：

- automation definition 工作必须先激活 `automation-standard-behavior` 和 `automation-normalizer`，再使用 simple automation builder。
- role-owned automation 必须由 owning specialist role session 创建。
- owner task 不能创建、修改、暂停或恢复 automation；它只执行 `automation_goal` 并回报结果。
- watcher script 必须输出受支持的 `script_status` JSON contract。旧的 `{"trigger": ...}` 风格 payload 会被拒绝。

这能把创建、运行时执行、状态汇报三类 turn 分开，避免 watcher 命中后漂移到 automation 配置修改，或重复派发 owner task。

## 仓库结构

- `cmd/sesame`
  CLI 入口
- `internal/cli`
  TUI、REPL、客户端调用和终端渲染
- `internal/daemon`
  runtime 组装、恢复、HTTP 服务和调度
- `internal/engine`
  turn 执行、prompt 组装、tool wiring 和上下文刷新
- `internal/session`
  session 排队、delegation 和 runtime handoff
- `internal/task`
  后台 task 模型和执行
- `internal/tools`
  内置工具、工具运行时、能力门禁和执行控制
- `internal/automation`
  watcher、simple owner-task automation 和 automation 生命周期
- `internal/reporting`
  report 投递
- `internal/roles`
  文件化 role catalog 和 role service
- `internal/store/sqlite`
  本地持久化
- `web/console`
  基于 React 的 Console UI

## 当前状态

Sesame 正在继续收敛到更明确的 workspace runtime 模型：

- workspace 是主要 runtime 边界
- task 是后台执行主语义
- role 是文件化 execution profile，而不是第二条公开聊天主线
- runtime diagnostics、reports、tasks、roles 和 automations 已经能在 console 中查看
- automation skills 与 tool-layer checks 协同执行模式边界
- TUI 与 Discord flows 共用同一套 daemon / session runtime

项目已经可以用于本地运维类工作流，但整体架构仍在继续收紧和简化。

## 路线图

- 继续围绕 workspace、task、report 和 context-head 简化 runtime spine
- 改进长时间运行 workspace 的 memory 和 history compaction
- 扩展 console 中的 runtime inspection 和 repair 流程
- 强化 role versioning、policy 边界和 diagnostics
- 在同一套本地 runtime 模型上增加更多外部入口

## 开发

从源码构建 CLI：

```bash
go build ./cmd/sesame
```

运行包检查：

```bash
go test ./...
```

构建 console：

```bash
cd web/console
npm run build
```

如果本地 checkout 中包含被 ignore 的测试文件，发布前运行相关 package tests。

## 许可证

许可证信息尚未最终确定。
