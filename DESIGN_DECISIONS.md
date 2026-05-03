# Sesame 设计决策记录

## 一、为什么自动化必须走 3 层工作流，而不是 main_parent 直接调 automation_create_simple

### 决策

用户请求创建自动化时，流程是：

```
main_parent → delegate_to_role → specialist → skill_use → automation_create_simple
```

main_parent 被禁止调用 `automation_create_simple`。prompt 中明确写死：*"main_parent must not call automation_create_simple. If you think you need it, stop and delegate to the owning specialist role instead."*

### 原因

1. **所有权模型**：自动化必须归属于 specialist role（`owner: role:<role_id>`），不能挂在 main_parent 下。这是因为：
   - 自动化的 watcher 脚本、dedupe 逻辑、执行权限都跟 role 绑定
   - role 有独立的 session、context head、budget，生命周期独立于 main_parent
   - 删除 role 时，其拥有的自动化一起清理

2. **能力门控**：`automation_create_simple` 工具在 main_parent 侧 `IsEnabled` 返回 false（通过 `RoleSpec == nil` 检测），只有 specialist 能看到和调用。这防止 main_parent 越权创建无主自动化。

3. **skill 激活链路**：`automation_create_simple` 工具的注册表中声明了 `capability_tags: ["automation-standard-behavior"]`。specialist 必须先通过 `skill_use` 激活 `automation-standard-behavior` 和 `automation-normalizer`，这两个系统 skill 的 `capability_tags` 匹配后，工具才可见。这一层门控确保 specialist 在执行操作前加载了正确的行为指令。

### 关键实现点

- `internal/tools/builtin_automation_native.go`：`IsEnabled` 检查 `cfg.RoleSpec != nil`
- `internal/tools/builtin_delegate_role.go`：`IsEnabled` 检查 `cfg.RoleSpec == nil`（只有 main_parent 能委托）
- `internal/sessionrole/sessionrole.go:mainParentPrompt`：prompt 中写入自动化工作流指令
- `internal/engine/prompt.go:defaultGlobalSystemPrompt`：全局 prompt 中补充角色管理和自动化工作流章节

---

## 二、为什么 Owner Task 复用 session 但强制新 context_head

### 决策

`agentTaskExecutor.resolveTaskRunContext()` 在目标 specialist 角色存在时调用 `EnsureSpecialistSession(..., forceNewHead=true)`。该方法仍然复用已有 role session（`GetSpecialistSessionID`），但每次 Owner Task 都创建新的 context head，让单次任务执行从干净上下文开始。

### 原因

1. **保留 role 生命周期**：role session 仍然是持久的，任务继续进入 `session.Manager` 队列，权限、budget、reporting、role 绑定不变。
2. **隔离任务上下文**：Owner Task 的 tool 调用、搜索结果和最终回复不再堆在同一个 head 下，避免后续任务读到上一轮的冗余执行历史。
3. **状态转移走 durable memory / report**：跨任务需要保留的信息应通过 memory、report、automation run 记录等结构化通道传递，而不是依赖完整对话历史堆积。

### 代价

1. 同一 specialist 的连续 Owner Task 不再自动看到上一轮完整对话。如果需要跨任务去重或状态连续性，必须写入 durable memory 或 automation run 状态。
2. context head 数量会随 Owner Task 增长。该修复只做隔离，不改变 compaction / summarization 逻辑。

---

## 三、Budget 默认值是兜底下限，不是上限

### 决策

`effectiveRoleBudget()` 中 `clampPositiveInt` / `clampPositiveFloat` / `clampRuntimeBudget` 的逻辑在修复前是错误的：把 `defaultValue` 当成了天花板。role.yaml 里设置 `max_context_tokens: 64000` 会被硬编码默认值 16000 覆盖。

修复后：`defaultBudget` 只是兜底（role 未设置时使用），role 设置的值无论比默认高还是低都直接生效。

### 修复前后对比

```go
// 修复前 (BUG)
func clampPositiveInt(roleValue, defaultValue int) int {
    if roleValue <= 0 {
        return defaultValue
    }
    if defaultValue > 0 && roleValue > defaultValue {
        return defaultValue  // 64000 > 16000 → 返回 16000，role 值被丢弃
    }
    return roleValue
}

// 修复后
func clampPositiveInt(roleValue, defaultValue int) int {
    if roleValue <= 0 {
        return defaultValue
    }
    return roleValue  // role 设置什么就是什么
}
```

### 默认值选择

| 参数 | 旧值 | 新值 | 理由 |
|------|------|------|------|
| `MaxContextTokens` | 16000 | 128000 | 200K 上下文模型普遍，16000 严重浪费；128K 留出余量给 system prompt 和 tool 结果 |
| `MaxToolCalls` | 20 | 100 | 一次复杂自动化（搜索+读文件+确认+执行）远超过 20 步 |
| `MaxCost` | 5.0 | 删除 | Web UI 没有用量统计，成本追踪链路空转；预算限制逻辑整体移除 |
| `CostUSD` | 有 | 删除 | 同上，types / store / engine 中所有 CostUSD 字段清理 |

### 原则

- role.yaml 的 budget 设置代表**角色能力上限**，应该比全局默认更宽松
- 全局默认是**安全兜底**，防止没有配置 budget 的角色失控
- 这个决策与"自动化应该由 specialist 拥有"一致：把控制权下放给角色定义者

