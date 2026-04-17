# Sesame

Sesame 是一个本地终端 agent runtime，面向单机 workspace 协作场景。

它把对话、工具执行、权限审批、任务派发、自动化巡检和结果回报放到同一条本地运行链路里，帮助你在一个 workspace 里直接完成“提需求、执行、观察状态、继续处理”的闭环。

## 它能干什么

- TUI / REPL 对话入口
- 自动连接或拉起本地 runtime
- workspace 级会话与上下文历史
- shell、文件、搜索、补丁、任务、权限、skills 等内置工具
- 角色会话委派能力，例如把巡检类工作交给专门角色处理
- workspace mailbox / cron / incident / automation runtime
- task-backed 自动化执行与结果回报

## 它解决什么问题

- 需要在终端里直接完成“提需求、执行工具、查看结果、继续处理”的连续工作流
- 需要让本地 agent 的任务、巡检、报告和运行状态有统一入口可查
- 需要在一个 workspace 内持续保留上下文历史，而不是每次都从头说明背景
- 需要把一次性的人工操作逐步沉淀成可重复执行的自动化流程
- 需要在自动化触发后快速知道它有没有运行、最近是否触发、以及最后是如何处理的

## 常用入口

和上下文历史相关的主要命令：

- `/history`
- `/history load <head_id>`
- `/reopen`

## 快速开始

环境要求：

- Go `1.24+`
- 已配置模型，配置文件路径为 `~/.sesame/config.json`

运行：

需要先进入工作区，工作区建议为空文件夹
```bash
go run ./cmd/sesame
```

查看状态：

```bash
go run ./cmd/sesame --status
```



## 运行时目录

默认情况下：

- 全局配置放在 `~/.sesame/`
- 当前 workspace 的 session、history、task、mailbox、automation runtime 放在 `<workspace>/.sesame/`


## 仓库结构

- `cmd/sesame`
  CLI 入口
- `internal/cli`
  终端交互、TUI、客户端调用
- `internal/daemon`
  runtime 启动、事件循环、会话驱动
- `internal/engine`
  turn 执行、提示词拼装、工具调度、上下文刷新
- `internal/session` / `internal/sessionrole`
  会话管理、角色会话解析与委派
- `internal/tools`
  内置工具、执行器、权限与运行时锁
- `internal/automation`
  巡检、自动化、派发与交付
- `internal/reporting`
  mailbox / reporting 收口
- `internal/store/sqlite`
  本地存储实现
- `internal/types`
  共享类型定义

## 下一步计划

- 梳理历史与记忆存储结构，减少当前存储模型较粗、语义边界不清的问题
- 之前考虑各种情况过度设计了很多内容，后面做大量删减之后运行效果更好了
- 记忆分长期短期等，后面这块重构一下，目前就是直接都塞在数据库中，比较乱，长期运行上下文可能有问题
- 完善多 agent 协作能力，让主助手和专门 agent 可以更自然地协同处理不同类型任务
- 支持手动创建和管理 agent，让长期角色和临时 agent 的使用方式更清晰
- 接入 Discord，把外部入口接到现有 runtime 和会话协作链路里


## 其他
- 只在wsl和linux系统上用了，对MAC和win的运行效果不清楚
- 目前项目已经满足我对生产服务器的监控需要,日常监控容器健康情况和服务器资源占用情况等都能完成，token消耗较低。
- 后续再拓展到其它工作方向上看看