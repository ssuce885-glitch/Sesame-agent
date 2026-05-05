# Sesame V2 上下文治理与 Workflow 路线图

更新时间：2026-05-05

## 结论

V2 后续开发方向应围绕现有 runtime 主干继续演进：

```text
V2 runtime 稳定化
  -> 上下文治理
  -> Workflow 产品化
  -> skills/tools 权限化
  -> 架构治理制度化
```

这份路线图不是要恢复 V1 的复杂概念，而是把当前 V2 已经跑通的 `agent/session/tasks/reports/automation/roles/tools/store` 主链做厚。新增能力必须挂回现有主链，不能再造第二套 engine、scheduler、context system 或 role runtime。

## 当前基础

V2 已经具备这些基础能力：

- `internal/v2/agent`：turn loop、prompt assembly、tool loop、context budget、micro-compaction、threshold compaction、Project State refresh。
- `internal/v2/session`：主会话和 specialist role session 的队列与生命周期。
- `internal/v2/tasks`：后台 task、role task、runner、trace、输出汇聚。
- `internal/v2/reports`：task report 创建、queued report_batch 投递回 main session。
- `internal/v2/automation`：watcher、dispatch、owner role task。
- `internal/v2/roles`：文件型 role service，读写 `roles/<role_id>/role.yaml` 和 `prompt.md`。
- `internal/v2/tools`：tool namespace、role policy gate、workspace/file/shell 边界。
- `internal/v2/memory` 和 Project State API：workspace 级长期状态基础。
- `web/console`：Chat、Reports、Tasks、Task Trace、Roles、Automations、Context、Memory、Project State 的 UI 基础。

## 设计原则

### 1. 先观测，再治理

上下文系统的第一步不是增加更多自动写入，而是让用户和开发者能看到：

- 当前 turn 实际注入了哪些上下文。
- 哪些 Project State、Memory、Report 或历史摘要影响了回答。
- 哪些历史被 compact，原始证据在哪里。
- 哪些上下文过期、被降权或被排除。

### 2. 原始资产和上下文索引分离

不要一开始用新模型替换现有 `messages/reports/memories/project_state`。

建议保留现有原始资产，并新增上下文索引层：

```text
messages / reports / memories / project_state
  -> ContextBlock index
  -> Context selection
  -> prompt bundle
```

这样可以做查询、归档、回溯和审计，同时不破坏当前已跑通的对话、task、report 链路。

### 3. Workflow 复用现有 runtime

Workflow 不应成为新的 runner。第一版 workflow 只负责编排已有能力：

```text
trigger
  -> automation / manual dispatch
  -> task
  -> owner role session
  -> report
  -> approval gate
  -> main session
```

### 4. Prompt 是调度规则，不是知识仓库

稳定身份、安全边界和行为规则属于 prompt；项目知识属于 Project State；历史事实属于 Memory；业务流程属于 Workflow；工具规则属于 Tool Policy；skill 行为属于 Skill Manifest。

## 1. 上下文治理

### 1.1 Context Layer

V2 的上下文应分成多层，而不是只依赖聊天历史：

| 层级 | 用途 | 示例 |
| --- | --- | --- |
| Turn Context | 当前一轮对话和工具调用 | 用户刚说的话、当前 tool result |
| Session Context | 当前主会话连续目标 | 今天正在做 V2 审查 |
| Workspace Project State | 项目长期状态 | 当前架构、开放问题、决策、风险 |
| Role Memory | 角色自己的长期经验 | `reddit_monitor` 的监控规则和通知策略 |
| Task / Report Context | 子任务结果沉淀 | specialist task 最终报告、trace |
| Cold Archive | 可检索但不常驻的历史 | 老报告、旧调试记录、已完成任务 |

### 1.2 ContextBlock 最小模型

建议新增结构化上下文块模型。第一版以索引层为目标，不替换原始表。

```text
ContextBlock
- id
- type: goal | decision | fact | constraint | task_result | report | memory | warning
- owner: workspace | main_session | role:<role_id> | task:<task_id>
- visibility: global | role_only | task_only | private
- source_ref: message:<id> | report:<id> | memory:<id> | file:<path> | tool_call:<id>
- confidence
- importance_score
- evidence
- ttl / expiry_policy
- created_at
- updated_at
```

关键语义：

