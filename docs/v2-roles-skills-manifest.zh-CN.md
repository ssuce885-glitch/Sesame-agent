# V2 Roles / Skills Manifest

更新时间：2026-05-03

## 结论

Workspace role 和 skills 已形成独立业务资产边界，可作为单独提交审查。

验证结果：

```bash
go test ./internal/skillcatalog ./internal/v2/roles ./internal/v2/tools
```

通过。

## 建议提交

提交信息：

```text
add workspace roles and skills
```

建议包含：

- `roles/reddit_monitor/role.yaml`
- `roles/reddit_monitor/prompt.md`
- `roles/reddit_monitor/.role-versions/000006.yaml`
- `skills/automation-normalizer/SKILL.md`
- `skills/automation-standard-behavior/SKILL.md`
- `skills/discord/SKILL.md`
- `skills/email/SKILL.md`
- `skills/scrapling/SKILL.md`
- `skills/slack/SKILL.md`

不要混入：

- V1 删除
- V2 runtime backend
- CLI/config/system prompt
- Web Console 迁移

## 当前 Role

### `reddit_monitor`

文件：

- `roles/reddit_monitor/role.yaml`
- `roles/reddit_monitor/prompt.md`
- `roles/reddit_monitor/.role-versions/000006.yaml`

当前配置：

- skills: `scrapling`, `email`, `automation-standard-behavior`, `automation-normalizer`
- permission profile: `workspace`
- can delegate: `false`
- automation ownership: `reddit_monitor`
- max tool calls: `80`
- max context tokens: `80000`

## 当前 Skills

- `automation-normalizer`
- `automation-standard-behavior`
- `discord`
- `email`
- `scrapling`
- `slack`

## 已知人工确认项

`roles/reddit_monitor/prompt.md` 当前包含：

- 收件人邮箱：`1582914562@qq.com`
- 本地代理地址：`http://127.0.0.1:7897`

这些配置是否进入仓库需要人工确认。可选处理方向：

1. 保留为当前 workspace 的真实业务配置。
2. 改成环境变量或 workspace setting。
3. 改成占位符，并在 README/role 文档里说明如何配置。

## 联调项

仍需真实联调：

- Reddit JSON API 抓取
- `scrapling` fallback
- email 发送链路
- watcher `needs_agent` dispatch
- role final response -> report view
