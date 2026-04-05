import { describe, expect, it } from "vitest";

import { 初始对话状态, 对话状态归并 } from "./chatState";

describe("对话状态归并", () => {
  it("在 tool 边界后开启新的 assistant_message，并把结果预览回填到 tool_call", () => {
    let state = 对话状态归并(初始对话状态, {
      type: "event",
      event: {
        id: "evt_1",
        seq: 1,
        session_id: "sess_1",
        turn_id: "turn_1",
        type: "assistant.delta",
        time: "2026-04-04T00:00:00Z",
        payload: {
          text: "工具前说明。",
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
        type: "tool.started",
        time: "2026-04-04T00:00:01Z",
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
        id: "evt_3",
        seq: 3,
        session_id: "sess_1",
        turn_id: "turn_1",
        type: "tool.completed",
        time: "2026-04-04T00:00:02Z",
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
        id: "evt_4",
        seq: 4,
        session_id: "sess_1",
        turn_id: "turn_1",
        type: "assistant.delta",
        time: "2026-04-04T00:00:03Z",
        payload: {
          text: "这里是总结。",
        },
      },
    });

    state = 对话状态归并(state, {
      type: "event",
      event: {
        id: "evt_5",
        seq: 5,
        session_id: "sess_1",
        turn_id: "turn_1",
        type: "turn.usage",
        time: "2026-04-04T00:00:04Z",
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

    expect(state.latestSeq).toBe(5);
    expect(state.blocks).toHaveLength(2);
    expect(state.blocks[0]).toMatchObject({
      kind: "assistant_message",
      status: "completed",
      content: [
        {
          type: "text",
          text: "工具前说明。",
        },
        {
          type: "tool_call",
          tool_call_id: "call_1",
          tool_name: "file_read",
          args_preview: "{\"path\":\"README.md\"}",
          result_preview: "README body",
          status: "completed",
        },
      ],
    });
    expect(state.blocks.map((block) => block.id)).not.toContain("tool_result_call_1");
    expect(state.blocks[1]).toMatchObject({
      kind: "assistant_message",
      content: [
        {
          type: "text",
          text: "这里是总结。",
        },
      ],
      usage: {
        input_tokens: 120,
        output_tokens: 36,
        cached_tokens: 24,
      },
    });
  });
});
