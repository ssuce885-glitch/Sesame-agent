import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MessageList } from "./MessageList";
import { I18nProvider } from "../i18n";

describe("MessageList", () => {
  it("localizes empty-state suggestion buttons for zh-CN", () => {
    window.localStorage.setItem("sesame-console.locale", "zh-CN");

    render(
      <I18nProvider>
        <MessageList messages={[]} connection="idle" onSuggestionClick={() => {}} />
      </I18nProvider>,
    );

    expect(screen.getByRole("button", { name: "解释这个代码库" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "运行测试" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "检查 git 状态" })).toBeInTheDocument();
  });

  it("keeps autoscrolling when a streaming message grows without adding a new row", async () => {
    const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    try {
      const { rerender } = render(
        <I18nProvider>
          <MessageList
            messages={[{ id: "a1", kind: "assistant_message", text: "hello", streaming: true }]}
            connection="open"
          />
        </I18nProvider>,
      );

      await waitFor(() => expect(scrollIntoView).toHaveBeenCalledTimes(1));

      rerender(
        <I18nProvider>
          <MessageList
            messages={[{ id: "a1", kind: "assistant_message", text: "hello world", streaming: true }]}
            connection="open"
          />
        </I18nProvider>,
      );

      await waitFor(() => expect(scrollIntoView).toHaveBeenCalledTimes(2));
    } finally {
      HTMLElement.prototype.scrollIntoView = originalScrollIntoView;
    }
  });
});
