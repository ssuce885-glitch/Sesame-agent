# Sesame

[中文](./README.md) | [English](./README.en.md)

Sesame 是一个以终端版本为公开主线的本地通用代理。

它提供全屏 TUI、本地 daemon、持久化 session、工具调用、skill 加载和工作区感知的上下文管理，适合做终端自动化、系统巡检、定时报告和多代理协作。

## 设计方向

Sesame 正在朝一个可被人工对话、定时任务或外部程序唤起的本地 agent runtime 演进。

当前版本已经具备终端对话、daemon、session、调度、skills 和上下文管理这些基础设施，但“外部高频检测 + agent 按需执行”的完整事件触发链路还在继续补齐。

目标是把高频、廉价、确定性的检测留给脚本、监控器或业务系统，把真正需要理解、判断、修复、汇总和调用扩展 skills 的部分交给 agent 执行。这样可以减少长期运行中的 token 消耗，也更容易做权限约束、失败回退和结果汇报。

## 当前状态

当前仓库已经可以用于日常终端自动化工作流：

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

## 目标使用场景

未来主要面向这些使用场景：

- 服务器与服务健康检查：由外部脚本或监控程序做高频探测，异常时唤起 daemon 执行诊断、修复尝试和通知
- 定时巡检与周期报告：按固定时间收集状态、汇总结果，并投递到 mailbox、终端或邮件通道
- 批处理与任务守护：在任务失败、队列积压、处理延迟异常时自动收集证据并给出处置建议
- 本地工作流自动化：通过自然语言让 agent 生成脚本、执行脚本、校验结果并整理输出
- 多 agent 协作：把复杂任务拆分为多个子任务并回收结果，最后由主 agent 汇总结论
- skills 驱动的垂直扩展：安装邮件、部署、数据库或业务系统相关 skills 之后，把 Sesame 扩展成面向特定领域的自动化执行器

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
  "max_tool_steps": 100,
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

为靠近上面的目标方向，下一阶段重点会放在几类能力上：

- 增强事件触发式 agent 执行链路，让外部脚本、监控程序和业务系统可以在异常、告警或状态变化时直接唤起 daemon 执行任务
- 继续把高频检测与低频智能决策分层，减少长期运行中的模型消耗，并提升自动化链路的可控性
- 补齐面向自动执行的任务模板，包括证据收集、受限修复、结果验证、失败回退和报告投递
- 继续强化多 agent 协作，包括 skill 调用约束、子代理工具边界和报告汇总策略
- 补上 task understanding / retrieval，让 runtime 先理解任务意图，再做 skill 的检索、排序和选择，而不是主要依赖名字或 trigger 的字符串匹配
- 增加更硬的 runtime budget / arbitration，避免只有 prompt 软约束、缺少真正的执行收束
- 继续补齐 skill 规范化流程，把外部下载来的 skill 稳定收敛到 Sesame canonical format
- 评估是否增加一等 `web_search` / `news_lookup` 能力，而不是长期只依赖 `web_fetch`
- 继续增强可观测性，让 TUI / 调试视图能直接看到 selected profile、skill activations 和 budget usage
