# Context System TODO

更新时间：2026-05-07

本文记录 Sesame 上下文系统近期已经完成的工作、正在讨论的设计点，以及后续可执行 TODO。它用于公开说明项目进展，也用于避免把已实现能力、设计目标和未来规划混在一起。

## 目标

Sesame 的上下文系统不是为了把所有历史都塞进 prompt，而是为了让长期运行的 workspace agent 能在规则、状态、记忆、报告和历史之间保持清晰边界。

核心目标：

- 让 main agent 拥有 workspace 监督视图。
- 让 specialist role 拥有自己的长期工作台。
- 让 task、automation、workflow 的结果能通过 report 回到主线。
- 让 Memory / ContextBlock 成为可检索、可审计、可控可见性的长期上下文来源。
- 让真实 prompt 和 Web Console context preview 最终使用同一套组装逻辑。

## 已完成

### 规则与指令来源

- 已支持 workspace 根目录下的 `AGENTS.md` 作为持久规则来源。
- 每个 turn 会读取并注入 `AGENTS.md`，同时提示模型不要把它当作新的用户请求。
- 已支持 current turn instruction conflict：当前用户消息或 task prompt 可以本轮临时覆盖 `AGENTS.md`。
- 发生 instruction conflict 时，会把冲突注入 prompt，并记录 `instruction_conflicts_detected` 事件。

### Runtime State

- 已将原 `ProjectState` 产品语义收敛为 Workspace Runtime State。
- main turn 会注入 Workspace Runtime State。
- role turn 会注入对应 role 的 Role Runtime State。
- main turn 默认不注入 role 的完整内部 runtime state。
- Role Runtime State 已有独立存储表 `v2_role_runtime_states`。

### Scope 与可见性模型

- 已实现 `contextasm` 纯逻辑包。
- 已定义 execution scope：`main`、`role`、`task`。
- 已定义 owner：`user`、`workspace`、`main_session`、`role:<id>`、`task:<id>`、`workflow_run:<id>`、`automation:<id>`。
- 已定义 visibility：`global`、`workspace`、`main_only`、`role_shared`、`role_only`、`task_only`、`private`、`session`。
- 已实现 `FilterVisibleBlocks` / `IsVisibleToScope`，用于 Memory / ContextBlock 的可见性过滤。

### Memory 与 ContextBlock

- 已有 Memory 存储、搜索、写入和读取工具。
- `memory_write` 会根据当前执行上下文写入默认 owner / visibility。
- `recall_archive` 和 `load_context` 会按 execution scope 做可见性过滤。
- 已有 `v2_context_blocks` 表和 ContextBlock CRUD API。
- Web Console context preview 已能展示 ContextBlock、Memory、Reports 的 available / included / excluded 状态。

### 历史裁剪与压缩

- 已实现 conversation compaction。
- 触发 compaction 时，会保存 raw message snapshot。
- 后续上下文从 compact boundary 后继续，并携带 compact summary。
- 已实现 tool result micro-compaction，旧的大型 tool result 会被清理成短 preview，避免长期占用上下文窗口。

### Web Console 与 API

- 已有 `/v2/context/preview`。
- 已有 `/v2/context/blocks` CRUD。
- 已有 `/v2/project_state` 读写接口。
- Context 页面可以看到当前 prompt preview、runtime state、context blocks、memory 和 reports 的状态。

## 正在讨论

### ContextBlock / Memory 是否默认自动注入

当前状态：

- Memory / ContextBlock 已经可写、可查、可 preview。
- 但它们还没有进入 agent turn 的自动选择注入链路。

讨论点：

- 默认自动注入会提升连续性，但也可能引入过期事实、低相关内容或隐藏规则污染。
- 更稳妥的方向是先实现 selector，再按 relevance、visibility、importance、recency 和 token budget 选择。

### Runtime State 的更新方式

当前状态：

- Workspace Runtime State 由 turn 后异步 LLM 总结更新。
- Role Runtime State 已有存储和注入，但还需要更完整的事件驱动更新链路。

讨论点：

- 只靠 LLM 总结容易滞后或漂移。
- 后续应结合 task、report、workflow、automation run 等结构化事件做 section-level 更新。

### Preview 与真实 prompt 的一致性

当前状态：

- Context preview 能展示将会包含的主要 prompt 内容。
- ContextBlock / Memory 在 preview 中展示为 available，但真实 agent prompt 不会自动注入。

讨论点：

- 后续需要让 preview 和真实 prompt assembly 使用同一套 Context Package。
- Preview 应明确展示 selected、available、dropped 的原因。

### Context authority 边界

当前状态：

- `AGENTS.md` 是 workspace 持久规则源。
- Runtime State 明确不是规则源。
- Memory / ContextBlock 也不应成为隐藏规则源。

讨论点：

- `preference`、`constraint`、`decision` 等 ContextBlock 类型需要更明确的优先级和来源审计。
- 当前用户消息临时覆盖持久规则时，是否自动建议写回 `AGENTS.md` 或 ContextBlock，需要继续打磨交互。

## 下一步 TODO

### P0

- [ ] 实现 Context Selector，从 Memory / ContextBlock / Reports 中选择当前 turn 相关上下文。
- [ ] 将 selector 接入 `Agent.RunTurn`，让 selected context 真正进入 prompt。
- [ ] 让 `/v2/context/preview` 与真实 prompt assembly 共享同一套 selector。
- [ ] 在最终模型请求前增加 hard budget gate，避免 system prompt + current turn 本身超出上下文窗口。

### P1

- [ ] 定义统一 `ContextPackage`，包含 included blocks、source refs、token estimate、why_selected、dropped reason。
- [ ] 为 dropped context 记录原因：not visible、expired、low relevance、budget exceeded、duplicate。
- [ ] 增加 Memory / ContextBlock 的类型排序：preference、constraint、decision、open_loop、status、watchpoint、outcome、artifact、fact、note。
- [ ] 为 Workspace Runtime State 增加 section-level 更新，而不是整段替换。
- [ ] 为 Role Runtime State 增加 task/report/automation 触发更新。

### P2

- [ ] 在 Web Console 中展示 visible tools、token budget by source、selected message range。
- [ ] 为 `AGENTS.md` 记录 hash、truncated 状态和 source ref。
- [ ] 增加 ContextBlock promotion 流程，例如 `role_only -> role_shared -> workspace`。
- [ ] 增加 context package snapshot，便于复盘某次 turn 到底看到了什么。
- [ ] 为长期自动化场景增加 stale context 检测和清理建议。

## 当前公开说明口径

可以对外说明：

> Sesame 已经实现了 workspace / role runtime state、`AGENTS.md` 注入、instruction conflict、conversation compaction、tool result micro-compaction，以及 Memory / ContextBlock 的 owner / visibility / scope 过滤。当前 Memory / ContextBlock 仍处于可写、可查、可 preview 阶段，自动选择注入和统一 Context Package 是下一阶段重点。

