import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";

import App, {
  Composer,
  ToolCallCard,
  getContextUsageWarning,
  mergeAdjacentAssistantBlocks,
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
  it("合并相邻且同 turn 的 assistant_output，并保留最后一个 usage", () => {
    const blocks: 时间线块[] = [
      {
        id: "assistant_1",
        turn_id: "turn_1",
        kind: "assistant_output",
        text: "第一段",
        status: "streaming",
      },
      {
        id: "assistant_2",
        turn_id: "turn_1",
        kind: "assistant_output",
        text: "\n第二段",
        status: "completed",
        usage: makeUsage(3200, 180),
      },
      {
        id: "tool_1",
        turn_id: "turn_1",
        kind: "tool_call",
        status: "completed",
        tool_name: "file_read",
      },
      {
        id: "assistant_3",
        turn_id: "turn_2",
        kind: "assistant_output",
        text: "第三段",
      },
    ];

    expect(mergeAdjacentAssistantBlocks(blocks)).toEqual([
      {
        id: "assistant_1",
        turn_id: "turn_1",
        kind: "assistant_output",
        text: "第一段\n第二段",
        status: "completed",
        usage: makeUsage(3200, 180),
      },
      {
        id: "tool_1",
        turn_id: "turn_1",
        kind: "tool_call",
        status: "completed",
        tool_name: "file_read",
      },
      {
        id: "assistant_3",
        turn_id: "turn_2",
        kind: "assistant_output",
        text: "第三段",
      },
    ]);
  });

  it("根据结果长度决定工具卡片默认展开还是折叠", () => {
    const shortResult = "短结果";
    const longResult = "很长的工具结果".repeat(30);

    const shortRender = render(
      <ToolCallCard
        block={{
          id: "tool_short",
          kind: "tool_call",
          status: "completed",
          tool_name: "glob",
          result_preview: shortResult,
        }}
      />,
    );

    expect(shortRender.container.querySelector("details")).toHaveAttribute("open");
    expect(screen.getByText(shortResult)).toBeInTheDocument();

    shortRender.unmount();

    const longRender = render(
      <ToolCallCard
        block={{
          id: "tool_long",
          kind: "tool_call",
          status: "completed",
          tool_name: "grep",
          result_preview: longResult,
        }}
      />,
    );

    expect(longRender.container.querySelector("details")).not.toHaveAttribute("open");
    expect(screen.getByText(`${longResult.slice(0, 80)}...`)).toBeInTheDocument();
    expect(screen.queryByText(longResult)).not.toBeInTheDocument();
  });

  it("根据最新 assistant_output 的 usage 计算 context warning", () => {
    expect(getContextUsageWarning([])).toBeNull();

    expect(
      getContextUsageWarning([
        {
          id: "assistant_warn",
          kind: "assistant_output",
          usage: makeUsage(4800, 200),
        },
      ]),
    ).toEqual({
      level: "warn",
      text: "上下文已用 5000 tokens，接近压缩阈值",
    });

    expect(
      getContextUsageWarning([
        {
          id: "assistant_danger",
          kind: "assistant_output",
          usage: makeUsage(5600, 200),
        },
      ]),
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
