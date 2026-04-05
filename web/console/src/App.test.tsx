import { useState } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import App, {
  Composer,
  SessionListRail,
  ToolCallCard,
  getContextUsageWarning,
  resetInputHistoryForTests,
} from "./App";

function makeUsage(inputTokens: number, cachedTokens: number) {
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

describe("app", () => {
  it("renders the chat shell and navigation", () => {
    render(
      <MemoryRouter initialEntries={["/chat"]}>
        <App />
      </MemoryRouter>,
    );

    expect(screen.getByText("Agent Console")).toBeInTheDocument();
    expect(screen.getAllByRole("link")).toHaveLength(2);
  });

  it("renders the metrics page", () => {
    render(
      <MemoryRouter initialEntries={["/metrics"]}>
        <App />
      </MemoryRouter>,
    );

    expect(screen.getByText("input tokens")).toBeInTheDocument();
    expect(screen.getByText("cached tokens")).toBeInTheDocument();
  });
});

describe("console ui helpers", () => {
  it("keeps completed tool calls collapsed by default and shows previews when opened", () => {
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

  it("computes the context warning from the latest assistant_message usage", () => {
    expect(getContextUsageWarning([])).toBeNull();

    const warn = getContextUsageWarning(
      [
        {
          id: "assistant_warn",
          kind: "assistant_message",
          content: [{ type: "text", text: "warn" }],
          usage: makeUsage(4800, 200),
        },
      ] as Parameters<typeof getContextUsageWarning>[0],
    );
    expect(warn).toMatchObject({
      level: "warn",
    });
    expect(warn?.text).toContain("5000 tokens");

    const danger = getContextUsageWarning(
      [
        {
          id: "assistant_danger",
          kind: "assistant_message",
          content: [{ type: "text", text: "danger" }],
          usage: makeUsage(5600, 200),
        },
      ] as Parameters<typeof getContextUsageWarning>[0],
    );
    expect(danger).toMatchObject({
      level: "danger",
    });
    expect(danger?.text).toContain("5800 tokens");
  });
});

describe("session list rail", () => {
  it("renders a persistent delete button for idle sessions without triggering selection", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const onDelete = vi.fn();

    render(
      <SessionListRail
        sessions={[
          {
            id: "sess_idle",
            title: "删除我",
            last_preview: "preview",
            workspace_root: "E:/project/go-agent",
            state: "idle",
            updated_at: "2026-04-05T14:54:00Z",
            is_selected: false,
          },
        ]}
        selectedSessionId=""
        workspaceRoot="E:/project/go-agent"
        loading={false}
        onWorkspaceRootChange={() => {}}
        onCreate={() => {}}
        onSelect={onSelect}
        onDelete={onDelete}
      />,
    );

    await user.click(screen.getByRole("button", { name: "删除会话 删除我" }));

    expect(onSelect).not.toHaveBeenCalled();
    expect(onDelete).toHaveBeenCalledWith(expect.objectContaining({ id: "sess_idle" }));
  });

  it("disables the delete button for non-idle sessions", () => {
    render(
      <SessionListRail
        sessions={[
          {
            id: "sess_busy",
            title: "忙碌会话",
            last_preview: "preview",
            workspace_root: "E:/project/go-agent",
            state: "running",
            updated_at: "2026-04-05T14:54:00Z",
            is_selected: false,
          },
        ]}
        selectedSessionId=""
        workspaceRoot="E:/project/go-agent"
        loading={false}
        onWorkspaceRootChange={() => {}}
        onCreate={() => {}}
        onSelect={() => {}}
        onDelete={() => {}}
      />,
    );

    const deleteButton = screen.getByRole("button", { name: "删除会话 忙碌会话" });
    expect(deleteButton).toBeDisabled();
    expect(deleteButton).toHaveAttribute("title", "运行中的会话暂不支持删除");
  });
});

describe("input history", () => {
  it("supports arrow-key history navigation, dedupes adjacent entries, and restores the draft", async () => {
    const user = userEvent.setup();

    const view = render(<ComposerHarness />);

    const scoped = within(view.container);
    const textarea = scoped.getByRole("textbox");
    const sendButton = scoped.getByRole("button");

    await user.type(textarea, "first");
    await user.click(sendButton);

    await user.type(textarea, "second");
    await user.click(sendButton);

    await user.type(textarea, "second");
    await user.click(sendButton);

    await user.type(textarea, "draft");
    fireEvent.keyDown(textarea, { key: "ArrowUp" });
    expect(textarea).toHaveValue("second");

    fireEvent.keyDown(textarea, { key: "ArrowUp" });
    expect(textarea).toHaveValue("first");

    fireEvent.keyDown(textarea, { key: "ArrowDown" });
    expect(textarea).toHaveValue("second");

    fireEvent.keyDown(textarea, { key: "ArrowDown" });
    expect(textarea).toHaveValue("draft");
  });
});
