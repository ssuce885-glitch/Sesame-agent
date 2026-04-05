# Console Event Stream Reliability Design

**Date:** 2026-04-05  
**Status:** Draft  
**Scope:** 修复 go-agent web console 的 SSE 事件流恢复、前端状态归并、服务端事件投递可靠性问题

---

## 1. Goals

1. 重连、半开连接、慢订阅者积压等场景下，console 不再静默挂起或静默丢事件。
2. 前端只按严格单调递增的事件序列更新 UI，重复事件、过期事件、序列缺口都不会污染渲染状态。
3. SSE 连接断开后，客户端优先按 `after=<lastAppliedSeq>` 续传；一旦检测到 gap，自动回退到 snapshot 恢复。
4. 工具调用块的 `args_preview`、`result_preview`、`status` 分别独立合并，不允许后到事件用 `undefined` 覆盖先到字段。
5. assistant delta 只能追加到同一 `turn_id` 的未完成 assistant block，重连后不会串到错误消息。
6. 乐观消息 ID 和 `client_turn_id` 使用 UUID，消除 `Date.now()` 冲突窗口。
7. 服务端事件总线不再在缓冲区满时静默丢事件；慢客户端将被显式断开并走标准恢复流程。

## 2. Non-Goals

- 不重写 console 为 `fetch + ReadableStream` 自定义 SSE transport，仍然使用浏览器原生 `EventSource`。
- 不修改现有业务事件的 JSON 形状，已持久化事件仍然保持 `types.Event` 结构。
- 不改造 console 的视觉样式、布局和信息密度；本次只处理可靠性和状态正确性。
- 不新增多标签页协同、离线缓存或跨页面共享游标。

---

## 3. Problem Inventory

### 3.1 已确认问题

1. 客户端没有显式的 seq guard，收到事件后直接应用，当前实现无法在前端检测重复事件、过期事件和序列缺口。
2. `App.tsx` 对同一条 SSE `message.data` 做了多次 `JSON.parse`，存在重复解析和潜在分叉处理逻辑。
3. console 只依赖 `EventSource.onerror` 触发重连，没有 liveness timeout；TCP 半开时 UI 会一直停留在 open 状态。
4. 当前重连是固定 1.5s 重试，没有指数退避，也没有在 gap 场景回退到 snapshot 恢复。
5. `chatState.ts` 的工具块更新使用 spread 合并，字段保留规则依赖服务端事件总是“补全旧字段”，脆弱且不可证明。
6. assistant delta 归并依赖 `turn_id + status !== completed`，但 timeline snapshot 的 assistant block 没有保留 `turn_id`，live block 与 snapshot block 的归并键不一致。
7. 乐观 block ID 与 `client_turn_id` 使用 `Date.now()` 生成，理论上存在同毫秒冲突。
8. `stream.Bus` 在订阅者缓冲区满时直接 `default:` 丢弃事件，客户端不会被告知缺失，属于静默数据损坏。
9. HTTP 事件流接口只提供 `Subscribe`，没有 unsubscribe 生命周期，订阅者清理不完整。

### 3.2 当前已有能力

1. 服务端事件已持久化到 SQLite，并按 `seq asc` 回放历史事件。
2. `/v1/sessions/:id/timeline` 已能返回快照和 `latest_seq`。
3. SSE handler 先订阅 live bus，再回放历史事件，能够覆盖“回放期间有新事件发布”的 handoff 场景。

本次设计基于这些现有能力补齐前端检测、恢复路径和服务端投递保证，而不是推倒现有协议。

---

## 4. Chosen Architecture

采用“现有协议最小扩展 + 端到端恢复闭环”的方案：

1. 保留现有 `/v1/sessions/:id/events?after=<seq>` 和 `/timeline` 两个入口。
2. 服务端为 SSE 增加非持久化 `keepalive` 事件，并在 `after` 超前时做钳制处理。
3. 前端维护 `lastAppliedSeq`，所有业务事件在进入 reducer 前必须先通过 monotonic seq 检查。
4. 正常断线时继续使用 `after=<lastAppliedSeq>` 续传；一旦发现 gap，关闭 SSE、重新抓取 timeline snapshot、用 snapshot 重建状态，再从 snapshot 的 `latest_seq` 重新订阅。
5. 服务端 bus 改为“背压即断开该订阅者”，禁止静默丢事件；客户端收到断流后走同一套重连/恢复逻辑。
6. timeline snapshot 与 live event 使用同一组归并键，保证 `turn_id` 在 snapshot 和 live block 中都可用。

这个方案覆盖所有已知问题，同时避免将浏览器端 transport 重写成一套新的流解析器。

