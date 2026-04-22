# Sesame

[English](README.md) | 简体中文

Sesame 是一个本地优先的 workspace agent runtime，面向终端驱动的软件操作场景。

它把对话、工具执行、权限审批、后台任务、自动化、报告和运行时状态收拢到同一个以 workspace 为边界的运行链路里，让你可以在同一份上下文里提出需求、执行工作、查看结果并继续处理。

## 为什么是 Sesame

- 本地优先。Workspace 状态保存在你的机器上，而不是远端 SaaS 控制面。
- 单一运行主线。交互式对话、后台执行、审批、报告和自动化共享同一套 runtime 模型。
- 以 workspace 为边界。上下文历史、任务、incident、报告、角色和运行时数据都绑定在 workspace 上。
- 终端原生。日常工作用 CLI / TUI，需要更强可视化时使用 Web Console。
- 文件化角色。Specialist role 存放在 `roles/<role_id>/` 下，可以作为 workspace 资产管理。

## 功能特性

- 面向交互式 agent 工作流的 CLI 和 TUI 入口
- 自动管理 workspace 运行时状态的本地 daemon
- 支持 load、reopen 和分支式操作的上下文历史
- 内置 shell、文件、补丁、搜索、任务委托和审批等工具
- 基于 task 的 specialist delegation，并通过 child report 回到主对话
- workspace 级 automations、incidents、mailbox reports 和 runtime inspection
- 用于 chat、runtime、roles 和 usage 的 Web Console

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
go run ./cmd/sesame --workspace-root /path/to/workspace
```

首次使用请先完成 setup：

```bash
go run ./cmd/sesame setup --workspace-root /path/to/workspace
```

后续如果需要重新打开配置向导：

```bash
go run ./cmd/sesame configure --workspace-root /path/to/workspace
```

`configure` 会打开一个共享配置主页，包含两个入口：
- `Model Setup`（必需）
- `Third-Party Integrations`（可选）

Discord 配置位于 `Third-Party Integrations` 下。启动时只要求完成 `Model Setup`，Discord 可稍后再配置。

当配置缺失时，直接执行 `sesame` 启动会自动进入 setup。

或者查看 daemon / runtime 状态：

```bash
go run ./cmd/sesame --workspace-root /path/to/workspace --status
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
  -> child report / approval / result
  -> 主 parent 向用户回复
```

Specialist role 可以在内部使用 session 或 context handle 作为实现细节，但公开模型应当是 workspace runtime orchestration，而不是多 agent 聊天室。

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
  内置工具、工具运行时、审批和执行控制
- `internal/automation`
  watcher、dispatch、incident 和 automation 生命周期
- `internal/reporting`
  mailbox / report 投递
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
- runtime diagnostics、reports、approvals 和 automations 已经能在 console 中查看

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

构建 console：

```bash
cd web/console
npm run build
```

当前公开仓库不包含测试源码。发布结构只保留了运行 Sesame 所需的 runtime、CLI、daemon、connector 和 console 代码。

## 许可证

许可证信息尚未最终确定。