- `owner` 决定归属，例如 workspace、main session、role 或 task。
- `visibility` 决定是否能注入其他 role 或 main session。
- `source_ref` 和 `evidence` 用于审计和回溯。
- `ttl/expiry_policy` 用于 stale memory suppression。
- `importance_score` 用于 context selection，而不是直接把所有内容塞进 prompt。

### 1.3 Context Inspector

Web Console 的 Context 页面应升级为 Inspector：

- 展示当前模型实际会看到的上下文 preview。
- 展示被选中的 ContextBlock、Project State、Memory、Report 和 compact summary。
- 展示 compact boundary、snapshot id、原始区间。
- 展示某条记忆来自哪次 message、report、tool result 或文件。
- 展示哪些上下文被排除、过期或降权。
- 支持按 owner、visibility、source、type、importance 过滤。

建议 API：

```text
GET /v2/context/preview?session_id=<id>
GET /v2/context/blocks?workspace_root=<path>
POST /v2/context/blocks
PUT /v2/context/blocks/{id}
DELETE /v2/context/blocks/{id}
```

第一版必须只读 preview 优先，避免过早引入复杂写入策略。

### 1.4 Context Eval

上下文系统必须有评测，否则无法判断“记得好不好”。

| 测试 | 目标 |
| --- | --- |
| Recall Precision | 需要时能找回正确历史 |
| Stale Memory Suppression | 旧信息、过期信息不误导模型 |
| Compaction Fidelity | 压缩后不丢目标、约束、决策 |
| Role Isolation | 某个 role 的私有记忆不会泄漏给其他 role |

建议把 eval 固化为 V2 核心测试套件的一部分，和 automation transaction test 同级。

## 2. Workflow 产品化

### 2.1 Workflow 最小模型

第一版不要做复杂 DSL。先定义可审计的流程模板和运行记录。

```text
Workflow
- id
- name
- trigger: manual | schedule | watcher | webhook | file_change
- owner_role
- input_schema
- steps
- required_tools
- approval_policy
- report_policy
- failure_policy
- resume_policy
```

运行态：

```text
WorkflowRun
- id
- workflow_id
- state: queued | running | waiting_approval | completed | failed | interrupted
- trigger_ref
- task_ids
- report_ids
- approval_ids
- trace
- created_at
- updated_at
```

### 2.2 标准链路

当前已跑通的 automation 事务链应成为 workflow 的默认骨架：

```text
watcher emits needs_agent
  -> runtime dispatch lock
  -> owner role task
     or bound workflow trigger
  -> specialist session / workflow run
  -> task trace / approval trace
  -> report
  -> main report_batch turn
  -> policy-driven resume / pause / escalation
```

Workflow 只负责把这条链路产品化，而不是复制一套执行系统。

当前状态补充：

- `Automation` 已可选绑定 `workflow_id`。
- `workflow_id` 为空时，保持 `needs_agent -> owner role task` 兼容路径。
- `workflow_id` 非空时，watcher 会触发同 workspace 的 workflow run，automation run 审计记录同时保留 `task_id` / `workflow_run_id` 双路径兼容。

### 2.3 Approval Gate

外部副作用必须有一等公民的审批点。

| 动作类型 | 默认策略 |
| --- | --- |
| 读文件、读报告 | 可自动执行 |
| 写 workspace 文件 | 需要 role/tool policy 允许 |
| 执行 shell | 高风险，默认受限 |
| 外发消息 | 默认需要确认 |
| 修改 automation | 只能由有权限 role 执行 |
| 删除数据 | 默认需要确认或 dry-run |

建议新增：

```text
Approval
- id
- workflow_run_id
- requested_action
- risk_level
- summary
- proposed_payload
- state: pending | approved | rejected | expired
- decided_by
- decided_at
```

当前状态：

- Approval Gate MVP 已接入现有 `internal/v2/workflows` 主链，没有新增 runner 或 scheduler。
- workflow executor 遇到 `approval` step 时会创建一等 Approval 记录，把 `WorkflowRun.state` 置为 `waiting_approval`，并停止后续 steps。
- `GET/POST/PUT /v2/approvals` 与 `GET /v2/approvals` 已可用于创建、查询、决策审批；approval 决策后的 resume 仍在后续阶段。

### 2.4 官方 Workflow 模板