---

## 5. Server Design

### 5.1 SSE Keepalive

HTTP 事件流接口新增周期性 `keepalive` 事件，用于让客户端检测连接仍然活着。

约束如下：

- 仅在连接空闲时发送，默认间隔 15 秒。
- `keepalive` 不是持久化业务事件，不写入 `events` 表，不占用业务 `seq`。
- 事件格式固定为：

```text
event: keepalive
data: {"session_id":"sess_123","latest_seq":42,"time":"2026-04-05T11:22:33Z"}

```

- `latest_seq` 仅用于诊断和日志，不参与客户端 seq 单调校验。
- 每次发送 keepalive 都必须 `Flush()`。

### 5.2 `after` Validation and Clamp

服务端继续接受 `after=<seq>`，但不再信任客户端传入值一定有效。

处理规则如下：

1. 读取当前 session 的 `latestSeq`。
2. 如果 `after < 0`，返回 `400 Bad Request`。
3. 如果 `after <= latestSeq`，按原行为回放 `seq > after` 的历史事件，然后切到 live 订阅。
4. 如果 `after > latestSeq`，将其钳制为 `latestSeq`，不返回错误，也不让连接停在未来游标上。

这样可消除“客户端持有超前 cursor 后，连接成功但永远收不到补齐事件”的静默挂起场景。

### 5.3 Subscriber Lifecycle and Backpressure

`stream.Bus` 需要从“广播 best effort”改成“可靠恢复友好的断开策略”。

设计要求：

- `Subscribe(sessionID)` 之外增加显式取消订阅能力，供 HTTP handler 在请求结束时清理。
- `Publish(event)` 不允许在满缓冲时静默丢事件。
- 当某个订阅者通道已满时：
  - 关闭该订阅者通道；
  - 从订阅者列表中移除；
  - 不影响其他订阅者继续接收事件。
- 关闭后的客户端会收到流结束或写失败，随后走前端重连和 replay。

这个策略的核心是“宁可断开并让客户端恢复，也不能默默漏一条事件然后继续渲染错误状态”。

### 5.4 HTTP Handler Lifecycle

`/v1/sessions/:id/events` handler 需要显式管理整个连接生命周期。

要求如下：

1. 订阅 live bus。
2. `defer unsubscribe()`，确保请求结束后及时移除订阅者。
3. 回放 `seq > after` 的历史事件，更新 `lastSeq`。
4. 进入 select loop：
   - live event：只转发 `event.Seq > lastSeq` 的事件；
   - keepalive ticker：发送 `keepalive`；
   - `r.Context().Done()`：退出。

### 5.5 Timeline Metadata Consistency

timeline snapshot 返回的 block 必须与 live reducer 使用同一套归并键。

要求如下：

- conversation item 归一化为 timeline block 时，保留其 `turn_id`。
- `user_message`、`assistant_output`、`tool_call`、`tool_result` 对应 block 都要带上所属 `turn_id`。
- 这样前端在 snapshot 载入后，后续 live `assistant.delta` 和 `assistant.completed` 才能继续命中同一 turn 的 block。

本次不需要数据库 schema 变更，因为 `conversation_items` 已经存了 `turn_id`；问题在于读取和归一化链路目前把它丢掉了。

---

## 6. Client Design

### 6.1 Event Parsing Model

`App.tsx` 的 SSE 处理必须改成“每条消息只解析一次”的模式。

要求如下：

1. 业务事件统一走一个 `handleStreamEvent(parsedEvent)`。
2. `JSON.parse(message.data)` 每条业务事件只执行一次。
3. `turn.completed` / `turn.failed` 的 query invalidation 复用同一个已解析对象，不再二次或三次解析。

### 6.2 Monotonic Sequence Guard

前端维护一个新的 `lastAppliedSeqRef`，作为唯一可信的业务事件游标。

处理规则如下：

1. 若 `event.seq <= lastAppliedSeqRef.current`：
   - 视为重复事件或过期事件；
   - 直接忽略，不进入 reducer。
2. 若 `event.seq === lastAppliedSeqRef.current + 1`：
   - 正常派发到 reducer；
   - 将 `lastAppliedSeqRef.current` 更新为 `event.seq`。
3. 若 `event.seq > lastAppliedSeqRef.current + 1`：
   - 视为 gap；
   - 立即关闭当前 SSE；
   - 触发 snapshot 恢复流程；
   - gap 事件本身不直接应用。

`lastAppliedSeqRef` 在以下场景重置：

- 初次加载 snapshot 后赋值为 `timeline.latest_seq`
- session 切换到 `null` 时置为 `0`
- snapshot 恢复成功后置为新 snapshot 的 `latest_seq`

