# V2 CLI / Config Manifest

更新时间：2026-05-03

## 结论

CLI 入口已切换到 V2 daemon + TUI。默认系统提示词已固定 Sesame 自我认知，避免模型沿用供应商身份。

验证结果：

```bash
go test ./cmd/sesame ./internal/config
```

通过。

## 建议提交

提交信息：

```text
wire sesame cli to v2 daemon and tui
```

建议包含：

- `cmd/sesame/main.go`
- `cmd/sesame/main_test.go`
- `internal/config/config.go`
- `internal/config/system_prompt_test.go`
- `README.md`

不要混入：

- V1 删除
- `internal/v2/**`
- Web Console 迁移
- role/skills 业务资产

## 行为变化

- `cmd/sesame` 不再调用旧 `internal/cli`。
- 默认地址从 `127.0.0.1:4317` 调整为 `127.0.0.1:8421`。
- 非 daemon 模式会先连接现有 daemon；连接失败时自动拉起当前二进制的 daemon 子进程。
- `0.0.0.0:port`、`:port`、`[::]:port` 会转换成可连接的本地地址。
- `ResolveSystemPrompt` 在未配置 prompt 或 prompt 文件为空时返回默认 Sesame 系统提示词。

## 自我认知约束

默认系统提示词要求：

- 自称 Sesame。
- 不自称 Claude、Anthropic、OpenAI、DeepSeek 或其他供应商助手。
- 被问到底层模型时，说明模型由本地 Sesame runtime 配置。

## 文档状态

- `README.md` 已更新到 V2 入口和包结构。
- `docs/frontend-tui.md`、`docs/frontend-web-console.md`、`docs/backend-api.md` 仍是旧架构文档，需要后续单独迁移或标记历史。
