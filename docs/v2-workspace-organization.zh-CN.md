# V2 工作区整理清单

更新时间：2026-05-03

## 当前状态

当前工作区是一次 V1 -> V2 大切换，不适合按单个小 diff 审查。

V1 删除边界已确认：348 个旧 V1 文件删除属于预期迁移结果，后续整理不再按异常恢复处理。

`git status` 摘要：

- 删除：348 个已跟踪文件，主要是旧 V1 runtime / engine / sqlite / tools / cli / api。
- 修改：20 个已跟踪文件，主要是入口、配置、Web Console、角色配置和项目文档。
- 新增：25 组未跟踪路径，主要是完整 V2 runtime、测试、skills、V2 文档和新版 Web 页面。

## 变更分组

### 1. V1 删除边界

状态：已确认。

审查清单：`docs/v2-v1-removal-manifest.zh-CN.md`

建议作为独立提交处理，提交信息类似：

```text
remove legacy v1 runtime
```

包含：

- `internal/api/http`
- `internal/automation`
- `internal/cli`
- `internal/context`
- `internal/daemon`
- `internal/engine`
- `internal/extensions`
- `internal/reporting`
- `internal/roles`
- `internal/scheduler`
- `internal/session`
- `internal/store/sqlite`
- `internal/task`
- `internal/tools`
- `cmd/eval`
- `cmd/lifecycle-test`
- `cmd/prefill-test-db`

注意：

- 这是最大风险提交，因为它会删除大量历史实现。
- 合并前应确认 V2 已覆盖 CLI/TUI、Web、roles、tasks、automation、context、reports 的最小业务闭环。

### 2. V2 后端 runtime

审查清单：`docs/v2-backend-runtime-manifest.zh-CN.md`

建议作为独立提交处理，提交信息类似：

```text
add v2 runtime backend
```

包含：

- `internal/v2/agent`
- `internal/v2/app`
- `internal/v2/automation`
- `internal/v2/client`
- `internal/v2/contracts`
- `internal/v2/memory`
- `internal/v2/observability`
- `internal/v2/reports`
- `internal/v2/roles`
- `internal/v2/schema`
- `internal/v2/session`
- `internal/v2/store`
- `internal/v2/tasks`
- `internal/v2/tools`
- `internal/v2/tui`

本组已覆盖：

- daemon HTTP server
- session queue
- append-only messages/events
- agent turn loop
- role CRUD/tools
- specialist task runner
- task trace
- reports
- memory/project state
- automation watcher dispatch
- TUI timeline/reports

### 3. CLI / 配置 / 系统提示词

审查清单：`docs/v2-cli-config-manifest.zh-CN.md`

建议作为独立提交处理，提交信息类似：

```text
wire sesame cli to v2 daemon and tui
```

包含：

- `cmd/sesame/main.go`
- `cmd/sesame/main_test.go`
- `internal/config/config.go`
- `internal/config/system_prompt_test.go`
- `README.md`

本组已处理：

- `sesame` 默认连接/拉起 V2 daemon。
- `0.0.0.0` / `:port` 监听地址转本地连接地址。
- 默认系统提示词固定 Sesame 自我认知，避免模型自称 Claude/Anthropic。

### 4. Web Console V2

审查清单：`docs/v2-web-console-manifest.zh-CN.md`

建议作为独立提交处理，提交信息类似：

```text
migrate web console to v2 api
```

包含：

- `web/console/src/App.tsx`
- `web/console/src/api`
- `web/console/src/components`
- `web/console/src/pages`
- `web/console/src/i18n.tsx`
- `web/console/vite.config.ts`
- `web/console/src/test`

本组已覆盖：

- Chat timeline
- Reports
- Tasks / task trace
- Roles create/edit/test run
- Automations
- Context / project state / memory
- SSE reconnect and robust event parsing

注意：

- `npm run build` 通过，但仍有 Vite 大 chunk 警告。
- 后续可做 route-level dynamic import 拆包。

### 5. Role / Skills 业务资产

审查清单：`docs/v2-roles-skills-manifest.zh-CN.md`

建议作为独立提交处理，提交信息类似：

```text
add workspace roles and skills
```

包含：

- `roles/reddit_monitor/role.yaml`
- `roles/reddit_monitor/prompt.md`
- `roles/reddit_monitor/.role-versions/000006.yaml`
- `skills/automation-normalizer/SKILL.md`
- `skills/automation-standard-behavior/SKILL.md`
- `skills/discord/SKILL.md`
- `skills/email/SKILL.md`
- `skills/scrapling/SKILL.md`
- `skills/slack/SKILL.md`

仍需人工确认：

- `roles/reddit_monitor/prompt.md` 里有收件人邮箱和本地代理地址，是否应该进入仓库需要确认。
- Reddit Monitor 还需要真实自动化联调。

### 6. 文档 / 测试 / 杂项

建议作为独立提交处理，提交信息类似：

```text
document v2 integration and cleanup plan
```

包含：

- `.gitignore`
- `DESIGN_DECISIONS.md`
- `docs/v2-audit-report.zh-CN.md`
- `docs/v2-backend-runtime-manifest.zh-CN.md`
- `docs/v2-cli-config-manifest.zh-CN.md`
- `docs/v2-integration-checklist.zh-CN.md`
- `docs/v2-roles-skills-manifest.zh-CN.md`
- `docs/v2-v1-removal-manifest.zh-CN.md`
- `docs/v2-web-console-manifest.zh-CN.md`
- `docs/v2-workspace-organization.zh-CN.md`
- `internal/skillcatalog/load.go`
- `internal/skillcatalog/load_test.go`
- `internal/skillcatalog/types.go`

## 当前验证状态

最近一次完整验证已通过：

```bash
go test ./...
go vet ./...
staticcheck ./...
npm test
npm run build
git diff --check
```

唯一非失败项：

- `npm run build` 提示 JS chunk 超过 500 kB。

## 推荐收敛顺序

1. 先确认 V1 删除是否作为一个大提交进入历史。
2. 再提交 V2 后端 runtime。
3. 再提交 CLI/config/system prompt。
4. 再提交 Web Console V2。
5. 再提交 roles/skills 业务资产。
6. 最后提交 docs/tests/gitignore 清理。

## 不建议现在做的事

- 不要用 `git reset --hard` 或 `git checkout -- .`。
- 不要把 V1 删除和 V2 新增揉成一个不可审查提交。
- 不要在未确认前移除 `reddit_monitor` 的邮箱/代理配置；应先决定是保留样例、改成环境变量，还是移到 workspace setting。
- 不要把 `web/console/dist` 或本地 `.sesame` 运行态加入仓库。
