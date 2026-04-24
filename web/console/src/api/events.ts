import { useEffect, useRef, useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { openEventStream } from "./client";
import type {
  ServerEvent,
  ToolEventPayload,
  DeltaPayload,
  FailurePayload,
  PermissionRequestPayload,
  PermissionResolvedPayload,
  NoticePayload,
  SessionMemoryPayload,
} from "./types";

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
    | "error"
    | "permission_block";
  text?: string;
  streaming?: boolean;
  status?: string;
  toolCallId?: string;
  toolName?: string;
  argsPreview?: string;
  resultPreview?: string;
  isError?: boolean;
  permissionRequestId?: string;
  requestedProfile?: string;
  reason?: string;
  decision?: string;
  usage?: {
    input_tokens: number;
    output_tokens: number;
    cached_tokens: number;
    cache_hit_rate: number;
  };
}

export interface SessionMemory {
  phase: "idle" | "running" | "updated" | "failed";
  text: string;
}

export interface ChatState {
  messages: ChatMessage[];
  latestSeq: number;
  connection: "idle" | "connecting" | "open" | "reconnecting" | "error";
  sessionMemory: SessionMemory;
}

export type ChatAction =
  | { type: "init"; messages: ChatMessage[]; latestSeq: number }
  | { type: "connection"; value: ChatState["connection"] }
  | { type: "event"; event: ServerEvent };

export const initialState: ChatState = {
  messages: [],
  latestSeq: 0,
  connection: "idle",
  sessionMemory: { phase: "idle", text: "" },
};

// ─── Reducer ──────────────────────────────────────────────────────────────────

