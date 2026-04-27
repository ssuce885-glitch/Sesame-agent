# 自动化 Skill 评审清单

评审 automation 行为、prompt 或生成计划时，使用这份清单。

## 模式检查

- 当前 turn 属于哪种模式：create automation、owner task 还是 status/report？
- 实际行为有没有保持在该模式内？
- 有没有在请求范围外从 report 模式漂移到 repair 模式？

## 所有权检查

- 如果某个 role 本来应该拥有 automation，是否由该 role 自己创建？
- `main_agent` 是否越权直接创建了 automation？
- `owner` 是否和预期执行身份一致？

## Owner Task 检查

- owner task 是否执行了 `automation_goal` 定义的业务动作？
- owner task 是否避免修改 automation 配置？
- owner task 是否避免修改 watcher 脚本和 role 配置？

## Status/Report 检查

- status/report 响应是否保持只读？
- 是否清楚汇报了当前状态？
- 是否避免夹带修复或重配置动作？

## Watcher Contract 检查

- `watch_script` 当前是否可运行？
- watcher 输出是否符合 runtime 预期的 signal contract？
- 是否避免了伪 shell loop 或未托管的后台行为？

## Routing 与 Policy 检查

- 当 supervision 重要时，`report_target` 和 `escalation_target` 是否显式？
- policy 默认值是否归一化到 simple policy envelope？
- supervision 是否正确回到预期父级，而不是意外困在同一个 role 内部？
