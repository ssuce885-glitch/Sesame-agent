# Workflow 模板样例

本目录提供可直接复用的 workflow JSON 模板。它们不会自动注入 runtime，也不会参与 seed/load；用途只是给 Web Console 和 API 录入提供官方样例。

`library.json` 是这批模板的分发索引，只提供 metadata 和文件映射，不会被 runtime 自动加载，也不会替代数据库里的 workflow 定义。

当前 parser 接受两种 `steps` 形状：

- `[{...}, {...}]`
- `{ "steps": [{...}, {...}] }`

如果使用顶层对象写法，当前导入/执行解析只要求对象里有 `steps` 字段即可；同一个 JSON 里的其他顶层 metadata 目前都会被忽略，不会覆盖表单或 API 里单独传入的 `name` / `trigger` / `owner_role`。

本目录里的模板文件使用第二种写法，所以有两种复制方式：

1. 在 Web Console 的 `Steps JSON` 中直接粘贴整个文件内容。
2. 把 `name` / `trigger` / `owner_role` 填到表单字段，再只粘贴文件里的 `steps` 数组。

`report_policy` / `approval_policy` 现在只是存档和预留字段。它们可以保留在样例里表达意图，但不会改变当前 executor 的执行行为。

通过 API 创建 workflow 时，推荐直接复用模板里的顶层字段；其中 `steps` 需要作为字符串字段传给当前 HTTP API：

```json
{
  "name": "Code Review",
  "trigger": "manual",
  "owner_role": "reviewer",
  "steps": "<把模板里的整个 JSON 或其中的 steps 数组序列化后放到这里>"
}
```

当前 runtime 可执行的入口只有两类：

- `manual`：通过 Web Console 或 `POST /v2/workflows/{id}/trigger` 手动触发。
- `automation`：把 workflow 绑定到 `Automation.workflow_id`，由 watcher/automation 在 `needs_agent` 路径间接触发。

`schedule`、`webhook`、`file_change` 仍在后续阶段，模板里不要依赖这些入口。

当前 step kind 只支持：

- `role_task`
- `approval`

其中 `approval` 会把 run 停在 `waiting_approval`，用于记录待审批动作；后续 resume 流程属于下一阶段能力。