模板应放在 `examples/workflows/`，不要直接塞进 runtime。

优先模板：

- 监控类流程：watcher -> role task -> report -> optional notification。
- 故障排查流程：log signal -> triage role -> trace -> 修复建议。
- 研究 intake 流程：source scan -> summary -> memory/project state update。
- 发布检查流程：pre-release checklist -> tests -> reports -> final gate。
- 代码审查流程：diff -> role review -> findings -> patch suggestion。

当前状态：

- `examples/workflows/` 已落地首批官方模板：`monitoring-role-report.json`、`incident-triage.json`、`research-intake.json`、`release-checklist.json`、`code-review.json`。
- `examples/workflows/README.zh-CN.md` 已说明 Web Console / API 的复制方式，以及当前只支持 `manual` 和 automation 入口。
- Web Console `Workflows` 页面已从单行 trace 摘要升级为结构化事件视图，可直接查看 `event/state/kind/task_id/approval_id/message/time`。

## 3. Prompt Stack

### 3.1 Prompt 分层

系统提示词不应继续长成一个不可维护的大字符串。建议分层：

```text
Core Identity Prompt
  - Sesame runtime 身份
  - 本地优先
  - 不冒充模型供应商

Runtime Policy Prompt
  - 工具使用规则
  - 安全边界
  - task/report/role 行为

Workspace Prompt
  - 当前 workspace 项目状态
  - 当前目标
  - 当前约束

Role Prompt
  - role 专业职责
  - role 权限
  - role 汇报格式

Skill Prompt
  - 激活 skill 的具体行为规范
  - 示例
  - 禁止事项

Task Prompt
  - 当前任务输入
  - expected output
  - failure handling
```

### 3.2 Prompt Bundle 审计

每次 turn 应记录 prompt bundle 元数据：

```text
prompt_bundle_id
core_prompt_version
runtime_policy_version
role_prompt_version
active_skills
workspace_state_version
context_blocks_used
tool_policy_version
hash / fingerprint
```

当模型表现异常时，可以回溯到底是哪一层 prompt、哪个 context block 或哪个 tool policy 影响了行为。

## 4. Skills 体系

### 4.1 Skill 定位

Skill 不应只是额外 prompt。建议定义为：

```text
Skill = 行为规范 + 工具约束 + 示例 + 验收规则 + 安全边界
```

例如 email skill 应明确：

- 什么时候允许发。
- 发之前是否必须确认。
- 必填字段是什么。
- 不允许泄漏什么。
- 失败时如何报告。
- 发送后如何记录 report。
- 是否允许自动重试。

### 4.2 Skill Manifest

建议每个 skill 具备 manifest：

```yaml
id: email
version: 0.1.0
description: Email notification workflow skill
scope: workspace
requires_tools:
  - shell
  - file_read
risk_level: external_side_effect
approval_required: true
prompt_file: SKILL.md
examples:
  - examples/send_digest.md
tests:
  - tests/dry_run_email.md
permissions:
  network: false
  external_send: requires_confirmation
```

### 4.3 Skill 分类

| 类型 | 存放位置 | 用途 |
| --- | --- | --- |
| System Skills | 仓库内，随 runtime 分发 | 通用行为，如 automation normalizer |
| Workspace Skills | workspace 本地 | 用户业务技能，如 email、scraping、通知 |
| Role Skills | role.yaml 激活 | 某个 role 绑定的技能组合 |

### 4.4 Skill Lint / Eval

当前已提供基础命令：

```text
sesame skill lint
sesame skill test
sesame skill install
sesame skill pack
```

检查项：

- 是否声明 required tools。
- 是否声明风险等级。
- 是否包含真实 secret。
- 是否和 role policy 冲突。
- 是否要求不存在的工具。
- prompt 是否过长。
- 是否含有危险指令。

## 5. Tools 策略化

### 5.1 Tool Result Envelope

建议所有 tool 返回统一 envelope，便于 Web、TUI、agent、report、memory 一致消费：

```json
{
  "ok": true,
  "is_error": false,
  "summary": "...",
  "output": "...",
  "artifacts": [],
  "data": {},
  "warnings": [],
  "requires_followup": false,
  "visibility": "session",
  "risk": "low"
}
```

### 5.2 Tool Capability 分级

