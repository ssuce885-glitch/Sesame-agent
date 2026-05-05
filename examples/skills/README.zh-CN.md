# Skill 模板样例

本目录提供可分发的 skill 模板目录。它们只是样例资产，不会自动写入 workspace，也不会被 runtime 自动加载。

使用方式：

```bash
sesame skill lint examples/skills/automation-normalizer/SKILL.md
sesame skill test examples/skills/automation-normalizer/SKILL.md
sesame skill install automation-normalizer --workspace /path/to/workspace
sesame skill pack automation-normalizer --out /tmp/automation-normalizer.zip
```

说明：

- `sesame skill install <name>` 会从 `examples/skills/<name>` 查找模板。
- 也可以直接传本地目录或 markdown 文件路径。
- `sesame skill test <path...> [--workspace <root>]` 只做离线校验：不启动 daemon、不调用模型、不执行外部工具。
- `sesame skill pack <template-path-or-name> --out <zip-path> [--workspace <root>]` 会先跑 lint/test 等价校验，失败时不会生成 zip。
- `pack` 输出包的根目录固定为 skill 名称；目录模板会保留全部普通文件，markdown 单文件模板只包含 `SKILL.md`。
- manifest 推荐写 `requires_tools`。`allowed_tools` / `allowed-tools` 只作为旧别名兼容，不会自动解锁 runtime 里的 gated tools。
- 模板内容只包含占位符，不包含真实邮箱、token、代理、API key 或外部 connector 凭据。
- `examples/` 和 `tests/` 下的 markdown 也是占位符资产，用于离线分发校验，不代表真实业务数据。
- 涉及通知/外发语义的模板只允许生成 draft，不允许直接 send。
