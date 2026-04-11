# Sesame

Sesame 是一个面向终端、本地 daemon 驱动的代码代理运行时。

它围绕持续会话、任务编排、定时报告、可扩展 skills 和工作区感知的上下文管理设计，适合把一次性问答升级成可持续运行的本地自动化工作流。

## 适合的工作场景

- 服务器状态监控与定时汇报
  让 agent 定期检查服务健康状态、日志、资源占用和异常信号，并把结果作为异步 report 回到当前会话。
- 故障排查与持续跟踪
  适合“先看一下现场，几分钟后再回来告诉我有没有恢复”这类需要延时 follow-up 的任务。
- 多任务拆分与异步协作
  把复杂工作拆成后台任务，由主会话负责调度、等待和汇总结果，而不是把所有步骤塞进一轮同步对话。
- 面向工作流的 skills 扩展
  安装 skills 后，可以把 Sesame 扩展成适配具体团队流程的 agent，例如发布巡检、生产值守、代码审查流程、交付检查或内部 SOP 执行器。
- 长时间运行的本地代理工作流
  适合持续运行、结果稍后返回、需要会话连续性的自动化任务，而不仅是一次命令执行。

## 为什么是 Sesame

- 终端优先的本地 agent 体验，带持久化 session 和本地 daemon
- 原生支持后台任务、定时报告、mailbox 和异步结果回流
- skills 可按全局或工作区安装，并在对话中按需激活
- 面向工作区的工具体系，覆盖文件、补丁、搜索、任务和权限控制
- 对长会话做了明确的上下文预算控制，适合持续运行的自动化场景

## 当前状态

当前仓库已经可以用于日常本地开发工作流：

- 终端 TUI 可直接对话
- 自动拉起或连接本地 daemon
- 自动为当前工作区选择或创建 session
- 支持 shell、读写文件、搜索、补丁、任务、权限等工具
- 支持全局和工作区 skill 发现与安装
- 支持真实定时/周期自动汇报任务
- 支持 mailbox 收件箱和 cron 任务管理
- 支持 `Esc` 中断当前对话
- 支持鼠标滚轮和 `PgUp` / `PgDn` 滚动
- 支持通过 `~/.sesame/config.json` 配置模型与运行参数

Sesame 目前已经适合日常本地开发、服务器巡检和定时汇报类工作流，但整体仍在快速演进。

## 环境要求

- Go `1.24+`
- 已配置可用模型，配置方式为环境变量或 `~/.sesame/config.json`

## 快速启动

在仓库根目录运行：

```bash
go run ./cmd/sesame
```

如果你更喜欢先编译：

```bash
go build -o sesame ./cmd/sesame
./sesame
```

如果 `~/.sesame/config.json` 不存在，或缺少必要字段，Sesame 会在终端里引导你完成初始化。

## 模型配置

用户配置文件路径：

```text
~/.sesame/config.json
```

OpenAI 兼容接口示例：

```json
{
  "active_profile": "coding",
  "permission_profile": "trusted_local",
  "model_providers": {
    "openai_compatible": {
      "api_family": "openai_responses",
      "base_url": "https://your-provider.example.com/v1",
      "api_key_env": "OPENAI_API_KEY"
    }
  },
  "profiles": {
    "coding": {
      "model": "glm-4-7-251222",
      "model_provider": "openai_compatible",
      "reasoning": "high",
      "verbosity": "medium"
    }
  },
  "max_tool_steps": 8,
  "max_recent_items": 12,
  "compaction_threshold": 32,
  "max_estimated_tokens": 16000,
  "microcompact_bytes_threshold": 8192
}
```

Anthropic 示例：

```json
{
  "active_profile": "default",
  "permission_profile": "trusted_local",
  "model_providers": {
    "anthropic": {
      "api_family": "anthropic_messages",
      "base_url": "https://api.anthropic.com",
      "api_key_env": "ANTHROPIC_API_KEY"
    }
  },
  "profiles": {
    "default": {
      "model": "claude-sonnet-4-5",
      "model_provider": "anthropic",
      "cache_profile": "anthropic_default"
    }
  }
}
```

配置文件中的模型通过 profile 选择；运行时也可以用 `--model` 仅覆盖当前激活 profile 的模型名。

本地假模型 smoke test：

```bash
SESAME_MODEL_PROVIDER=fake SESAME_MODEL=fake-smoke SESAME_PERMISSION_PROFILE=trusted_local go run ./cmd/sesame
```

## 终端使用

常用命令：

```bash
go run ./cmd/sesame
go run ./cmd/sesame --status
go run ./cmd/sesame --print "inspect this workspace"
go run ./cmd/sesame --resume sess_123
go run ./cmd/sesame daemon
```

TUI 快捷键：

- `Enter` 发送
- `Alt+Enter` 换行
- `Esc` 中断当前 turn
- `Mouse wheel` / `PgUp` / `PgDn` 滚动
- `Ctrl+C` 退出

内置 slash 命令：

- `/help`
- `/status`
- `/skills`
- `/tools`
- `/mailbox`
- `/cron list`
- `/cron inspect <id>`
- `/cron pause <id>`
- `/cron resume <id>`
- `/cron remove <id>`
- `/session list`
- `/session use <id>`
- `/clear`
- `/exit`

## Skills

Sesame 支持系统内置 skill、全局 skill 和工作区 skill。

运行时会在每轮开始时发现已安装 skill，并把已安装清单展示给模型。需要使用某个 skill 时，模型会通过 `skill_use` 工具按精确名称显式激活，而不是靠提示词里的 `$skill-name` 隐式触发。

目录位置：

- 全局：`~/.sesame/skills`
- 工作区：`<workspace>/.sesame/skills`

示例：

```bash
go run ./cmd/sesame skill list
go run ./cmd/sesame skill inspect https://github.com/openai/skills
go run ./cmd/sesame skill install ./path/to/skill
go run ./cmd/sesame skill install openai/skills --path skills/.curated/parallel --scope workspace
go run ./cmd/sesame skill remove parallel
```

安装完成后，新 skill 会在后续 turn 中出现在已安装 skills 列表里，可被 `skill_use` 激活。

## 任务、子代理与定时报告

Sesame 的原生能力不只是一轮问答。它支持后台任务、结果等待、异步回流和真实定时报告，适合把 agent 放进持续运行的本地工作流中。

- 使用 `task_*` 工具把复杂工作拆成后台执行单元
- 使用 `schedule_report` 创建真实的 delayed / recurring report
- 通过 mailbox 和会话恢复机制把异步结果带回当前会话
- 结合 skills，把任务编排能力扩展到更垂直的场景

## 上下文管理

Sesame 针对长会话做了明确的上下文预算控制，包括最近上下文窗口、prompt token 估算、滚动压缩、超大工具结果微压缩、workspace prompt 大小限制和 provider cache 接入。

这让它更适合监控、定时汇报和多步骤异步协作这类容易让上下文不断膨胀的工作流。

## 仓库结构

- `cmd/sesame`：CLI 入口
- `internal/`：daemon、runtime、tools、session、storage、config 等核心实现
- `README.md`：项目说明

## 近期方向

- 继续强化 skill/runtime 机制和工作区扩展能力
- 强化多任务协作、异步结果汇总和定时报告工作流
- 持续优化模型 provider/profile 配置体验
- 继续改进终端交互和对外文档

定位：自用 agent。

当前目标：暂时应用于帮我自动化监控服务器内生产项目，遇服务故障会邮件通知我。
