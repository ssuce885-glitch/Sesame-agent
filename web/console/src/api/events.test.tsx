import { act, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { openEventStream } from "./client";
import {
  initialState,
  reduceChat,
  timelineToMessages,
  useSessionEvents,
  type ChatState,
} from "./events";
import type { SSEEvent, TimelineBlock } from "./types";

vi.mock("./client", () => ({
  openEventStream: vi.fn(),
}));

function event(type: string, seq: number, turnId: string, payload: Record<string, unknown>): SSEEvent {
  return {
    id: `${seq}`,
    seq,
    type,
    payload: { ...payload, turn_id: turnId },
  };
}

describe("reduceChat", () => {
  it("replaces an optimistic user message when timeline catches up", () => {
    const optimistic = reduceChat(initialState, {
      type: "appendUserMessage",
      id: "opt_9999999999999",
      seq: 6,
      text: "hello",
    });

    const timeline: TimelineBlock[] = [
      { kind: "user_message", text: "hello" },
    ];
    const state = reduceChat(optimistic, {
      type: "init",
      messages: timelineToMessages(timeline),
      latestSeq: 6,
    });

    expect(state.messages).toHaveLength(1);
    expect(state.messages[0]).toMatchObject({ id: "tl_0_user_message", text: "hello" });
  });

  it("keeps assistant deltas scoped to their turn", () => {
    const state: ChatState = {
      ...initialState,
      messages: [
        {
          id: "a_1_turn-1",
          turnId: "turn-1",
          kind: "assistant_message",
          text: "first",
          streaming: true,
        },
      ],
      latestSeq: 1,
    };

    const next = reduceChat(state, {
      type: "event",
      event: event("assistant.delta", 2, "turn-2", { text: "second" }),
    });

    expect(next.messages).toHaveLength(2);
    expect(next.messages[0]).toMatchObject({ turnId: "turn-1", text: "first" });
    expect(next.messages[1]).toMatchObject({ turnId: "turn-2", text: "second", streaming: true });
  });

  it("completes the assistant message for the matching turn only", () => {
    const state: ChatState = {
      ...initialState,
      messages: [
        {
          id: "a_1_turn-1",
          turnId: "turn-1",
          kind: "assistant_message",
          text: "first",
          streaming: true,
        },
        {
          id: "a_2_turn-2",
          turnId: "turn-2",
          kind: "assistant_message",
          text: "second",
          streaming: true,
        },
      ],
      latestSeq: 2,
    };

    const next = reduceChat(state, {
      type: "event",
      event: event("assistant.completed", 3, "turn-1", {}),
    });

    expect(next.messages[0]).toMatchObject({ turnId: "turn-1", streaming: false });
    expect(next.messages[1]).toMatchObject({ turnId: "turn-2", streaming: true });
  });

  it("upserts repeated tool.started events by tool call id", () => {
    const first = reduceChat(initialState, {
      type: "event",
      event: event("tool.started", 1, "turn-1", {
        tool_call_id: "call-1",
        tool_name: "shell_command",
        arguments: "echo first",
      }),
    });

    const second = reduceChat(first, {
      type: "event",
      event: event("tool.started", 2, "turn-1", {
        tool_call_id: "call-1",
        tool_name: "shell_command",
        arguments: "echo second",
      }),
    });

    expect(second.messages).toHaveLength(1);
    expect(second.messages[0]).toMatchObject({ toolCallId: "call-1", argsPreview: "echo second" });
  });

  it("handles v2 underscore event names and payload aliases", () => {
    const withAssistant = reduceChat(initialState, {
      type: "event",
      event: event("assistant_delta", 1, "turn-1", { text: "hello" }),
    });

    const withTool = reduceChat(withAssistant, {
      type: "event",
      event: event("tool_call", 2, "turn-1", {
        id: "call-1",
        name: "shell_command",
        args: { cmd: "pwd" },
      }),
    });

    const withResult = reduceChat(withTool, {
      type: "event",
      event: event("tool_result", 3, "turn-1", {
        id: "call-1",
        name: "shell_command",
        output: "/tmp/project",
        is_error: false,
      }),
    });

    const completed = reduceChat(withResult, {
      type: "event",
      event: event("turn_completed", 4, "turn-1", {}),
    });

    expect(completed.messages[0]).toMatchObject({
      kind: "assistant_message",
      text: "hello",
      streaming: false,
    });
    expect(completed.messages[1]).toMatchObject({
      kind: "tool_call",
      toolCallId: "call-1",
      toolName: "shell_command",
      argsPreview: JSON.stringify({ cmd: "pwd" }),
      resultPreview: "/tmp/project",
      status: "completed",
    });
  });
});

describe("useSessionEvents", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("opens a fresh stream after an error closes the current stream", async () => {
    vi.useFakeTimers();
    const close = vi.fn();
    const callbacks: Array<{ onError: (err: unknown) => void }> = [];

    vi.mocked(openEventStream).mockImplementation((_sessionId, _afterSeq, _onEvent, _onOpen, onError) => {
      callbacks.push({ onError });
      return { close };
    });

    function Harness() {
      useSessionEvents("session-1", 0, () => {}, () => {});
      return null;
    }

    render(<Harness />);
    expect(openEventStream).toHaveBeenCalledTimes(1);

    act(() => {
      callbacks[0].onError(new Error("stream failed"));
    });
    expect(close).toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(openEventStream).toHaveBeenCalledTimes(2);
  });

  it("does not open the event stream until enabled", () => {
    const close = vi.fn();
    vi.mocked(openEventStream).mockReturnValue({ close });

    function Harness({ enabled }: { enabled: boolean }) {
      useSessionEvents("session-1", 42, () => {}, () => {}, enabled);
      return null;
    }

    const { rerender } = render(<Harness enabled={false} />);
    expect(openEventStream).not.toHaveBeenCalled();

    rerender(<Harness enabled={true} />);
    expect(openEventStream).toHaveBeenCalledTimes(1);
    expect(openEventStream).toHaveBeenLastCalledWith(
      "session-1",
      42,
      expect.any(Function),
      expect.any(Function),
      expect.any(Function),
    );
  });
});
