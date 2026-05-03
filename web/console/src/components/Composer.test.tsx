import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { Composer } from "./Composer";

describe("Composer", () => {
  it("keeps the draft when sending fails", async () => {
    const user = userEvent.setup();
    const onSend = vi.fn(async () => {
      throw new Error("send failed");
    });

    render(
      <I18nProvider>
        <Composer onSend={onSend} />
      </I18nProvider>,
    );

    const textbox = screen.getByRole("textbox");
    await user.type(textbox, "keep this draft");
    await user.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(onSend).toHaveBeenCalledWith("keep this draft"));
    expect(textbox).toHaveValue("keep this draft");
  });
});
