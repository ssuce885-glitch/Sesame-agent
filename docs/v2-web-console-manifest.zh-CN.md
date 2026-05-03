# V2 Web Console Manifest

更新时间：2026-05-03

## 结论

Web Console 已迁移到 V2 API，可作为独立提交边界审查。

验证结果：

```bash
npm test
npm run build
```

通过。

唯一非失败项：

- Vite 提示 `dist/assets/index-*.js` 超过 500 kB。

旧组件引用检查：

```bash
rg -n "RuntimePage|UsagePage|RoleEditor|RoleDiagnostics|SummaryCards|UsageChart|NoticeBlock|runtimePageComponents" web/console/src
```

结果：没有命中。旧页面和旧组件删除边界成立。

## 建议提交

提交信息：

```text
migrate web console to v2 api
```

建议包含：

- `web/console/src/App.tsx`
- `web/console/src/api/**`
- `web/console/src/components/**`
- `web/console/src/pages/**`
- `web/console/src/i18n.tsx`
- `web/console/src/test/**`
- `web/console/vite.config.ts`

不要混入：

- V1 删除
- `internal/v2/**`
- CLI/config/system prompt
- role/skills 业务资产

## 当前页面

- Chat
- Reports
- Tasks
- Task Trace
- Roles
- Automations
- Context / Project State / Memory

## 已删除旧页面/组件

- `RuntimePage`
- `UsagePage`
- `runtimePageComponents`
- `RoleEditor`
- `RoleDiagnostics`
- `SummaryCards`
- `UsageChart`
- `NoticeBlock`

## 当前已覆盖能力

- V2 timeline API
- V2 SSE event stream
- robust SSE parsing for CRLF and multi-line `data:`
- error -> reconnecting connection state
- Role create/edit/test run
- Task list and trace
- Reports list
- Automation create/list/run history
- Project state and memory management

## 后续项

- Route-level code splitting to remove the large chunk warning.
- More tests around role form policy/budget fields.
- More tests around task trace/report edge cases.