### 6.3 Connection Liveness and Backoff

客户端连接状态机保留 `idle | connecting | open | reconnecting` 四态，但补齐 liveness 与退避策略。

要求如下：

- 每收到一条业务事件或 `keepalive` 事件，都重置 liveness timer。
- liveness timeout 固定为 45 秒。
- 超时后主动关闭当前 `EventSource`，进入 reconnect。
- reconnect delay 采用指数退避：
  - base = 1000ms
  - 每次失败乘 2
  - 上限 15000ms
  - 增加 `±25%` 抖动
- 一旦 `onopen` 成功，退避尝试次数清零。

### 6.4 Recovery Flow

恢复流程分为两层：

#### 正常续传

- 连接断开后，用 `after=<lastAppliedSeqRef.current>` 重新建立 SSE。
- 如果后续事件保持单调递增，则继续应用，无需额外 snapshot。

#### 强制 snapshot 恢复

在以下任一条件触发时执行 snapshot 恢复：

- 客户端检测到 seq gap
- 客户端内部状态被显式判定为不可信
- 重连期间收到无法解释的事件顺序异常

恢复步骤固定为：

1. 关闭当前 SSE。
2. 置连接状态为 `reconnecting`。
3. 调用 `/v1/sessions/:id/timeline` 获取新 snapshot。
4. 用 snapshot 整体替换 reducer 状态。
5. 将 `lastAppliedSeqRef` 设为 snapshot 的 `latest_seq`。
6. 重新创建 `EventSource(...?after=<latest_seq>)`。

### 6.5 Tool Call State Merge

`chatState.ts` 中的工具块归并改为字段级保留规则，而不是简单 spread。

归并规则：

- `tool.started`
  - 创建或更新 block
  - 写入 `id`、`turn_id`、`tool_name`、`args_preview`
  - `status = "running"`
- `tool.completed`
  - 命中已有 block 时，只更新存在值的字段
  - `status = "completed"`
  - `result_preview` 仅在 payload 提供时更新
  - `args_preview` 仅在 payload 提供时更新，否则保留已有值
- 任何一次归并都不允许用 `undefined` 覆盖已有字段。

这样无论服务端未来是否继续在 `tool.completed` 中回传参数，前端都能稳定显示。

### 6.6 Assistant Delta Isolation

assistant block 的归并键统一为：

- `kind === "assistant_output"`
- `turn_id === event.turn_id`
- `status !== "completed"`

额外约束：

- 如果 `assistant.delta` 找不到匹配 block，才新建 block。
- `assistant.completed` 只允许完成同一 `turn_id` 的 assistant block。
- 不再保留“没有 `turn_id` 时回退到最后一个 assistant block”的宽松逻辑。

这要求 snapshot block 已经带上 `turn_id`，见 5.5。

### 6.7 UUID-based Optimistic IDs

以下 ID 统一改用 `crypto.randomUUID()`：

- optimistic user block ID
- submit turn 的 `client_turn_id`

格式要求：

- optimistic block：`optimistic_<uuid>`
- client turn：`turn-<uuid>`

这样即使同一毫秒内连续发送两次，也不会冲突。

---

## 7. Reducer and State Invariants

修复后前端必须满足以下不变量：

1. `state.latestSeq` 仅反映“已经成功应用到 reducer 的最大业务 seq”。
2. 重复事件不会改变任何 reducer state。
3. 任何 seq gap 都不会产生部分渲染；客户端必须先恢复到可信 snapshot，再继续应用新事件。
4. 同一个 `tool_call_id` 始终只对应一个 tool block。
5. 同一个 `turn_id` 的 assistant 输出在 snapshot 和 live 更新之间可连续归并。

---

## 8. Protocol and Compatibility

### 8.1 向后兼容

- 现有持久化 `types.Event` 结构不变。
- 现有业务事件 `assistant.delta`、`tool.started`、`tool.completed` 等 payload 不变。
- 新增 `keepalive` 仅影响 SSE transport，不影响 timeline、metrics 和数据库内容。

### 8.2 兼容性要求

- 前端必须同时监听：
  - 默认 `message` 事件，用于持久化业务事件；
  - 命名 `keepalive` 事件，用于 liveness。
- 若旧服务端尚未发送 `keepalive`，新前端仍然可以工作，但 45 秒后会主动重连；本次上线以服务端和前端一起发布为前提。

---

## 9. File Changes