| 能力 | 例子 | 默认策略 |
| --- | --- | --- |
| read_workspace | file_read, grep | 允许 |
| write_workspace | file_write, file_edit | 受 role path 限制 |
| execute_local | shell | 高风险 |
| network_read | fetch, browser, API read | 需要显式开启 |
| external_send | email, slack, discord | 默认确认 |
| mutate_runtime | automation_control, role_update | 仅特权 role |
| destructive | delete, reset, kill | 默认拒绝或确认 |

### 5.3 Role Tool Policy

`role.yaml` 后续不应只支持 `allowed_tools/denied_tools`，还应支持细粒度策略：

```yaml
tool_policy:
  shell:
    allowed: true
    timeout_seconds: 60
    max_output_bytes: 32768
    allowed_commands:
      - rg
      - cat
      - go test
  file_write:
    allowed_paths:
      - docs/*
      - examples/*
    denied_paths:
      - .env*
      - .git/*
```

说明：

- path glob 采用 slash path 语义（等价于 `path.Match`），`*` 不跨目录，`docs/*` 不会匹配 `docs/a/b.md`，且不支持递归 `**`；需要多层目录时应显式列出每层 pattern。
- 当配置 `tool_policy.shell.allowed_commands` 时，仅允许字面量命令前缀和无 shell 控制/展开语义的参数；包含 `;`、`&&`、`||`、`|`、backtick、`$` / `$(`、重定向或换行的命令应直接拒绝。

### 5.4 优先补充工具

优先补这些 runtime 内部工具，而不是过早堆外部集成：

- `context_inspect`：查看当前模型将看到的上下文。
- `report_query`：按 role、task、time 查询报告。
- `workflow_create` / `workflow_run`：创建和运行业务流程模板。
- `skill_lint` / `skill_install`：管理技能。
- `tool_policy_explain`：解释为什么某工具被拒绝。
- `migration_check`：检查 workspace 是否适合从旧版迁移到 V2。
- `artifact_write`：规范输出文件、报告、图表。

## 6. 架构治理

### 6.1 包边界规则

建议固定以下边界：

```text
cmd/sesame
  只能依赖 config + internal/v2/app + internal/v2/client/tui

internal/v2/app
  负责 wiring、routes、daemon lifecycle

internal/v2/agent
  负责 turn loop、prompt assembly、context selection、tool loop

internal/v2/tools
  只负责工具定义和执行，不直接管理业务流程

internal/v2/tasks
  负责 task lifecycle 和 runners

internal/v2/automation
  负责 watcher、dispatch、policy resume/pause

internal/v2/reports
  负责 report 创建、投递、查询

internal/v2/store/schema
  负责持久化和 migration

web/console
  只消费 V2 API，不绕过后端访问内部状态
```

后续应通过 import boundary test 或静态检查防止反向依赖。

### 6.2 大功能 Manifest

每个大功能至少应包含：

- `docs/adr/<id>-<title>.md`
- `docs/manifests/<feature-name>.md` 或 `docs/v2-<feature>-manifest.zh-CN.md`
- migration note
- test plan
- rollback plan

### 6.3 版本与发布纪律

建议引入正式 tag：

```text
v2-alpha.1
v2-alpha.2
v2-beta.1
v2.0.0
```

每个版本说明：

- 支持哪些 API。
- 不支持哪些 V1 功能。
- 如何迁移 workspace。
- roles/skills 怎么处理。
- Web/TUI 有哪些已知问题。
- 是否适合生产使用。

### 6.4 CI 硬门槛

最低 CI：

```bash
go test ./...
go vet ./...
staticcheck ./...
git diff --check
cd web/console && npm test
cd web/console && npm run build
```

建议新增：

- import boundary test。
- SQLite migration test。
- fresh workspace smoke test。
- Web API contract test。
- prompt snapshot test。
- tool policy test。
- role/skill lint test。
- automation transaction test。
- context eval suite。

## 推荐阶段

### P0：稳定 V2 基线

目标：

- fresh workspace 能稳定启动。
- TUI/Web 能对同一 session 正常同步。
- role CRUD、task trace、report delivery、automation watcher 都可验收。
- 旧文档和 README 不再误导。
- CI 全绿。
- 给当前稳定点打 tag。

交付物：

