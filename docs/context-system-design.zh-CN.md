# Sesame Context System Design

更新时间：2026-05-06

## 目标

Sesame 的上下文系统面向 workspace-scoped personal assistant runtime，而不是单一 coding agent。第一版设计服务长期 role、automation、workflow、task 和 report 的持续运行，让 main agent 能监督全局，让 specialist role 能看到自己的工作台，同时避免把长期状态、偏好、历史记录和权限规则混成一类。

## 核心原则

- `AGENTS.md` 是 workspace 内最高持久规则源。
- Global system prompt 是全局硬规则，用户不改代码或配置时不可变。
- 当前用户消息或 task prompt 可以本轮临时覆盖 `AGENTS.md`，但 agent 必须说明冲突，并询问是否需要更新 `AGENTS.md` 让规则持久化。
- 第一版不提供用户自定义的跨 workspace prompt。
- Runtime permission、role policy、tool policy 是硬执行边界，不能被 prompt、skill、memory 或当前用户消息绕过。
- Project State 的产品语义改为 Workspace Runtime State：它是运行状态 dashboard，不是规则源，也不是 archive。
- main agent 需要运营/监督视图，不需要每个 role 的完整内部上下文。
- role 需要自己的 Role Runtime State，用于长期职责、当前任务、owned automation、open loops 和 recent material outcomes。
- memory/context block 使用同一套底层存储，按 owner、visibility、scope 隔离。
- 第一版检索使用 SQLite FTS + scope filter + importance/recency 排序，不做向量检索。
- 压缩用于 continuity 和 supervision，不用于制造 authority。

## Context Source Inventory

### Always Considered

| 来源 | Scope | 用途 | 注入策略 |
| --- | --- | --- | --- |
| Global system prompt | global | 全局身份、硬规则、runtime invariants | 每 turn 必注入 |
| `AGENTS.md` | workspace | workspace 持久最高规则 | 每 turn 必注入，记录 hash/truncation |
| Current user message | turn | 本轮用户意图 | 每 turn 必注入 |
| Task/delegation prompt | task/role | role 本轮任务目标 | task/role turn 必注入 |
| Role prompt / role.yaml | role | specialist 职责、model、budget、policy | role turn 必注入 |
| Visible tool definitions | turn | 当前可调用能力 | 每 turn 由 runtime gate 计算 |

### Selected Or Scoped

| 来源 | Scope | 用途 | 注入策略 |
| --- | --- | --- | --- |
| Workspace Runtime State | workspace | main-facing 运营/监督 dashboard | main turn 注入摘要 |
| Role Runtime State | role/task | role-facing 工作台 | role/task turn 注入摘要 |
| ContextBlock | workspace/role/task | 原子事实、决策、偏好、open loop、outcome | 按 scope/visibility/importance/expiry 选择 |
| Memory | workspace | durable note 和可检索知识 | FTS 检索后按可见性过滤 |
| Reports | session/task/role | material outcomes 和 task final reports | 默认不全量注入，按引用或 state 摘要进入 |
| Task trace | task | 调试和继续任务的证据 | 按需工具展开 |
| Workflow/approval trace | workflow_run | workflow 运行状态和审批点 | active/waiting 状态进入 Runtime State，明细按需展开 |
| Automation runs | automation | watcher 结果和 dedupe 状态 | active/error/material outcome 进入 Runtime State |
| Conversation history | session | 对话连续性 | 近期消息 + compact summary |
| Active skill instructions | turn/role | skill 操作规范 | role 默认 skill 或 `skill_use` 激活后注入 |

## Priority Model

权限和规则需要分成两类看。

### Hard Execution Constraints

```text
Global system prompt
Runtime permission profile
Role/tool policy
```

这些不能被当前用户消息、`AGENTS.md`、role prompt、skill、memory 或 state 覆盖。

### Turn Execution Priority

```text
Global system prompt / runtime hard constraints
> Current user message or task prompt
> AGENTS.md
> Role prompt / skill instructions
> Preferences / ContextBlocks / Workspace or Role Runtime State
> Memory / conversation history / reports
```

当前用户消息可本轮临时覆盖 `AGENTS.md`。发生冲突时，agent 应说明冲突，并询问用户是否要更新 `AGENTS.md`。

### Durable Policy Authority

```text
Global system prompt
> AGENTS.md
> Role prompt / skill instructions
> Preferences / Runtime State / Memory / History
```

`AGENTS.md` 是 workspace 内最高持久规则。Workspace Runtime State、Role Runtime State、Memory 和 Reports 都不能成为隐藏规则源。

## Runtime State

第一版 Runtime State 使用 Markdown 固定 section。它是 dashboard，不是原始记录，也不是唯一事实源。

原始事实仍在：

```text
v2_tasks
v2_reports
v2_workflow_runs
v2_approvals
v2_automation_runs
v2_context_blocks
v2_memories
v2_messages
```

### Workspace Runtime State

给 main agent 的运营/监督视图。main 应知道所有 role 当前在做什么，但不默认看到 role 完整内部上下文。

建议 section：

```markdown
# Workspace Runtime State

## Workspace Objectives

## Role Workstreams

## Active Automations

## Active Workflow Runs

## Workspace Open Loops

## Recent Material Outcomes

## Runtime Health

## Watchpoints

## Important Artifacts
```

