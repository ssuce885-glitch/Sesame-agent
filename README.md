# Sesame

Sesame 是一个面向终端的本地代码代理。

它提供全屏 TUI、本地 daemon、持久化 session、工具调用、skill 加载和工作区感知的上下文管理，主工作流不依赖浏览器。

## 当前状态

当前仓库已经可以用于日常本地开发工作流：

- 终端 TUI 可直接对话
- 自动拉起或连接本地 daemon
- 自动为当前工作区选择或创建 session
- 支持 shell、读写文件、搜索、补丁、任务、权限等工具
- 支持全局和工作区 skill 发现与安装
- 支持 `Esc` 中断当前对话
- 支持鼠标滚轮和 `PgUp` / `PgDn` 滚动
- 支持通过 `~/.sesame/config.json` 配置模型与运行参数

当前公开方向以终端版本为主，`web/` 前端不作为这一版的主交付内容。

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
  "provider": "openai_compatible",
  "model": "glm-4-7-251222",
  "permission_profile": "trusted_local",
  "openai": {
    "api_key": "your-key",
    "base_url": "https://your-provider.example.com/v1",
    "model": "glm-4-7-251222"
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
  "provider": "anthropic",
  "model": "claude-sonnet-4-5",
  "permission_profile": "trusted_local",
  "anthropic": {
    "api_key": "your-key",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-5"
  }
}
```

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
- `/session list`
- `/session use <id>`
- `/clear`
- `/exit`

## Skills

Sesame 支持系统内置 skill、全局 skill 和工作区 skill。

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

## 仓库结构

- `cmd/sesame`：CLI 入口
- `internal/`：daemon、runtime、tools、session、storage、config 等核心实现
- `README.md`：项目说明

## 下一步计划

项目目前能力：

- 已具备本地 daemon、session 持久化和终端 TUI 对话能力
- 已支持 shell、文件读写、搜索、补丁、任务、权限控制等基础工具链
- 已支持 skill 发现、安装和工作区级扩展
- 已支持 turn 中断、上下文压缩和基础运行时状态管理

下一阶段我准备把 Sesame 往自动化方向再推进一层，重点包括：

- 集成 Harness，作为更稳定的任务编排和执行承载层
- 增加定时自动任务能力，比如按时间调度 workspace 巡检、自动总结、周期性执行命令
- 打通后台任务与 session / timeline / memory 的关联，让自动任务结果能自然回流到对话上下文
- 补充更清晰的任务可视化与停止/重试控制

定位：自用 agent。

当前目标：暂时应用于帮我自动化监控服务器内生产项目，遇服务故障会邮件通知我。
