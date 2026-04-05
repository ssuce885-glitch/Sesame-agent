import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import App from "./App";

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