`Role Workstreams` 是核心 section，每个 role 保持 1-3 行：

```text
role_id; state; responsibility; active task/workflow/automation refs; latest material report; open loop; next action/check.
```

main 默认可见：

- role workstream summary
- role_shared summary
- active tasks/workflows/approvals
- automation health
- material reports
- workspace/global context

main 默认不可见：

- role_only memory
- task_only internals
- private drafts
- full task logs
- every automation run detail

### Role Runtime State

给 specialist role 的工作台视图。

建议 section：

```markdown
# Role Runtime State: <role_id>

## Responsibility

## Owned Automations

## Active Work

## Open Loops

## Recent Material Outcomes

## Relevant Workspace Context

## Watchpoints

## Important Artifacts
```

role 默认可见：

- workspace/global context
- 自己的 role_only 和 role_shared context
- 当前 task 的 task_only context
- 自己 owned automations
- 与当前 task/workflow 相关的 reports/traces
- 少量 workspace-wide watchpoints

role 默认不可见：

- 其他 role 的 role_only memory
- main_only memory
- 不相关 task_only context
- private context
- 不相关 role 的内部 workstream 明细

## Memory And ContextBlock Visibility

第一版不拆 main memory 表和 role memory 表。底层共存储，逻辑上按 owner、visibility、scope 过滤。

### Owner

```text
user
workspace
main_session
role:<role_id>
task:<task_id>
workflow_run:<id>
automation:<id>
```

### Visibility

```text
global
workspace
main_only
role_only
role_shared
task_only
private
```

默认写入：

| 写入者 | 默认 owner | 默认 visibility |
| --- | --- | --- |
| user | user | workspace |
| main parent | main_session | workspace 或 main_only |
| role | role:<role_id> | role_shared |
| task | task:<task_id> | task_only |
| automation/workflow | automation:<id> / workflow_run:<id> | role_shared 或 workspace |

promotion 第一版只允许 user 或 main parent 执行：

```text
role_only -> role_shared -> workspace
```

role 不能直接把私有记忆升级成 workspace 级事实。

## ContextBlock Types

第一版类型建议：

```text
preference
decision
constraint
fact
status
open_loop
outcome
artifact
watchpoint
warning
note
```

语义：

- `preference`：用户长期协作偏好或 workspace/role 偏好，不是 prompt。
- `decision`：用户或 main 确认过的决策。
- `constraint`：低于 `AGENTS.md` 的约束引用。
- `status`：当前状态，容易过期。
- `open_loop`：需要继续处理的事项。
- `outcome`：material task/report/workflow 结果。
- `watchpoint`/`warning`：运行风险或需要持续关注的问题。

## Memory Retrieval V1

第一版使用 SQLite FTS，不做 embedding/vector。

检索流程：

1. 根据 execution scope 过滤候选可见性。
2. 用 current user message 或 task prompt 作为查询文本。
3. 调用现有 `MemoryRepository.Search`，优先走 `v2_memories_fts`。
4. 合并 ContextBlock 候选。
5. 按 importance、confidence、recency、type boost 排序。
6. 按 token budget 裁剪。
7. 在 context preview 中记录 query、selected sources、why_selected 和 dropped reason。

类型排序建议：

```text
preference / constraint / decision
> open_loop / status / watchpoint
> outcome / artifact / fact
> note
```

## Compression

压缩分两类。

### Conversation Compression

输入：

```text
messages
tool results
compact snapshots
```

输出：

```text
conversation summary
```

保留仍影响当前任务的目标、决策、开放问题、近期 tool result。大段日志、旧 diff、重复解释、完成流水账应压缩或丢弃。

### Runtime State Compression

输入：

```text
tasks
reports
workflow runs
approvals
automation runs
role_shared context blocks
memories
```

输出：

```text
Workspace Runtime State Markdown
Role Runtime State Markdown
```

Runtime State 更新应替换 section，而不是追加流水账。每条摘要应短、有 source ref、有 owner/scope、有状态。

触发：

- task 完成
- report 创建
- workflow run 状态变化
- approval 更新
- automation run 创建
- role 创建/更新
- daemon recovery
- 用户显式要求更新状态

## Audit And Preview

Context preview 应最终展示：

```text
context_package_id
system_prompt_hash
agents_md_path/hash/truncated
workspace_runtime_state_hash
role_runtime_state_hash
context_blocks_used
memories_used
reports_used
messages_selected_range
visible_tools
token_budget_by_source
dropped_context_reason
conflicts_detected
```

第一版可以先做纯逻辑模型和 preview fields，不要求一次性完成所有 API 和 UI。

## MVP Rollout

1. 新增纯逻辑 context assembler/model 包，定义 source、package、scope filter、runtime state Markdown builder。
2. 保留现有 `ProjectState` schema，产品语义改为 Workspace Runtime State。
3. 在 context preview 中逐步展示 Runtime State、selected ContextBlocks、selected Memories。
4. 之后再把 agent prompt assembly 从 `internal/v2/agent` 抽到 context assembler。
5. 暂不做 vector retrieval、不新增跨 workspace custom prompt、不新增第二套 role runtime。
