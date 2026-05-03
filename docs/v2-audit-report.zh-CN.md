# Sesame V2 工作区审计报告

日期：2026-05-03

## 结论

V2 主干可以继续推进，但当前工作区仍不适合直接合成一个大提交。主要原因不是测试失败，而是变更边界过大：V1 大面积删除、V2 后端新增、Web 控制台迁移、角色配置更新、文档迁移混在一起。

工作区拆分和提交边界见 `docs/v2-workspace-organization.zh-CN.md`。

V1 删除边界已确认：348 个旧 V1 文件删除属于预期迁移结果。

当前自动检查结果：

- `go test ./...` 通过
- `staticcheck ./...` 通过
- `npm test` 通过
- `npm run build` 通过，存在 Vite 大 chunk 警告
- `git diff --check` 通过

## 变更分组

建议按以下组收口，避免一个提交同时承担迁移、删除、前端、角色和文档风险。

### A. V2 后端与入口

范围：

- `cmd/sesame/main.go`
- `internal/config/config.go`
- `internal/config/system_prompt_test.go`
- `internal/skillcatalog/types.go`
- `internal/skillcatalog/load.go`
- `internal/v2/**`

状态：

- V2 后端测试通过。
- daemon smoke 已验证 `/v2/status`、`/v2/roles`、`POST/GET/PUT /v2/roles`。
- 已修复 role turn 使用 role prompt/model 的问题。
- 已给 watcher/shell/task output 加默认超时和输出上限。
- 已移除 daemon 首启自动写 `roles/file_watcher` 的副作用；示例 role 后续应通过显式模板/初始化命令创建。
- 已补 `internal/v2/app`、`internal/skillcatalog`、`cmd/sesame` 关键路径单测。

仍需关注：

- `internal/v2/app` 的 HTTP handler 单测还可以继续扩展到任务、自动化、内存等路由。

### B. Web Console V2 迁移

范围：

- `web/console/src/api/**`
- `web/console/src/pages/**`
- `web/console/src/components/**`
- `web/console/src/App.tsx`
- `web/console/vite.config.ts`

状态：

- Web test/build 通过。
- 已修复 `.gitignore` 忽略前端测试的问题。
- 已修复 SSE 解析对 CRLF、多行 `data:` 的脆弱性。
- 已让 SSE 错误先进入 `error` 状态，再自动重连。

仍需关注：

- 需要真实浏览器联调 Chat、Roles、Tasks、TaskTrace、Context、Reports、Automations。
- `npm run build` 有大 chunk 警告，后续可做 route-level code splitting。

### C. Role / Skills 本地业务资产

范围：

- `roles/`
- `skills/`

结论：

- `roles/` 和顶层 `skills/` 是 workspace 本地业务资产，不上传到 GitHub 代码仓库。
- `reddit_monitor` 已移到 `/home/sauce/project/Workspace/sesame-agent-local-workspace/`，可继续作为本机 workspace 做真实业务联调。
- 代码仓库侧已保留 role/skill 运行能力：RoleSpec 读写、role tools、skill catalog、automation dispatch、report delivery。

已处理：

- `role.yaml` 中 `max_context_tokens`、`policy.can_delegate`、`policy.automation_ownership` 已纳入 V2 RoleSpec 读写/API/Web 表单；`max_tool_calls`、`max_runtime` 和 `max_context_tokens` 已进入运行期 turn 预算。
- `can_delegate=false` 已在 `delegate_to_role` 工具层硬拒绝；`automation_ownership` 已用于 `automation_control` 的 owner 粒度权限检查。
- watcher 脚本是 `0644` 时，V2 watcher 现在支持 `.py` 通过 `python3` 执行。
- 真实模型下已跑通 automation watcher -> owner role task -> report 回 main session 的事务链。

建议：

- 如需向其他环境分发 `reddit_monitor`，后续做模板/安装命令，把邮箱、代理、外发确认策略改成本地配置或 secret。

### D. V1 删除边界

范围：

- `cmd/eval/**`
- `cmd/lifecycle-test/**`
- `cmd/prefill-test-db/**`
- `internal/api/http/**`
- `internal/automation/**`
- `internal/cli/**`
- `internal/context/**`
- `internal/daemon/**`
- `internal/engine/**`
- `internal/store/sqlite/**`
- 以及其他 V1 运行层目录

风险：

- 这是 348 个删除文件级别的变更，已确认是正式迁移决策。
- 删除后 `/v1/*`、旧 CLI、Discord connector、旧 eval/lifecycle 工具都会消失。
- README 已更新到 V2 入口、8421 和 `internal/v2` 包结构；其他旧 docs 仍需要迁移或标注历史。

### E. 文档与验收

范围：

- `docs/v2-integration-checklist.zh-CN.md`
- `docs/backend-api.md`
- `docs/frontend-tui.md`
- `docs/frontend-web-console.md`
- `README.md`
- `README.zh-CN.md`
- `DESIGN_DECISIONS.md`

状态：

- 已新增 V2 联调清单。
- 已新增工作区整理清单和各提交边界 manifest。
- README 已更新到 V2 CLI/daemon/Web Console 基线。

仍需关注：

- 旧文档仍指向 V1 API 和旧目录结构。
- `DESIGN_DECISIONS.md` 仍有 V1 术语和路径残留，需要更新为 V2 语义或标注历史。

## 已修复问题

- 前端测试文件和 `web/console/src/test/` 不再被 `.gitignore` 忽略。
- Role turn 会优先使用 role session 的 `SystemPrompt`，避免被全局 main prompt 覆盖。
- Role turn 会把 `roleSpec.Model` 传入 model request。
- Automation watcher 增加 30s 默认超时、64KiB stdout/stderr 上限，并支持 `.py` 通过 `python3` 执行。
- Shell task 增加 30min 默认超时。
- Task output 文件增加 1MiB 默认上限。
- Shell tool 增加 64KiB 输出上限，保留 120s 默认超时。
- HTTP server 增加基础 read/header/idle timeout。
- Web SSE 解析支持 CRLF、多行 `data:`、可选空格和 payload seq fallback。
- `roles/` 和顶层 `skills/` 保持在 `.gitignore` 中，避免 workspace 本地业务资产进入代码仓库。

## 提交前必须确认

1. `internal/v2/**` 和新增 Web 页面必须纳入版本控制，不能只提交 V1 删除。
2. `roles/` 和顶层 `skills/` 不进入代码仓库；本地业务资产通过本地 workspace 管理。
3. V1 删除已确认是正式决策。
4. 旧 docs 是否在同一轮迁移到 V2，或者明确标注 V1 历史。
5. Fresh clone 只验证 runtime 能启动和创建新 role；具体业务 role/skills 通过本地安装或模板准备。

## 建议收口顺序

1. 先提交 D 组：V1 删除边界。
2. 再提交 A 组：V2 后端与入口。
3. 再提交 B 组：Web Console V2 迁移。
4. 确认 C 组 role/skills 只保留在本地 workspace，不进入代码仓库。
5. 最后提交 E 组：README/docs 全量对齐。
