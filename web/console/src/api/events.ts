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
    case "init":
      return { ...state, messages: action.messages, latestSeq: action.latestSeq };
    case "connection":
      return { ...state, connection: action.value };
    case "event":
      return applyEvent(state, action.event);
    default:
      return state;
  }
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
      let idx = msgs.length - 1;
      while (idx >= 0 && (msgs[idx].kind !== "assistant_message" || msgs[idx].streaming !== true))
        idx--;
      if (idx < 0) {
        msgs.push({
          id: `a_${event.seq}_${crypto.randomUUID()}`,
          turnId: event.turn_id,
          kind: "assistant_message",
          text: payload.text,
          streaming: true,
        });
      } else {
        msgs[idx] = {
          ...msgs[idx],
          text: (msgs[idx].text ?? "") + payload.text,
          streaming: true,
        };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "assistant.completed": {
      const msgs = [...state.messages];
      let idx = msgs.length - 1;
      while (idx >= 0 && msgs[idx].kind !== "assistant_message") idx--;
      if (idx >= 0) {
        msgs[idx] = { ...msgs[idx], streaming: false };
      }
      return { ...state, messages: msgs, latestSeq: nextSeq };
    }

    case "tool.started": {
      const p = event.payload as ToolEventPayload;
      const msgs = [...state.messages];
      msgs.push({
        id: `tc_${p.tool_call_id}`,
        turnId: event.turn_id,
        kind: "tool_call",
        toolCallId: p.tool_call_id,
        toolName: p.tool_name,
        argsPreview: p.arguments,
        status: "running",
      });
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

  const connect = useCallback(() => {
    if (!sessionId) return;
    onConnectionChange("connecting");
    const es = openEventStream(sessionId, afterSeq);
    esRef.current = es;

    es.addEventListener("open", () => onConnectionChange("open"));
    es.addEventListener("error", () => {
      onConnectionChange("reconnecting");
      es.close();
    });

    es.addEventListener("message", (e: MessageEvent) => {
      if (!e.data) return;
      try {
        const event: ServerEvent = JSON.parse(e.data);
        onEvent(event);
      } catch {
        // ignore parse errors
      }
    });

    // Named SSE events
    es.addEventListener("turn.started", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("assistant.delta", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("assistant.completed", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("tool.started", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("tool.completed", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("permission.requested", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("permission.resolved", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("system.notice", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("turn.failed", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("turn.interrupted", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("session_memory.started", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("session_memory.completed", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("session_memory.failed", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
    es.addEventListener("context.compacted", (e: MessageEvent) => {
      try { onEvent(JSON.parse(e.data)); } catch {}
    });
  }, [sessionId, afterSeq, onEvent, onConnectionChange]);

  useEffect(() => {
    connect();
    return () => {
      esRef.current?.close();
      esRef.current = null;
    };
  }, [connect]);

  return {
    reconnect: connect,
    disconnect: () => esRef.current?.close(),
  };
}
