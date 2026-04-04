import { describe, expect, it } from "vitest";

import { 初始对话状态, 对话状态归并 } from "./chatState";

describe("对话状态归并", () => {
  it("可以把 tool call、assistant delta 和 usage 合并进同一条流", () => {
    let state = 对话状态归并(初始对话状态, {
      type: "event",
      event: {
        id: "evt_1",
        seq: 1,
        session_id: "sess_1",
        turn_id: "turn_1",
        type: "tool.started",
        time: "2026-04-04T00:00:00Z",
        payload: {
          tool_call_id: "call_1",
          tool_name: "file_read",
          arguments: "{\"path\":\"README.md\"}",
        },
      },
    });

    state = 对话状态归并(state, {
      type: "event",
      event: {
        id: "evt_2",
        seq: 2,
        session_id: "sess_1",
        turn_id: "turn_1",
        type: "tool.completed",
        time: "2026-04-04T00:00:01Z",
        payload: {
          tool_call_id: "call_1",
          tool_name: "file_read",
          result_preview: "README body",
        },
      },
    });

    state = 对话状态归并(state, {
      type: "event",
      event: {
        id: "evt_3",
        seq: 3,
        session_id: "sess_1",
        turn_id: "turn_1",
        type: "assistant.delta",
        time: "2026-04-04T00:00:02Z",
        payload: {
          text: "这里是总结。",
        },
      },
    });

    state = 对话状态归并(state, {
      type: "event",
      event: {
        id: "evt_4",
        seq: 4,
        session_id: "sess_1",
        turn_id: "turn_1",
        type: "turn.usage",
        time: "2026-04-04T00:00:03Z",
        payload: {
          provider: "openai_compatible",
          model: "glm-4-7-251222",
          input_tokens: 120,
          output_tokens: 36,
          cached_tokens: 24,
          cache_hit_rate: 0.2,
        },
      },
    });

    expect(state.latestSeq).toBe(4);
    expect(state.blocks).toHaveLength(2);
    expect(state.blocks[0]).toMatchObject({
      id: "call_1",
      kind: "tool_call",
      result_preview: "README body",
      status: "completed",
    });
    expect(state.blocks[1]).toMatchObject({
      kind: "assistant_output",
      text: "这里是总结。",
      usage: {
        input_tokens: 120,
        output_tokens: 36,
        cached_tokens: 24,
      },
    });
  });
});
