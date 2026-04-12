# Sesame

Sesame 是一个以终端版本为公开主线的本地代码代理。

它提供全屏 TUI、本地 daemon、持久化 session、工具调用、skill 加载和工作区感知的上下文管理，主工作流不依赖浏览器。

## 当前状态

当前仓库已经可以用于日常本地开发工作流：

- 终端 TUI 可直接对话
- 自动拉起或连接本地 daemon
- 自动为当前工作区选择或创建 session
- 支持 shell、读写文件、搜索、补丁、任务、权限等工具
- 支持全局和工作区 skill 发现与安装
- skill 注入已改成两段式：默认只给极短 implicit hints，显式激活或 runtime 选中的 skill 才注入完整正文
- 已按能力拆分 tool routing profile：`codebase_edit`、`system_inspect`、`web_lookup`、`browser_automation`、`scheduled_report`
- 支持 system skills，当前内置 `skill-installer` 和 `skill-normalizer`
- 支持真实定时/周期自动汇报任务
- 支持 mailbox 收件箱和 cron 任务管理
- 支持在 TUI / REPL 中处理权限中断，并通过 `/approve` / `/deny` 继续恢复当前 turn
- 支持 `Esc` 中断当前对话
- 支持鼠标滚轮和 `PgUp` / `PgDn` 滚动
- 支持通过 `~/.sesame/config.json` 配置模型与运行参数

## 最近完成

最近这一轮已经完成的重构和补强：

- 重构了 skills 注入层，去掉每轮默认注入的全量本地 skill 摘要，减少 prompt 污染
- 新增 capability-profile 路由骨架，让普通网页问答和浏览器自动化任务不再混在一起
- 新增 `skill-normalizer` system skill，用于把第三方或下载来的 skill 规范化成 Sesame 格式
- 增加按需 `Catalog snapshot`，当用户明确询问 skills / tools 时，模型能回答当前已加载目录内容，而不是只复述 turn-visible tools
- 修复权限请求后的交互链路，权限中断不再表现成“卡住不继续”
- 修正文案和渲染，`web_fetch` 不再被误显示成 `search`
- 修复严格 provider 下 session memory refresh 会错误压缩未闭合 tool exchange 的问题，避免中断或打满工具步数后触发 compactor transcript 校验报错
- 修复 OpenAI-compatible 流式 function call 参数容错，遇到过早 `done`、仅 delta 补全或异常 arguments 序列时不再直接打断整轮对话

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
- `Tab` / `Shift+Tab` 切换 `Chat` / `Agents` / `Mailbox` / `Cron` 视图
- `Esc` 中断当前 turn
- `Mouse wheel` / `PgUp` / `PgDn` 滚动
- `Ctrl+C` 退出

通用 slash 命令：

- `/help`
- `/status`
- `/skills`
- `/tools`
- `/approve [<request_id>] [once|run|session]`
- `/deny [<request_id>]`
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

TUI 视图命令：

- `/chat`
- `/agents`

## Skills

Sesame 支持系统内置 skill、全局 skill 和工作区 skill。

已安装 skill 默认按需激活：

- 普通 turn 只会收到允许隐式触发的短摘要
- 只有用户显式点名或 runtime 选中的 skill 才会注入完整正文

目录位置：

- 系统：`~/.sesame/skills/.system`
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

下一阶段重点会放在几类能力上：

- 正在思考如何做自动化任务报告的推送
- 继续强化多 agent 协作，包括 skill 调用约束、子代理工具边界和报告汇总策略
- 补上 task understanding / retrieval，让 runtime 先理解任务意图，再做 skill 的检索、排序和选择，而不是主要依赖名字或 trigger 的字符串匹配
- 增加更硬的 runtime budget / arbitration，避免只有 prompt 软约束、缺少真正的执行收束
- 继续补齐 skill 规范化流程，把外部下载来的 skill 稳定收敛到 Sesame canonical format
- 评估是否增加一等 `web_search` / `news_lookup` 能力，而不是长期只依赖 `web_fetch`
- 继续增强可观测性，让 TUI / 调试视图能直接看到 selected profile、skill activations 和 budget usage

定位：自用 agent。

当前目标：暂时应用于帮我自动化监控服务器内生产项目，遇服务故障会邮件通知我。
