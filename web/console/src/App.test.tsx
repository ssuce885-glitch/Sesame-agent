import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";

import App, {
  Composer,
  ToolCallCard,
  getContextUsageWarning,
  resetInputHistoryForTests,
} from "./App";
import type { Token用量, 时间线块 } from "./api";

function makeUsage(inputTokens: number, cachedTokens: number): Token用量 {
  return {
    provider: "openai_compatible",
    model: "gpt-4.1-mini",
    input_tokens: inputTokens,
    output_tokens: 120,
    cached_tokens: cachedTokens,
    cache_hit_rate: 0.2,
  };
}

function ComposerHarness() {
  const [value, setValue] = useState("");

  return (
    <Composer
      value={value}
      disabled={false}
      onChange={setValue}
      onSubmit={async () => {
        setValue("");
      }}
    />
  );
}

beforeEach(() => {
  resetInputHistoryForTests();
});

describe("应用", () => {
  it("默认展示对话页布局和中文导航", () => {
    render(
      <MemoryRouter initialEntries={["/chat"]}>
        <App />
      </MemoryRouter>,
    );

    expect(screen.getByText("Agent Console")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "对话" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "统计" })).toBeInTheDocument();
    expect(screen.getByText("会话")).toBeInTheDocument();
    expect(screen.getByText("输入指令")).toBeInTheDocument();
  });

  it("可以切换到统计页", () => {
    render(
      <MemoryRouter initialEntries={["/metrics"]}>
        <App />
      </MemoryRouter>,
    );

    expect(screen.getByText("Token 统计")).toBeInTheDocument();
    expect(screen.getByText("input tokens")).toBeInTheDocument();
    expect(screen.getByText("cached tokens")).toBeInTheDocument();
  });
});

describe("console ui helpers", () => {
  it("运行中的工具卡片默认展开并显示参数", () => {
    const shortRender = render(
      <ToolCallCard
        block={{
          type: "tool_call",
          tool_call_id: "tool_short",
          status: "completed",
          tool_name: "glob",
          args_preview: "{\"pattern\":\"*.go\"}",
        }}
      />,
    );

    expect(shortRender.container.querySelector("details")).not.toHaveAttribute("open");
    expect(screen.queryByText("{\"pattern\":\"*.go\"}")).not.toBeInTheDocument();

    shortRender.unmount();

    const longRender = render(
      <ToolCallCard
        block={{
          type: "tool_call",
          tool_call_id: "tool_long",
          status: "running",
          tool_name: "grep",
          args_preview: "{\"pattern\":\"TODO\"}",
        }}
      />,
    );

    expect(longRender.container.querySelector("details")).toHaveAttribute("open");
    expect(screen.getByText("{\"pattern\":\"TODO\"}")).toBeInTheDocument();

    longRender.unmount();

    const completedRender = render(
      <ToolCallCard
        block={{
          type: "tool_call",
          tool_call_id: "tool_completed",
          status: "completed",
          tool_name: "file_read",
          args_preview: "{\"path\":\"README.md\"}",
          result_preview: "README body",
        }}
      />,
    );

    const details = completedRender.container.querySelector("details");
    if (!(details instanceof HTMLDetailsElement)) {
      throw new Error("expected details element");
    }
    details.open = true;
    fireEvent(details, new Event("toggle"));

    expect(screen.getByText("README body")).toBeInTheDocument();
  });

  it("根据最新 assistant_message 的 usage 计算 context warning", () => {
    expect(getContextUsageWarning([])).toBeNull();

    expect(
      getContextUsageWarning([
        {
          id: "assistant_warn",
          kind: "assistant_message",
          content: [{ type: "text", text: "warn" }],
          usage: makeUsage(4800, 200),
        },
      ] as unknown as 时间线块[]),
    ).toEqual({
      level: "warn",
      text: "上下文已用 5000 tokens，接近压缩阈值",
    });

    expect(
      getContextUsageWarning([
        {
          id: "assistant_danger",
          kind: "assistant_message",
          content: [{ type: "text", text: "danger" }],
          usage: makeUsage(5600, 200),
        },
      ] as unknown as 时间线块[]),
    ).toEqual({
      level: "danger",
      text: "上下文即将触发压缩（5800 tokens）",
    });
  });
});

describe("输入历史", () => {
  it("支持方向键浏览历史、去重，并在回到底部时恢复草稿", async () => {
    const user = userEvent.setup();

    render(<ComposerHarness />);

    const textarea = screen.getByLabelText("输入指令");
    const sendButton = screen.getByRole("button", { name: "发送" });

    await user.type(textarea, "第一条");
    await user.click(sendButton);

    await user.type(textarea, "第二条");
    await user.click(sendButton);

    await user.type(textarea, "第二条");
    await user.click(sendButton);

    await user.type(textarea, "草稿");
    fireEvent.keyDown(textarea, { key: "ArrowUp" });
    expect(textarea).toHaveValue("第二条");

    fireEvent.keyDown(textarea, { key: "ArrowUp" });
    expect(textarea).toHaveValue("第一条");

    fireEvent.keyDown(textarea, { key: "ArrowDown" });
    expect(textarea).toHaveValue("第二条");

    fireEvent.keyDown(textarea, { key: "ArrowDown" });
    expect(textarea).toHaveValue("草稿");
  });
});
