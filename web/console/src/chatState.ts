import type {
  AssistantContentBlock,
  服务端事件,
  时间线块,
  Token用量,
  回复增量负载,
  工具事件负载,
  失败负载,
} from "./api";

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
            id: `optimistic_${crypto.randomUUID()}`,
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
      return 追加工具调用(nextState, event, "running");
    case "tool.completed":
      return 完成工具调用(nextState, event);
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
    case "system.notice":
      return {
        ...nextState,
        blocks: [
          ...nextState.blocks,
          {
            id: `notice_${event.seq}`,
            turn_id: event.turn_id,
            kind: "notice",
            text: (event.payload as { text: string }).text,
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
  let index = 从后查找块(
    blocks,
    (block) => block.kind === "assistant_message" && block.turn_id === event.turn_id && block.status !== "completed",
  );

  if (index < 0) {
    blocks.push(创建助手消息块(event.turn_id, event.seq));
    index = blocks.length - 1;
  }

  const block = 复制助手消息块(blocks[index]);
  const lastContent = block.content[block.content.length - 1];
  if (lastContent?.type === "text") {
    block.content[block.content.length - 1] = {
      ...lastContent,
      text: `${lastContent.text}${payload.text}`,
    };
  } else {
    block.content.push({
      type: "text",
      text: payload.text,
    });
  }
  block.status = "streaming";
  blocks[index] = block;
  return { ...state, blocks };
}

function 标记最后回复完成(state: 对话状态, turnId?: string): 对话状态 {
  const blocks = [...state.blocks];
  const index = 从后查找块(
    blocks,
    (block) => block.kind === "assistant_message" && (!turnId || block.turn_id === turnId) && block.status !== "completed",
  );
  if (index < 0) {
    return state;
  }
  blocks[index] = {
    ...blocks[index],
    status: "completed",
  };
  return { ...state, blocks };
}

function 追加工具调用(state: 对话状态, event: 服务端事件, status: string): 对话状态 {
  const payload = event.payload as 工具事件负载;
  const blocks = [...state.blocks];
  let index = 从后查找块(
    blocks,
    (block) => block.kind === "assistant_message" && block.turn_id === event.turn_id && block.status !== "completed",
  );

  if (index < 0) {
    blocks.push(创建助手消息块(event.turn_id, event.seq));
    index = blocks.length - 1;
  }

  const block = 复制助手消息块(blocks[index]);
  block.content.push({
    type: "tool_call",
    tool_call_id: payload.tool_call_id,
    tool_name: payload.tool_name,
    args_preview: payload.arguments,
    status,
  });
  // tool.started marks the end of the current assistant message boundary.
  block.status = "completed";
  blocks[index] = block;
  return { ...state, blocks };
}

function 完成工具调用(state: 对话状态, event: 服务端事件): 对话状态 {
  const payload = event.payload as 工具事件负载;
  const blocks = [...state.blocks];
  const toolLocation = 查找工具调用块(blocks, payload.tool_call_id);
  if (toolLocation) {
    const block = 复制助手消息块(blocks[toolLocation.blockIndex]);
    const content = block.content[toolLocation.contentIndex];
    block.content[toolLocation.contentIndex] = {
      ...content,
      status: "completed",
      result_preview: payload.result_preview ?? content.result_preview,
    } as AssistantContentBlock;
    blocks[toolLocation.blockIndex] = block;
  }
  return { ...state, blocks };
}

function 追加用量(state: 对话状态, turnId: string | undefined, usage: Token用量): 对话状态 {
  const blocks = [...state.blocks];
  const index = 从后查找块(
    blocks,
    (block) => block.kind === "assistant_message" && (!turnId || block.turn_id === turnId),
  );
  if (index < 0) {
    return state;
  }
  blocks[index] = {
    ...blocks[index],
    usage,
  };
  return { ...state, blocks };
}

function 创建助手消息块(turnId: string | undefined, seq: number): 时间线块 {
  return {
    id: `assistant_${turnId ?? seq}_${crypto.randomUUID()}`,
    turn_id: turnId,
    kind: "assistant_message",
    status: "streaming",
    content: [],
  };
}

function 复制助手消息块(block: 时间线块): 时间线块 {
  return {
    ...block,
    content: [...(block.content ?? [])],
  };
}

function 查找工具调用块(blocks: 时间线块[], toolCallId: string) {
  for (let blockIndex = blocks.length - 1; blockIndex >= 0; blockIndex -= 1) {
    const block = blocks[blockIndex];
    if (block.kind !== "assistant_message" || !block.content) {
      continue;
    }
    for (let contentIndex = block.content.length - 1; contentIndex >= 0; contentIndex -= 1) {
      const content = block.content[contentIndex];
      if (content.type === "tool_call" && content.tool_call_id === toolCallId) {
        return { blockIndex, contentIndex };
      }
    }
  }
  return null;
}

function 从后查找块(blocks: 时间线块[], matcher: (block: 时间线块) => boolean) {
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    if (matcher(blocks[index])) {
      return index;
    }
  }
  return -1;
}
