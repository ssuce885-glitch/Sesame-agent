import { useEffect, useRef, useState } from "react";
import type { ChatMessage } from "../api/events";
import { UserMessage } from "./blocks/UserMessage";
import { AssistantMessage } from "./blocks/AssistantMessage";
import { ToolCall } from "./blocks/ToolCall";
import { NoticeBlock } from "./blocks/NoticeBlock";
import { ErrorBlock } from "./blocks/ErrorBlock";
import { PermissionBlock } from "./blocks/PermissionBlock";

interface MessageListProps {
  messages: ChatMessage[];
  connection: "idle" | "connecting" | "open" | "reconnecting" | "error";
}

export function MessageList({ messages, connection }: MessageListProps) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [newMsg, setNewMsg] = useState(false);

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    if (autoScroll) {
      bottomRef.current?.scrollIntoView({ behavior: preferredScrollBehavior() });
    } else {
      setNewMsg(true);
    }
  }, [messages.length, autoScroll]);

  function handleScroll() {
    const el = containerRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    setAutoScroll(atBottom);
    if (atBottom) setNewMsg(false);
  }

  function scrollToBottom() {
    bottomRef.current?.scrollIntoView({ behavior: preferredScrollBehavior() });
    setAutoScroll(true);
    setNewMsg(false);
  }

  // Group messages by turn, separating user messages from assistant+tool blocks
  const turns: { user?: ChatMessage; blocks: ChatMessage[] }[] = [];
  for (const msg of messages) {
    if (msg.kind === "user_message") {
      turns.push({ user: msg, blocks: [] });
    } else {
      if (turns.length === 0) {
        turns.push({ blocks: [] });
      }
      turns[turns.length - 1].blocks.push(msg);
    }
  }

  return (
    <div className="relative flex-1 overflow-hidden">
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="h-full overflow-y-auto px-3 py-4 md:px-6"
        style={{ backgroundColor: "var(--color-bg)" }}
      >
        {messages.length === 0 && (
          <div
            className="flex flex-col items-center justify-center h-full text-sm"
            style={{ color: "var(--color-text-muted)" }}
          >
            <p>Send a message to start the conversation.</p>
          </div>
        )}

        {turns.map((turn, ti) => (
          <div key={ti} className="mb-6">
            {turn.user && (
              <UserMessage
                text={turn.user.text ?? ""}
              />
            )}
            {turn.blocks.map((msg) => {
              switch (msg.kind) {
                case "assistant_message":
                  return (
                    <AssistantMessage
                      key={msg.id}
                      text={msg.text ?? ""}
                      streaming={msg.streaming ?? false}
                      usage={msg.usage}
                    />
                  );
                case "tool_call":
                  return (
                    <ToolCall
                      key={msg.id}
                      toolName={msg.toolName ?? "tool"}
                      argsPreview={msg.argsPreview}
                      resultPreview={msg.resultPreview}
                      status={msg.status ?? "running"}
                      isError={msg.isError ?? false}
                    />
                  );
                case "notice":
                  return <NoticeBlock key={msg.id} text={msg.text ?? ""} />;
                case "error":
                  return <ErrorBlock key={msg.id} text={msg.text ?? ""} />;
                case "permission_block":
                  return (
                    <PermissionBlock
                      key={msg.id}
                      requestId={msg.permissionRequestId ?? ""}
                      profile={msg.requestedProfile ?? ""}
                      reason={msg.reason}
                      decision={msg.decision}
                      text={msg.text ?? ""}
                    />
                  );
                default:
                  return null;
              }
            })}
          </div>
        ))}

        <div ref={bottomRef} />
      </div>

      {/* New messages indicator */}
      {newMsg && !autoScroll && (
        <button
          onClick={scrollToBottom}
          className="absolute bottom-4 left-1/2 -translate-x-1/2 px-4 py-2 rounded-full text-sm"
          style={{
            backgroundColor: "var(--color-accent)",
            color: "#fff",
            border: "none",
            cursor: "pointer",
            boxShadow: "0 2px 8px rgba(0,0,0,0.15)",
          }}
        >
          ↓ New messages
        </button>
      )}

      {/* Connection status */}
      <div
        className="absolute top-3 right-4 flex items-center gap-1.5 text-xs"
        style={{ color: "var(--color-text-muted)" }}
      >
        <span
          className="inline-block w-2 h-2 rounded-full"
          style={{
            backgroundColor:
              connection === "open"
                ? "var(--color-success)"
                : connection === "reconnecting" || connection === "connecting"
                ? "var(--color-warning)"
                : connection === "error"
                ? "var(--color-error)"
                : "var(--color-border)",
          }}
        />
        {connection === "connecting" && "Connecting..."}
        {connection === "reconnecting" && "Reconnecting..."}
        {connection === "open" && "Connected"}
        {connection === "error" && "Error"}
        {connection === "idle" && "Idle"}
      </div>
    </div>
  );
}

function preferredScrollBehavior(): ScrollBehavior {
  if (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  ) {
    return "auto";
  }
  return "smooth";
}
