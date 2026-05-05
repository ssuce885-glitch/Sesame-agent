# V2 Roles / Skills 本地资产边界

更新时间：2026-05-05

## 结论

`roles/` 和顶层 `skills/` 是 workspace 本地业务资产，不属于 GitHub 代码仓库内容。

当前仓库策略：

- `roles/` 已加入 `.gitignore`
- `skills/` 已加入 `.gitignore`
- 已从 Git 索引移除当前本地 `reddit_monitor` role 和 workspace skills
- 本地文件已移到 `/home/sauce/project/Workspace/sesame-agent-local-workspace/`，可继续用于联调

## 为什么不上传

- role 包含真实业务目标、通知策略、收件人、代理等 workspace 配置。
- skill 可能包含本地通知、抓取、第三方服务使用方式。
- 这些内容更接近用户数据和部署配置，不是 Sesame runtime 源码。

## 代码仓库应包含

- V2 runtime 后端
- CLI/TUI 入口
- Web Console
- role/skill 的加载、创建、编辑、执行能力
- automation、task、report、context、memory 等通用运行时
- 文档和测试

## 代码仓库不应包含

- `roles/<role_id>/...`
- 顶层 `skills/<skill_name>/...`
- `.sesame/`
- 数据库、日志、构建产物、测试 workspace

## 后续分发方式

如果未来要提供示例或模板，建议新建独立边界：

- `examples/roles/...`
- `examples/skills/...`
- `sesame init role <template>`
- `sesame skill install <template>`

示例内容必须使用占位符，不写入真实邮箱、代理、API key、token 或内部地址。

## 当前模板和 CLI 用法

本仓库现在提供只读分发边界：

- `examples/skills/`：官方 skill 模板目录
- `examples/workflows/library.json`：workflow 模板分发索引

它们都不会被 runtime 自动加载，只用于复制、安装和录入。

常用命令：

```bash
sesame skill lint examples/skills/*/SKILL.md
sesame skill test examples/skills/*/SKILL.md
sesame skill install automation-normalizer --workspace /path/to/workspace
sesame skill install /absolute/path/to/local-skill-template --workspace /path/to/workspace
sesame skill pack automation-normalizer --out /tmp/automation-normalizer.zip
```

`sesame skill lint <path...> [--workspace <root>]` 支持一次检查一个或多个 skill 文件，当前会检查：

- `id` 或 `name` 非空
- `description` 非空
- `requires_tools` 非空且工具名存在于当前内置 runtime tool 集
- `risk_level` 非空
- front matter 不包含明显 secret 关键词
- body 或 `prompt_file` 不为空

`sesame skill test <path...> [--workspace <root>]` 不启动 daemon、不调用模型、不执行外部工具；会先复用 lint，再额外检查 manifest 里的 `examples` / `tests`：

- 路径必须是相对路径并且留在 skill 目录内
- 不能命中 symlink，也不能穿过 symlink 目录
- 必须是可读、非空的 regular file
- 任一 error 返回 1；usage 返回 2

manifest 字段约定：

- 推荐使用 `requires_tools` 记录 skill 依赖的内置工具。
- `allowed_tools` / `allowed-tools` 只是历史别名，当前 parser 会兼容合并到 `requires_tools`。
- 这些字段只用于声明和 lint，不会自动解锁 gated tools；实际可见性仍由 role policy、runtime gate 和执行上下文决定。

`sesame skill install` 当前行为：

- 模板名会先从 `examples/skills/<name>` 查找
- 也支持直接安装本地目录或 markdown 文件
- 安装目标固定为 `<workspace>/skills/<skill-name>/`
- 已存在目标不会被覆盖

`sesame skill pack` 当前行为：

- `--out` 必填，且不会覆盖已存在目标
- source 解析与 `sesame skill install` 共用模板名/路径解析逻辑
- 打包前必须先通过 lint/test 等价校验，否则不生成 zip
- zip 根目录固定为 `installSkillName` 规则得到的 skill 名称
- 目录模板会保留全部普通文件；markdown 单文件模板只打包为 `<skillName>/SKILL.md`
- source 或目录内任意 symlink entry 都会被拒绝

## 本地联调项

`reddit_monitor` 仍可作为本机 workspace 业务资产继续联调：

- Reddit JSON API 抓取
- `scrapling` fallback
- email 发送链路
- watcher `needs_agent` dispatch
- role final response -> report view