| File | Change |
|---|---|
| `web/console/src/App.tsx` | 重写 SSE 连接管理、单次解析、seq guard、liveness timeout、指数退避、snapshot 恢复 |
| `web/console/src/chatState.ts` | 工具块字段级归并、assistant turn 隔离、UUID 乐观 ID |
| `web/console/src/api.ts` | `client_turn_id` 改用 UUID |
| `web/console/src/App.test.tsx` | 补充 SSE 重连、gap 恢复、keepalive/liveness 测试 |
| `web/console/src/chatState.test.ts` | 补充工具块保留字段、assistant turn 隔离、重复 seq 忽略测试 |
| `internal/api/http/events.go` | keepalive、`after` 钳制、unsubscribe 生命周期 |
| `internal/api/http/deps.go` | 扩展 bus 接口，支持 unsubscribe |
| `internal/api/http/timeline.go` | snapshot block 保留 `turn_id` |
| `internal/stream/bus.go` | 取消静默丢事件，改为背压断开慢订阅者 |
| `internal/stream/sse.go` | 新增 keepalive 写出辅助函数 |
| `internal/api/http/e2e_test.go` | 覆盖 keepalive、慢订阅者断开、`after` 超前、gap 恢复前提 |
| `internal/api/http/http_test.go` | 覆盖 timeline `turn_id` 输出与事件流 handler 行为 |

---

## 10. Testing

### 10.1 Go Tests

新增或修改测试覆盖以下场景：

1. `/events?after=<latestSeq+100>` 不报错、不挂死，连接会从当前最新 seq 继续等待 live 事件。
2. SSE 空闲 15 秒时会输出 `keepalive` 事件。
3. 慢订阅者缓冲区打满后不会静默漏事件，而是被关闭并从 bus 移除。
4. HTTP handler 在请求结束时会 unsubscribe。
5. timeline snapshot 中的 `user_message`、`assistant_output`、`tool_call`、`tool_result` block 都带正确 `turn_id`。

### 10.2 Web Tests

新增或修改测试覆盖以下场景：

1. 相同 seq 的业务事件重复到达，只应用一次。
2. 收到 `seq = lastAppliedSeq + 2` 时，不直接渲染该事件，而是触发 snapshot 恢复。
3. 45 秒内没有业务事件也没有 keepalive 时，连接会被主动关闭并进入 reconnect。
4. 收到 keepalive 时会刷新 liveness timer，但不会改动 reducer blocks。
5. `tool.started` 后接 `tool.completed`，即使 completed payload 不带参数，也能保留 `args_preview`。
6. 不同 `turn_id` 的 `assistant.delta` 不会串写到旧 block。
7. optimistic block ID 与 `client_turn_id` 使用 UUID 生成。

### 10.3 Manual Verification

实现完成后需要手工验证以下流程：

1. 正常发送消息，观察工具调用、assistant delta、usage 和完成状态仍然按原样显示。
2. 在对话进行中停止 `agentd`，等待 liveness timeout，确认 UI 自动进入 reconnect。
3. 重新启动 `agentd`，确认 UI 能恢复并继续显示后续事件。
4. 构造高频工具/长回复场景，确认 console 不会出现缺块、重复块或错误拼接。

---

## 11. Risks and Mitigations

### 11.1 Risk: Gap Recovery Causing Extra Timeline Fetches

若网络环境差，gap 恢复会触发额外 `/timeline` 请求。

缓解：

- 只有在明确检测到 gap 或状态不可信时才执行 snapshot 恢复。
- 普通断线重连仍然优先走 `after=<lastAppliedSeq>`。

### 11.2 Risk: Keepalive Increasing SSE Noise

新增 keepalive 会增加少量 SSE 流量。

缓解：

- 15 秒一次的纯文本事件流量极小；
- 相比静默挂起，这是必要成本。

### 11.3 Risk: Disconnecting Slow Subscribers More Aggressively

背压即断开会使极慢客户端更频繁重连。

缓解：

- 断开后客户端会走持久化事件 replay，不会丢数据；
- 本次目标是可靠性优先于“尽量不断线”。

---

## 12. Acceptance Criteria

满足以下条件时，本次修复视为完成：

1. console 在半开连接和服务端短暂不可用场景下，最多在 45 秒内进入自动恢复流程，不会永久停在 open 状态。
2. 任意重复业务事件不会导致重复 block、错误状态覆盖或额外文本追加。
3. 任意业务事件序列缺口都会触发 snapshot 恢复，不会造成部分渲染。
4. 工具块在 started/completed 跨事件归并后，参数预览和结果预览都保持正确。
5. assistant live delta 与 snapshot block 能按 `turn_id` 正确衔接。
6. 服务端不存在“慢订阅者丢事件但连接继续存活”的路径。

