import { describe, expect, it } from "vitest";
import { parseSSEText } from "./client";

describe("parseSSEText", () => {
  it("handles CRLF frames and optional spaces after colons", () => {
    const events = parseSSEText('id: 12\r\nevent: assistant_delta\r\ndata: {"text":"hello"}\r\n\r\n');

    expect(events).toEqual([
      {
        id: "12",
        seq: 12,
        type: "assistant_delta",
        payload: { text: "hello" },
      },
    ]);
  });

  it("joins multi-line data records", () => {
    const events = parseSSEText('id: 13\nevent: turn_failed\ndata: {"message":\ndata: "bad"}\n\n');

    expect(events).toHaveLength(1);
    expect(events[0]).toMatchObject({
      id: "13",
      seq: 13,
      type: "turn_failed",
      payload: { message: "bad" },
    });
  });
});