- `v2-alpha.1` tag。
- `docs/migration-v1-to-v2.md`。
- `docs/v2-known-limitations.md`。
- `docs/v2-api.md`。
- `examples/workspace-minimal/`。
- CI checks。

### P1：上下文治理和 Workflow

目标：

- ContextBlock 模型。
- Context Inspector。
- Project State、Memory、Report 的统一选择策略。
- Workflow 模板。
- Approval Gate。
- Workflow run trace。

交付物：

- context block schema。
- context preview API。
- Web Context Inspector。
- `internal/v2/workflows`。
- workflow examples。
- context eval suite。

### P2：Skills 和 Tools 体系

目标：

- skill manifest。
- skill installer。
- skill lint。
- tool result envelope。
- tool capability policy。
- role tool policy。
- system skills / workspace skills / role skills 分层。

交付物：

- `internal/skillcatalog` v2。
- `examples/skills`。
- `sesame skill install`。
- `sesame skill lint`。
- tool policy schema。
- tool result schema。

当前状态（第一轮，已接入 runtime 主链）：

- `ToolDefinition.capabilities` / `risk` 已加入内置 tool 元数据。
- `ToolResult` 已补 `ok`、`summary`、`artifacts`、`warnings`、`requires_followup`、`visibility`、`risk` envelope 字段，并保留 `output` / `is_error` / `data` 兼容。
- role `tool_policy` 已接入 `roles -> tasks -> agent/tools runtime` 主链，支持：
  - `allowed`
  - `timeout_seconds`
  - `max_output_bytes`
  - `allowed_commands`
  - `allowed_paths`
  - `denied_paths`
- `tool_policy_explain` 已可解释当前 role 下某个 tool 的 allow/deny 原因，并返回 capabilities/risk。
- `allowed_tools` / `denied_tools`、`allowed_paths` / `denied_paths` 仍保留兼容。

当前状态（本轮已完成第一批低风险交付）：

- `internal/skillcatalog` 已支持更完整的 skill manifest/front matter 字段：
  - `id` / `name`
  - `version`
  - `description`
  - `scope`
  - `requires_tools`（兼容 `allowed_tools` / `allowed-tools` 旧别名；仅声明依赖，不自动解锁工具）
  - `risk_level`
  - `approval_required`
  - `prompt_file`
  - `examples`
  - `tests`
  - `permissions`
- `sesame skill lint <path...> [--workspace <root>]` 已接入，不启动 daemon 即可检查一个或多个模板文件的字段、工具依赖、secret 关键词和 prompt/body 完整性。
- `sesame skill test <path...> [--workspace <root>]` 已接入，会先复用 lint，再校验 `examples` / `tests` 相对路径文件存在、留在 skill 目录内、不是 symlink、可读且非空；整个过程不启动 daemon、不调用模型、不执行外部工具。
- `sesame skill install <template-path-or-name> --workspace <root>` 已接入，可从 `examples/skills/<name>` 或本地目录/markdown 文件安装模板到 workspace `skills/`。
- `sesame skill pack <template-path-or-name> --out <zip-path> [--workspace <root>]` 已接入，打包前会执行与 lint/test 等价的校验；通过后输出以 `installSkillName` 为根目录的 zip，目录模板保留全部普通文件，单 markdown 模板打成 `<skillName>/SKILL.md`，且不会覆盖已有目标文件。
- `examples/skills/` 已落地首批 skill template library。
- `examples/workflows/library.json` 已落地 workflow template library index，用于分发 metadata，不会自动加载到 runtime。

当前状态（仍未启用）：

- 外部 connectors 和真实外发执行链路

### P3：生态和外部集成

等 P0-P2 稳定后，再做：

- Discord 新版 connector。
- email/slack 通知。
- browser/web research。
- vision tool 重新接入。
- remote executor。

这些不应在当前阶段优先做，否则外部集成会在架构稳定前制造复杂度。

## 优先级摘要

最应该先做的五件事：

1. Context Inspector。
2. ContextBlock 最小模型。
3. Workflow Run 最小模型。
4. Approval Gate。
5. Prompt Bundle 记录。

判断标准：

- 是否增强现有 V2 主链，而不是引入第二套 runtime。
- 是否提高可观测性、可回溯性和可控性。
- 是否能被测试和验收。
- 是否避免把业务资产重新塞回代码仓库。
