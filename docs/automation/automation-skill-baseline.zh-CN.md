# 自动化 Skill 基线

这份文档定义项目内 automation system skills 的基线行为。

运行时应只保留两个 automation skill：

- `automation-standard-behavior`
- `automation-normalizer`

## 目标

目标是让 simple-chain automation 的行为稳定、可审计、模式边界清晰。

simple chain 为：

`watcher signal -> owner task -> report`

## 模式边界

### Create Automation

这个模式用于定义、更新或替换 automation。

- 只收集 automation 定义所需输入。
- 使用 automation 工具完成创建、更新和查询。
- 不要提前执行业务动作。
- 如果某个 role 本来就应该拥有这条 automation，那么默认应由该 role 自己创建，除非用户明确要求其他流程。

### Owner Task

这个模式用于 watcher 命中后的运行时执行。

- 执行 `automation_goal` 定义的业务动作。
- 按要求格式回报结果。
- 不要在这里创建或修改 automation。
- 不要在这里修 watcher 脚本或 role 配置。

### Status/Report

这个模式用于检查、解释和汇报当前状态。

- 读取当前状态。
- 汇报当前状态。
- 明确指出不匹配和阻塞点。
- 除非用户明确要求修复，否则不要修改 automation 定义或 watcher 脚本。

## 硬约束

- `main_agent` 不能悄悄替本应由 owning role 创建的 automation 越权创建。
- owner-task 执行不能漂移到配置层工作。
- status/report turn 不能漂移到修复工作。
- watcher 定义必须符合 runtime 实际要求的 signal contract。
- routing 和 policy 默认值必须显式、稳定。

## 两个 Skill 的职责

### `automation-standard-behavior`

负责：

- 流程框架
- 模式识别
- 跨模式禁止项
- 创建 / 执行 / 汇报 的边界

### `automation-normalizer`

负责：

- 必填字段
- watcher contract
- routing 默认值
- policy 默认值
- assumptions
- rejection rules

## 为什么要这样做

之前的四个 automation skill：

- `automation-standard-behavior`
- `automation-intake`
- `automation-normalizer`
- `automation-dispatch-planner`

职责重叠太多，模型更容易把不同模式拼在一起，出现：

- 模式混用
- supervision routing 偏移
- watcher payload 不符合 runtime 协议却仍被接受

新的两 skill 基线更清楚：

- 一个管行为和边界
- 一个管归一化和硬规则
