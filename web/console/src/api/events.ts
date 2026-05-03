import { useCallback, useEffect, useRef } from "react";
import { openEventStream } from "./client";
import type { SSEEvent, TimelineBlock, TimelineContent } from "./types";

// ─── Types ─────────────────────────────────────────────────────────────────────

export interface ChatMessage {
  id: string;
  seq?: number;
  turnId?: string;
  kind:
    | "user_message"
    | "assistant_message"
    | "tool_call"
    | "notice"
    | "error";
  text?: string;
  streaming?: boolean;
  status?: string;
  toolCallId?: string;
  toolName?: string;
  argsPreview?: string;
  resultPreview?: string;
  isError?: boolean;
}

export interface ChatState {
  messages: ChatMessage[];
  latestSeq: number;
  connection: "idle" | "connecting" | "open" | "reconnecting" | "error";
}

export type ChatAction =
  | { type: "init"; messages: ChatMessage[]; latestSeq: number }
  | { type: "connection"; value: ChatState["connection"] }
  | { type: "event"; event: SSEEvent }
  | { type: "appendUserMessage"; id: string; seq: number; text: string };

export const initialState: ChatState = {
  messages: [],
  latestSeq: 0,
  connection: "idle",
};

// ─── Reducer ──────────────────────────────────────────────────────────────────

export function reduceChat(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case "init": {
      const tlLatestSeq = action.latestSeq;
      const liveAfterInit = state.messages.filter((m) =>
        shouldKeepLiveMessage(m, action.messages, tlLatestSeq),
      );
      const merged = liveAfterInit.length > 0
        ? [...action.messages, ...liveAfterInit]
        : action.messages;
      return { ...state, messages: merged, latestSeq: Math.max(tlLatestSeq, state.latestSeq) };
    }
    case "connection":
      return { ...state, connection: action.value };
    case "event":
      return applyEvent(state, action.event);
    case "appendUserMessage":
      return {
        ...state,
        latestSeq: Math.max(state.latestSeq, action.seq),
        messages: [
          ...state.messages,
          {
            id: action.id,
            seq: action.seq,
            kind: "user_message",
            text: action.text,
          },
        ],
      };
    default:
      return state;
  }
}

function shouldKeepLiveMessage(
  msg: ChatMessage,
  timelineMessages: ChatMessage[],
  timelineLatestSeq: number,
): boolean {
  if ((msg.seq ?? 0) <= timelineLatestSeq) {
    return false;
  }
  return !timelineMessages.some((timelineMsg) => sameLogicalMessage(msg, timelineMsg));
}

function sameLogicalMessage(a: ChatMessage, b: ChatMessage): boolean {
  if (a.kind !== b.kind) {
    return false;
  }
  if (a.turnId && b.turnId && a.turnId === b.turnId) {
    return true;
  }
  if (a.kind === "tool_call" && a.toolCallId && a.toolCallId === b.toolCallId) {
    return true;
  }
  return a.kind === "user_message" && a.text === b.text;
}

function findLastIndex<T>(items: T[], predicate: (item: T) => boolean): number {
  for (let i = items.length - 1; i >= 0; i--) {
    if (predicate(items[i])) return i;
  }
  return -1;
}

function eventTurnId(event: SSEEvent): string | undefined {
  const value = event.payload.turn_id;
  return typeof value === "string" ? value : undefined;
}

function turnMatches(msg: ChatMessage, turnId: string | undefined): boolean {
  return !turnId || !msg.turnId || msg.turnId === turnId;
}

function payloadString(
  payload: Record<string, unknown>,
  ...keys: string[]
): string | undefined {
  for (const key of keys) {
    const value = payload[key];
    if (typeof value === "string") {
      return value;
    }
  }
  return undefined;
}

function payloadPreview(
  payload: Record<string, unknown>,
  ...keys: string[]
): string | undefined {
  for (const key of keys) {
    const value = payload[key];
    if (typeof value === "string") {
      return value;
    }
    if (value != null) {
      try {
        return JSON.stringify(value);
      } catch {
        return String(value);
      }
    }
  }
  return undefined;
}

function eventType(type: string): string {
  switch (type) {
    case "turn_started":
      return "turn.started";
    case "turn_completed":
      return "assistant.completed";
    case "turn_failed":
      return "turn.failed";
    case "turn_interrupted":
      return "turn.interrupted";
    case "assistant_delta":
      return "assistant.delta";
    case "tool_call":
      return "tool.started";
    case "tool_result":
      return "tool.completed";
    default:
      return type;
  }
}