---

## 四、工具可见性的双层门控

### 决策

哪些工具对当前 turn 可见，由两层检查决定：

**第一层：`RoleSpec` 门控**
- `IsEnabled` 检查 `cfg.RoleSpec != nil`：只有 specialist role 可见（如 `automation_create_simple`）
- `IsEnabled` 检查 `cfg.RoleSpec == nil`：只有 main_parent 可见（如 `delegate_to_role`、`role_create`）

**第二层：`capability_tags` 门控**
- 工具注册时声明 `capability_tags: ["automation-standard-behavior"]`
- 用户/system skill 在 `skill_use` 后，其 `policy.capability_tags` 被注入当前 turn
- `registry_execute.go` 中匹配：工具 tags 与当前 turn 的激活 tags 取交集

### 原因

1. **防止越权**：main_parent 不能创建自动化，specialist 不能委托他人，这是结构性的
2. **渐进式能力解锁**：specialist 初始只有基础工具，`skill_use automation-standard-behavior` 后才获得 `automation_control` 和 `automation_create_simple`。这确保模型先读了 skill 的 system prompt（包含操作规程），再获得工具权限
3. **可审计**：每次工具调用都有明确的激活链（哪个 skill 在什么时间点被激活）

---

## 五、廉价检测优先（Cheap Scanning First）

### 决策

自动化 watcher 使用纯脚本（Python/Shell）做高频轮询，只在脚本输出 `needs_agent` 或 `needs_human` 时才调模型。

### 原因

1. **成本控制**：Reddit API 轮询每分钟一次，用模型每分钟烧一次太贵。纯脚本 + cron 几乎零成本
2. **确定性过滤**：脚本可以精确判断"这个帖子 ID 上次已经处理过"（dedupe_key）、"分数不够 50 跳过"、"不是 AI 相关跳过"。这些确定性规则不需要模型
3. **信号摘要**：watcher 脚本输出 JSON，包含 `summary` 和 `facts` map，直接注入 Owner Task prompt。模型收到的不是 "去翻 Reddit" 而是 "帖子 X：标题 Y，分数 Z，已有评论数 W"，一步到位

### 链路

```
watcher_reddit_ai.py (每分钟 cron)
  ↓ 筛选热门 AI 帖子，输出 needs_agent + facts JSON
watcher.go: Tick()
  ↓ 检测 needs_agent 信号，构造 TriggerEvent
SimpleRuntime.HandleMatch()
  ↓ dedup → 构建 Owner Task prompt → Create TaskTypeAgent
tasks.AgentRunner
  ↓ EnsureSpecialistSession → session.Manager.SubmitTurn
reports.Service
  ↓ 收集 task result → 投递到 main_parent report_batch turn
main_parent
  ↓ 处理报告，向用户汇报
```

---

## 六、报告投递的冷却与去重

### 决策

V2 的 `internal/v2/reports.Service` 在 task 完成时创建 durable report，并在目标 session 空闲时提交一个 `report_batch` turn 给 main agent。若目标 session 正在执行用户 turn 或已有排队 turn，report 保持 `delivered=false`，daemon 的短周期 flush 会在 session 空闲后重新投递。

### 原因

1. **不打断用户 turn**：main session 忙碌时只写 durable report，不插队运行 report turn。
2. **不丢结果**：session 未注册或正在忙碌时，report 保持未送达，后续 flush 会继续尝试。
3. **合并报告**：一次 flush 会把同一 session 的所有未送达 reports 合并成一个 `report_batch` turn。
4. **去重**：`dedupe_key` 确保同一事件不会重复创建 Owner Task。`ClaimSimpleAutomationRun` 使用原子 upsert 实现分布式锁。

---

## 七、Role 的文件化设计

### 决策

Role 定义存储在 `roles/<role_id>/` 目录下，包含：
- `role.yaml`：元数据（display_name、budget、policy、skill_names）
- `prompt.md`：specialist 的 system prompt
- `automations/<automation_id>/`：脚本文件（watcher + detector）

不使用数据库存储 role 定义。数据库只存 session、turn、context items 等运行时状态。

### 原因

1. **版本控制友好**：role 定义可以 `git commit`，回滚方便
2. **可编辑性**：用户可以直接用编辑器修改 prompt.md 然后重启生效
3. **去中心化分发**：安装 role 就是 `cp -r roles/reddit_monitor ./`，不需要 SQL migration
4. **`.role-versions/` 机制**：`role_create` 工具自动保存每次修改的快照到 `.role-versions/000001.yaml`，提供基本的版本回溯能力

---

## 八、Skill 与 Role 的职责分界

### 现状

SKILL.md 目前承载了混合语义——既有交互式工具定义（`agent-browser`、`scrapling`），又通过 `triggers`、`agent.*`、`policy.capability_tags` 承担了自动化能力门控。

### 待做

- SKILL.md 格式向 Claude Code 官方规范靠拢，保留 `name`、`description`、`allowed-tools`、`when_to_use`
- `triggers` 列表 → 移到 watcher 配置
- `policy.capability_tags` → 移到 role.yaml
- `agent.*` 子代理定义 → 用 Claude Code 的 `context: fork` + `agent` 替代
- `policy.allow_full_injection` → Sesame 特有，保留（控制 skill body 是否注入上下文的开关）