export function reduceChat(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case "init": {
      // Merge: use timeline as the base, but preserve any SSE messages
      // that arrived after the timeline's latestSeq (they aren't in the
      // timeline snapshot yet, so dropping them would lose live data).
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
  if (
    a.kind === "permission_block" &&
    a.permissionRequestId &&
    a.permissionRequestId === b.permissionRequestId
  ) {
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

function turnMatches(msg: ChatMessage, turnId: string | undefined): boolean {
  return !turnId || !msg.turnId || msg.turnId === turnId;
}

function applyEvent(state: ChatState, event: ServerEvent): ChatState {
  const nextSeq = Math.max(state.latestSeq, event.seq);

  switch (event.type) {
    case "user_message": {
      const text =
        typeof (event.payload as { text?: unknown })?.text === "string"
          ? (event.payload as { text: string }).text
          : "";
      const msgs = [...state.messages];
      msgs.push({
        id: event.id,
        seq: event.seq,
        turnId: event.turn_id,
        kind: "user_message",
        text,
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "turn.started":
      return { ...state, latestSeq: nextSeq };

    case "assistant.delta": {
      const payload = event.payload as DeltaPayload;
      const msgs = [...state.messages];
      const idx = findLastIndex(
        msgs,
        (msg) =>
          msg.kind === "assistant_message" &&
          msg.streaming === true &&
          turnMatches(msg, event.turn_id),
      );
      if (idx < 0) {
        msgs.push({
          id: `a_${event.seq}_${event.turn_id ?? crypto.randomUUID()}`,
          seq: event.seq,
          turnId: event.turn_id,
          kind: "assistant_message",
          text: payload.text,
          streaming: true,
        });
      } else {
        msgs[idx] = {
          ...msgs[idx],
          seq: event.seq,
          text: (msgs[idx].text ?? "") + payload.text,
          streaming: true,
        };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "assistant.completed": {
      const msgs = [...state.messages];
      const idx = findLastIndex(
        msgs,
        (msg) => msg.kind === "assistant_message" && turnMatches(msg, event.turn_id),
      );
      if (idx >= 0) {
        msgs[idx] = { ...msgs[idx], seq: event.seq, streaming: false };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "tool.started": {
      const p = event.payload as ToolEventPayload;
      const msgs = [...state.messages];
      const idx = p.tool_call_id
        ? msgs.findIndex((m) => m.kind === "tool_call" && m.toolCallId === p.tool_call_id)
        : -1;
      const nextToolCall: ChatMessage = {
        id: p.tool_call_id ? `tc_${p.tool_call_id}` : `tc_${event.seq}`,
        seq: event.seq,
        turnId: event.turn_id,
        kind: "tool_call",
        toolCallId: p.tool_call_id,
        toolName: p.tool_name,
        argsPreview: p.arguments,
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
      const p = event.payload as ToolEventPayload;
      const msgs = [...state.messages];
      const idx = msgs.findIndex(
        (m) => m.kind === "tool_call" && m.toolCallId === p.tool_call_id,
      );
      if (idx >= 0) {
        msgs[idx] = {
          ...msgs[idx],
          seq: event.seq,
          status: p.is_error ? "failed" : "completed",
          resultPreview: p.result_preview,
          isError: p.is_error,
        };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "permission.requested": {
      const p = event.payload as PermissionRequestPayload;
      const text = p.reason
        ? `Permission needed: ${p.requested_profile}. Reason: ${p.reason}`
        : `Permission needed: ${p.requested_profile}`;
      const msgs = [...state.messages];
      msgs.push({
        id: `perm_${p.request_id ?? event.seq}`,
        seq: event.seq,
        turnId: event.turn_id,
        kind: "permission_block",
        permissionRequestId: p.request_id,
        requestedProfile: p.requested_profile,
        reason: p.reason,
        text,
        status: "requested",
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "permission.resolved": {
      const p = event.payload as PermissionResolvedPayload;
      const msgs = [...state.messages];
      const idx = msgs.findIndex(
        (m) =>
          m.kind === "permission_block" && m.permissionRequestId === p.request_id,
      );
      if (idx >= 0) {
        msgs[idx] = {
          ...msgs[idx],
          seq: event.seq,
          status: p.decision,
          text: `Permission ${p.decision}: ${p.requested_profile}`,
          decision: p.decision,
        };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "system.notice": {
      const p = event.payload as NoticePayload;
      const msgs = [...state.messages];
      msgs.push({
        id: `notice_${event.seq}`,
        seq: event.seq,
        turnId: event.turn_id,
        kind: "notice",
        text: p.text,
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "turn.failed": {
      const p = event.payload as FailurePayload;
      const msgs = [...state.messages];
      msgs.push({
        id: `error_${event.seq}`,
        seq: event.seq,
        turnId: event.turn_id,
        kind: "error",
        text: p.message,
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "turn.interrupted": {
      const msgs = [...state.messages];
      msgs.push({
        id: `notice_${event.seq}`,
        seq: event.seq,
        turnId: event.turn_id,
        kind: "notice",
        text: "Turn interrupted. Waiting for further input.",
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "session_memory.started":
      return {
        ...state,
        latestSeq: nextSeq,
        sessionMemory: { phase: "running", text: "Summarizing session memory..." },
      };

    case "session_memory.completed": {
      const p = event.payload as SessionMemoryPayload;
      const parts: string[] = [];
      if (p.workspace_entries_upserted) parts.push(`+${p.workspace_entries_upserted} workspace`);
      if (p.global_entries_upserted) parts.push(`+${p.global_entries_upserted} global`);
      if (p.workspace_entries_pruned) parts.push(`-${p.workspace_entries_pruned} pruned`);
      return {
        ...state,
        latestSeq: nextSeq,
        sessionMemory: {
          phase: "updated",
          text: parts.length > 0 ? `Memory updated: ${parts.join(" / ")}` : "Session memory updated",
        },
      };
    }

    case "session_memory.failed": {
      const p = event.payload as SessionMemoryPayload;
      return {
        ...state,
        latestSeq: nextSeq,
        sessionMemory: {
          phase: "failed",
          text: p.message ?? "Session memory refresh failed",
        },
      };
    }

    case "context.compacted": {
      const msgs = [...state.messages];
      msgs.push({
        id: `notice_${event.seq}`,
        seq: event.seq,
        turnId: event.turn_id,
        kind: "notice",
        text: "Context was compacted by the system.",
      });
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    default:
      return { ...state, latestSeq: nextSeq };
  }
}

// ─── Timeline → ChatMessage conversion ─────────────────────────────────────────

export function timelineToMessages(
  blocks: import("./types").TimelineBlock[],
): ChatMessage[] {
  return blocks.map((b) => {
    if (b.kind === "tool_call") {
      return {
        id: `tl_${b.tool_call_id ?? b.id}`,
        turnId: b.turn_id,
        kind: "tool_call",
        toolCallId: b.tool_call_id,
        toolName: b.tool_name,
        argsPreview: b.args_preview,
        resultPreview: b.result_preview,
        status: b.status,
        isError: b.status === "failed",
      };
    }
    if (b.kind === "permission_block") {
      return {
        id: `tl_${b.permission_request_id ?? b.id}`,
        turnId: b.turn_id,
        kind: "permission_block",
        permissionRequestId: b.permission_request_id,
        requestedProfile: b.requested_profile,
        reason: b.reason,
        text: b.text,
        status: b.status,
        decision: b.decision,
      };
    }
    const text = firstTimelineText(b);
    return {
      id: `tl_${b.id}`,
      turnId: b.turn_id,
      kind: b.kind as ChatMessage["kind"],
      text,
      status: b.status,
      streaming: b.status === "streaming",
      usage: b.usage,
    };
  });
}

function firstTimelineText(block: import("./types").TimelineBlock): string | undefined {
  if (typeof block.text === "string" && block.text !== "") {
    return block.text;
  }
  if (!Array.isArray(block.content) || block.content.length === 0) {
    return block.text;
  }

  const text = block.content
    .filter(
      (part): part is Extract<import("./types").ContentBlock, { type: "text" }> =>
        part.type === "text" && typeof part.text === "string" && part.text !== "",
    )
    .map((part) => part.text)
    .join("");

  return text !== "" ? text : block.text;
}

// ─── Hook ─────────────────────────────────────────────────────────────────────

export function useSessionEvents(
  sessionId: string | null,
  afterSeq: number,
  onEvent: (event: ServerEvent) => void,
  onConnectionChange: (status: ChatState["connection"]) => void,
) {
  const esRef = useRef<EventSource | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const afterSeqRef = useRef(afterSeq);
  const onEventRef = useRef(onEvent);
  const onConnectionChangeRef = useRef(onConnectionChange);

  const connect = useCallback(() => {
    if (!sessionId) return;
    if (reconnectTimerRef.current != null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    esRef.current?.close();
    onConnectionChangeRef.current("connecting");
    const es = openEventStream(sessionId, afterSeqRef.current);
    esRef.current = es;

    es.addEventListener("open", () => onConnectionChangeRef.current("open"));
    es.addEventListener("error", () => {
      onConnectionChangeRef.current("reconnecting");
      es.close();
      if (esRef.current === es) {
        esRef.current = null;
      }
      reconnectTimerRef.current = window.setTimeout(() => {
        reconnectTimerRef.current = null;
        connect();
      }, 1000);
    });

    const dispatchEvent = (e: MessageEvent) => {
      if (!e.data) return;
      try {
        onEventRef.current(JSON.parse(e.data));
      } catch {
        // ignore parse errors
      }
    };

    es.addEventListener("message", (e: MessageEvent) => {
      dispatchEvent(e);
    });

    // Named SSE events
    es.addEventListener("turn.started", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("assistant.delta", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("assistant.completed", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("tool.started", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("tool.completed", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("permission.requested", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("permission.resolved", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("system.notice", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("turn.failed", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("turn.interrupted", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("session_memory.started", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("session_memory.completed", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("session_memory.failed", (e: MessageEvent) => {
      dispatchEvent(e);
    });
    es.addEventListener("context.compacted", (e: MessageEvent) => {
      dispatchEvent(e);
    });
  }, [sessionId]);

  useEffect(() => {
    afterSeqRef.current = afterSeq;
  }, [afterSeq]);

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    onConnectionChangeRef.current = onConnectionChange;
  }, [onConnectionChange]);

  useEffect(() => {
    connect();
    return () => {
      if (reconnectTimerRef.current != null) {
        window.clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      esRef.current?.close();
      esRef.current = null;
    };
  }, [connect]);

  return {
    reconnect: connect,
    disconnect: () => esRef.current?.close(),
  };
}