function applyEvent(state: ChatState, event: SSEEvent): ChatState {
  const nextSeq = Math.max(state.latestSeq, event.seq);
  const turnId = eventTurnId(event);

  switch (eventType(event.type)) {
    case "turn.started":
      return { ...state, latestSeq: nextSeq };

    case "assistant.delta": {
      const text = payloadString(event.payload, "text") ?? "";
      const msgs = [...state.messages];
      const idx = findLastIndex(
        msgs,
        (msg) =>
          msg.kind === "assistant_message" &&
          msg.streaming === true &&
          turnMatches(msg, turnId),
      );
      if (idx < 0) {
        msgs.push({
          id: `a_${event.seq}_${turnId ?? crypto.randomUUID()}`,
          seq: event.seq,
          turnId,
          kind: "assistant_message",
          text,
          streaming: true,
        });
      } else {
        msgs[idx] = {
          ...msgs[idx],
          seq: event.seq,
          text: (msgs[idx].text ?? "") + text,
          streaming: true,
        };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "assistant.completed": {
      const msgs = [...state.messages];
      const idx = findLastIndex(
        msgs,
        (msg) => msg.kind === "assistant_message" && turnMatches(msg, turnId),
      );
      if (idx >= 0) {
        msgs[idx] = { ...msgs[idx], seq: event.seq, streaming: false };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "tool.started": {
      const toolCallId = payloadString(event.payload, "tool_call_id", "id");
      const toolName = payloadString(event.payload, "tool_name", "name") ?? "tool";
      const argsPreview = payloadPreview(event.payload, "arguments", "args_preview", "args");
      const msgs = [...state.messages];
      const idx = toolCallId
        ? msgs.findIndex((m) => m.kind === "tool_call" && m.toolCallId === toolCallId)
        : -1;
      const nextToolCall: ChatMessage = {
        id: toolCallId ? `tc_${toolCallId}` : `tc_${event.seq}`,
        seq: event.seq,
        turnId,
        kind: "tool_call",
        toolCallId,
        toolName,
        argsPreview,
        status: "running",
      };
      if (idx >= 0) {
        msgs[idx] = { ...msgs[idx], ...nextToolCall };
      } else {
        msgs.push(nextToolCall);
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "tool.completed": {
      const toolCallId = payloadString(event.payload, "tool_call_id", "id");
      const isError = event.payload.is_error === true;
      const msgs = [...state.messages];
      const idx = toolCallId
        ? msgs.findIndex((m) => m.kind === "tool_call" && m.toolCallId === toolCallId)
        : -1;
      if (idx >= 0) {
        msgs[idx] = {
          ...msgs[idx],
          seq: event.seq,
          status: isError ? "failed" : "completed",
          resultPreview: payloadPreview(event.payload, "result_preview", "output"),
          isError,
        };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "system.notice":
    case "task_result_ready": {
      const text =
        payloadString(event.payload, "text", "message", "title") ??
        (eventType(event.type) === "task_result_ready" ? "Task result ready." : "");
      const msgs = [...state.messages];
      msgs.push({
        id: `notice_${event.seq}`,
        seq: event.seq,
        turnId,
        kind: "notice",
        text,
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "turn.failed": {
      const msgs = [...state.messages];
      msgs.push({
        id: `error_${event.seq}`,
        seq: event.seq,
        turnId,
        kind: "error",
        text: payloadString(event.payload, "message", "error") ?? "Turn failed.",
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "turn.interrupted": {
      const msgs = [...state.messages];
      msgs.push({
        id: `notice_${event.seq}`,
        seq: event.seq,
        turnId,
        kind: "notice",
        text: "Turn interrupted. Waiting for further input.",
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    default:
      return { ...state, latestSeq: nextSeq };
  }
}

// ─── Timeline → ChatMessage conversion ─────────────────────────────────────────

export function timelineToMessages(blocks: TimelineBlock[]): ChatMessage[] {
  return blocks.flatMap((block, index) => timelineBlockToMessages(block, index));
}

function timelineBlockToMessages(block: TimelineBlock, index: number): ChatMessage[] {
  if (isToolKind(block.kind)) {
    return [toolMessageFromBlock(block, index)];
  }

  const messages: ChatMessage[] = [];
  const text = firstTimelineText(block);
  if (text) {
    messages.push({
      id: `tl_${index}_${block.kind}`,
      kind: chatKindFromTimelineKind(block.kind),
      text,
      status: block.status,
      streaming: block.status === "streaming",
    });
  }

  for (const part of block.content ?? []) {
    if (isToolContent(part)) {
      messages.push(toolMessageFromContent(part, index));
    }
  }

  if (messages.length === 0 && block.title) {
    messages.push({
      id: `tl_${index}_${block.kind}`,
      kind: chatKindFromTimelineKind(block.kind),
      text: block.title,
      status: block.status,
    });
  }
  return messages;
}

function chatKindFromTimelineKind(kind: string): ChatMessage["kind"] {
  if (kind === "user_message" || kind === "user") {
    return "user_message";
  }
  if (kind === "assistant_message" || kind === "assistant") {
    return "assistant_message";
  }
  if (kind === "error") {
    return "error";
  }
  return "notice";
}

function isToolKind(kind: string): boolean {
  return kind === "tool_call" || kind === "tool";
}

function isToolContent(part: TimelineContent): boolean {
  return part.type === "tool_call" || part.type === "tool";
}

function toolMessageFromBlock(block: TimelineBlock, index: number): ChatMessage {
  const tool = (block.content ?? []).find(isToolContent);
  return {
    id: `tl_tool_${tool?.tool_call_id ?? index}`,
    kind: "tool_call",
    toolCallId: tool?.tool_call_id,
    toolName: tool?.tool_name ?? block.title ?? "tool",
    argsPreview: tool?.args_preview,
    resultPreview: tool?.result_preview ?? block.text,
    status: tool?.status ?? block.status,
    isError: (tool?.status ?? block.status) === "failed",
  };
}

function toolMessageFromContent(part: TimelineContent, index: number): ChatMessage {
  return {
    id: `tl_tool_${part.tool_call_id ?? index}`,
    kind: "tool_call",
    toolCallId: part.tool_call_id,
    toolName: part.tool_name ?? "tool",
    argsPreview: part.args_preview,
    resultPreview: part.result_preview,
    status: part.status,
    isError: part.status === "failed",
  };
}

function firstTimelineText(block: TimelineBlock): string | undefined {
  if (typeof block.text === "string" && block.text !== "") {
    return block.text;
  }
  if (!Array.isArray(block.content) || block.content.length === 0) {
    return block.text;
  }

  const text = block.content
    .filter((part) => part.type === "text" && typeof part.text === "string" && part.text !== "")
    .map((part) => part.text)
    .join("");

  return text !== "" ? text : block.text;
}

// ─── Hook ─────────────────────────────────────────────────────────────────────

export function useSessionEvents(
  sessionId: string | null,
  afterSeq: number,
  onEvent: (event: SSEEvent) => void,
  onConnectionChange: (status: ChatState["connection"]) => void,
  enabled = true,
) {
  const streamRef = useRef<{ close: () => void } | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const afterSeqRef = useRef(afterSeq);
  const onEventRef = useRef(onEvent);
  const onConnectionChangeRef = useRef(onConnectionChange);
  const enabledRef = useRef(enabled);

  const connect = useCallback(() => {
    if (!sessionId || !enabledRef.current) return;
    if (reconnectTimerRef.current != null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    streamRef.current?.close();
    onConnectionChangeRef.current("connecting");
    const stream = openEventStream(
      sessionId,
      afterSeqRef.current,
      (event) => {
        afterSeqRef.current = Math.max(afterSeqRef.current, event.seq);
        onEventRef.current(event);
      },
      () => onConnectionChangeRef.current("open"),
      () => {
        onConnectionChangeRef.current("error");
        stream.close();
        if (streamRef.current === stream) {
          streamRef.current = null;
        }
        reconnectTimerRef.current = window.setTimeout(() => {
          reconnectTimerRef.current = null;
          onConnectionChangeRef.current("reconnecting");
          connect();
        }, 1000);
      },
    );
    streamRef.current = stream;
  }, [sessionId]);

  useEffect(() => {
    afterSeqRef.current = afterSeq;
  }, [afterSeq]);

  useEffect(() => {
    enabledRef.current = enabled;
    if (!enabled) {
      if (reconnectTimerRef.current != null) {
        window.clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      streamRef.current?.close();
      streamRef.current = null;
      onConnectionChangeRef.current("idle");
    }
  }, [enabled]);

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    onConnectionChangeRef.current = onConnectionChange;
  }, [onConnectionChange]);

  useEffect(() => {
    if (enabled) {
      connect();
    }
    return () => {
      if (reconnectTimerRef.current != null) {
        window.clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      streamRef.current?.close();
      streamRef.current = null;
    };
  }, [connect, enabled]);

  return {
    reconnect: connect,
    disconnect: () => streamRef.current?.close(),
  };
}
