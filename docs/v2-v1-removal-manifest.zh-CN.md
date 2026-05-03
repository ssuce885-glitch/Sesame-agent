# V1 删除边界 Manifest

更新时间：2026-05-03

## 结论

V1 删除边界已确认。当前删除的 348 个已跟踪文件属于 V2 迁移的预期结果，不再按异常恢复处理。

剩余代码引用检查：

```bash
rg -n "go-agent/internal/(api/http|automation|cli|context|daemon|engine|extensions|reporting|roles|scheduler|session|store/sqlite|task|tools|skills|permissions|connectors|runtimegraph|stream|workspace)" \
  --glob '!internal/v2/**'
```

结果：没有命中。剩余非 V2 代码没有继续 import 已删除的旧 V1 包。

## 删除分布

```text
57 internal/store
54 internal/tools
40 internal/cli
34 internal/engine
24 internal/api
18 internal/daemon
15 internal/automation
12 internal/connectors
11 cmd/eval
9 internal/task
9 internal/extensions
8 web/console
8 internal/context
7 internal/session
6 internal/scheduler
6 internal/reporting
5 internal/roles
5 internal/memory
3 internal/skills
3 internal/runtimegraph
3 internal/permissions
2 internal/workspace
2 internal/stream
2 internal/sessionrole
2 internal/instructions
1 internal/sessionbinding
1 cmd/prefill-test-db
1 cmd/lifecycle-test
```

## 建议提交

提交信息：

```text
remove legacy v1 runtime
```

推荐只包含删除文件，不混入 V2 新增实现、Web 迁移、role/skills 或文档修改。

可用于审查的路径范围：

```bash
git diff --name-only --diff-filter=D
```

## 风险说明

这是最大风险提交，因为它删除了旧 runtime 的主要实现面：

- V1 HTTP API
- V1 CLI/TUI
- V1 daemon/runtime wiring
- V1 engine/context/memory/reporting
- V1 sqlite store
- V1 tools
- V1 automation/scheduler/task/session

合入前必须确认 V2 已覆盖当前需要的最小业务闭环：

- `cmd/sesame` 能启动/连接 V2 daemon
- TUI 能发送消息并实时渲染
- Web Console 能使用 V2 API
- Role CRUD 和 role tools 可用
- Specialist task 能运行并可 trace
- Automation watcher 能 dispatch role task
- Reports 能在 TUI/Web 查看
