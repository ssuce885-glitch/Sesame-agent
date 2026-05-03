# V2 Roles / Skills 本地资产边界

更新时间：2026-05-03

## 结论

`roles/` 和顶层 `skills/` 是 workspace 本地业务资产，不属于 GitHub 代码仓库内容。

当前仓库策略：

- `roles/` 已加入 `.gitignore`
- `skills/` 已加入 `.gitignore`
- 已从 Git 索引移除当前本地 `reddit_monitor` role 和 workspace skills
- 本地文件保留，可继续用于当前 workspace 联调

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

## 本地联调项

`reddit_monitor` 仍可作为本机 workspace 业务资产继续联调：

- Reddit JSON API 抓取
- `scrapling` fallback
- email 发送链路
- watcher `needs_agent` dispatch
- role final response -> report view
