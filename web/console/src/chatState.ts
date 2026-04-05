import type { 服务端事件, 时间线块, Token用量, 回复增量负载, 工具事件负载, 失败负载 } from "./api";

export interface 对话状态 {
  blocks: 时间线块[];
  latestSeq: number;
  connection: "idle" | "connecting" | "open" | "reconnecting";
}

export type 对话动作 =
  | { type: "snapshot"; blocks: 时间线块[]; latestSeq: number }
  | { type: "connection"; value: 对话状态["connection"] }
  | { type: "optimistic-user"; text: string }
  | { type: "event"; event: 服务端事件 };

export const 初始对话状态: 对话状态 = {
  blocks: [],
  latestSeq: 0,
  connection: "idle",
};

export function 对话状态归并(state: 对话状态, action: 对话动作): 对话状态 {
  switch (action.type) {
    case "snapshot":
      return {
        blocks: action.blocks,
        latestSeq: action.latestSeq,
        connection: state.connection,
      };
    case "connection":
      return {
        ...state,
        connection: action.value,
      };
    case "optimistic-user":
      return {
        ...state,
        blocks: [
          ...state.blocks,
          {
            id: `optimistic_${Date.now()}`,
            kind: "user_message",
            text: action.text,
            status: "pending",
          },
        ],
      };
    case "event":
      return 应用事件(state, action.event);
    default:
      return state;
  }
}

function 应用事件(state: 对话状态, event: 服务端事件): 对话状态 {
  const nextState: 对话状态 = {
    ...state,
    latestSeq: Math.max(state.latestSeq, event.seq),
  };

  switch (event.type) {
    case "assistant.delta":
      return 追加回复增量(nextState, event);
    case "assistant.completed":
      return 标记最后回复完成(nextState, event.turn_id);
    case "tool.started":
      return 合并工具块(nextState, event, "running");
    case "tool.completed":
      return 合并工具块(nextState, event, "completed");
    case "turn.usage":
      return 追加用量(nextState, event.turn_id, event.payload as Token用量);
    case "turn.failed":
      return {
        ...nextState,
        blocks: [
          ...nextState.blocks,
          {
            id: `error_${event.seq}`,
            turn_id: event.turn_id,
            kind: "error",
            text: (event.payload as 失败负载).message,
          },
        ],
      };
    case "context.compacted":
      return {
        ...nextState,
        blocks: [
          ...nextState.blocks,
          {
            id: `notice_${event.seq}`,
            turn_id: event.turn_id,
            kind: "notice",
            text: "系统已完成一次上下文压缩。",
          },
        ],
      };
    default:
      return nextState;
  }
}

function 追加回复增量(state: 对话状态, event: 服务端事件): 对话状态 {
  const payload = event.payload as 回复增量负载;
  const blocks = [...state.blocks];
  const index = 从后查找块(blocks, (block) => block.kind === "assistant_output" && block.turn_id === event.turn_id && block.status !== "completed");
  if (index >= 0) {
    blocks[index] = {
      ...blocks[index],
      text: `${blocks[index].text ?? ""}${payload.text}`,
      status: "streaming",
    };
    return { ...state, blocks };
  }

  blocks.push({
    id: `assistant_${event.turn_id ?? event.seq}`,
    turn_id: event.turn_id,
    kind: "assistant_output",
    text: payload.text,
    status: "streaming",
  });
  return { ...state, blocks };
}

function 标记最后回复完成(state: 对话状态, turnId?: string): 对话状态 {
  const blocks = [...state.blocks];
  const index = 从后查找块(blocks, (block) => block.kind === "assistant_output" && (!turnId || block.turn_id === turnId));
  if (index < 0) {
    return state;
  }
  blocks[index] = {
    ...blocks[index],
    status: "completed",
  };
  return { ...state, blocks };
}

function 合并工具块(state: 对话状态, event: 服务端事件, status: string): 对话状态 {
  const payload = event.payload as 工具事件负载;
  const blocks = [...state.blocks];
  const index = blocks.findIndex((block) => block.id === payload.tool_call_id);
  const nextBlock: 时间线块 = {
    id: payload.tool_call_id,
    turn_id: event.turn_id,
    kind: "tool_call",
    status,
    tool_name: payload.tool_name,
    args_preview: payload.arguments,
    result_preview: payload.result_preview,
  };

  if (index >= 0) {
    blocks[index] = {
      ...blocks[index],
      ...nextBlock,
      result_preview: payload.result_preview ?? blocks[index].result_preview,
    };
    return { ...state, blocks };
  }

  blocks.push(nextBlock);
  return { ...state, blocks };
}

function 追加用量(state: 对话状态, turnId: string | undefined, usage: Token用量): 对话状态 {
  const blocks = [...state.blocks];
  const index = 从后查找块(blocks, (block) => block.kind === "assistant_output" && (!turnId || block.turn_id === turnId));
  if (index < 0) {
    return state;
  }
  blocks[index] = {
    ...blocks[index],
    usage,
  };
  return { ...state, blocks };
}

function 从后查找块(blocks: 时间线块[], matcher: (block: 时间线块) => boolean) {
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    if (matcher(blocks[index])) {
      return index;
    }
  }
  return -1;
}
