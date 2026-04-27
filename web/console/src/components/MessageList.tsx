import { useEffect, useRef, useState } from "react";
import type { ChatMessage } from "../api/events";
import { UserMessage } from "./blocks/UserMessage";
import { AssistantMessage } from "./blocks/AssistantMessage";
import { ToolCallGroup } from "./blocks/ToolCallGroup";
import { NoticeBlock } from "./blocks/NoticeBlock";
import { ErrorBlock } from "./blocks/ErrorBlock";
import { ArrowDown } from "./Icon";
import { useI18n } from "../i18n";

interface MessageListProps {
  messages: ChatMessage[];
  connection: "idle" | "connecting" | "open" | "reconnecting" | "error";
  onSuggestionClick?: (text: string) => void;
  suggestionsDisabled?: boolean;
}

export function MessageList({ messages, connection, onSuggestionClick, suggestionsDisabled }: MessageListProps) {
  const { t } = useI18n();
  const suggestions = [
    t("chat.suggestions.explainCodebase"),
    t("chat.suggestions.runTests"),
    t("chat.suggestions.checkGitStatus"),
  ];
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
  }, [messages, autoScroll]);

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

  // Group messages by turn; collect consecutive tool_calls into ToolCallGroup
  type BlockItem =
    | { kind: "single"; msg: ChatMessage }
    | { kind: "tool_group"; msgs: ChatMessage[] };

  const turns: { user?: ChatMessage; blocks: BlockItem[] }[] = [];
  for (const msg of messages) {
    if (msg.kind === "user_message") {
      turns.push({ user: msg, blocks: [] });
    } else {
      if (turns.length === 0) {
        turns.push({ blocks: [] });
      }

      if (msg.kind === "tool_call") {
        const blocks = turns[turns.length - 1].blocks;
        const last = blocks[blocks.length - 1];
        if (last && last.kind === "tool_group") {
          last.msgs.push(msg);
        } else {
          blocks.push({ kind: "tool_group", msgs: [msg] });
        }
      } else {
        turns[turns.length - 1].blocks.push({ kind: "single", msg });
      }
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
          <div className="flex flex-col items-center justify-center h-full select-none">
            {/* Decorative ring */}
            <div
              className="w-16 h-16 rounded-full mb-6"
              style={{
                background: "conic-gradient(from 0deg, var(--color-accent), var(--color-assistant), var(--color-tool), var(--color-accent))",
                opacity: 0.15,
                filter: "blur(1px)",
              }}
            />
            <p
              className="text-base mb-4"
              style={{ color: "var(--color-text-muted)" }}
            >
              {t("chat.emptyPrompt")}
            </p>
            {onSuggestionClick && (
              <div className="flex flex-wrap gap-2 justify-center">
                {suggestions.map((s) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => onSuggestionClick(s)}
                    disabled={suggestionsDisabled}
                    className="rounded-full px-4 py-1.5 text-sm"
                    style={{
                      backgroundColor: "var(--color-surface)",
                      border: "1px solid var(--color-border)",
                      color: "var(--color-text-muted)",
                      cursor: suggestionsDisabled ? "not-allowed" : "pointer",
                      opacity: suggestionsDisabled ? 0.4 : 1,
                      transition: "border-color 0.15s, color 0.15s, opacity 0.15s",
                    }}
                    onMouseEnter={(e) => {
                      if (suggestionsDisabled) return;
                      e.currentTarget.style.borderColor = "var(--color-accent)";
                      e.currentTarget.style.color = "var(--color-text)";
                    }}
                    onMouseLeave={(e) => {
                      if (suggestionsDisabled) return;
                      e.currentTarget.style.borderColor = "var(--color-border)";
                      e.currentTarget.style.color = "var(--color-text-muted)";
                    }}
                  >
                    {s}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}

        {turns.map((turn, ti) => (
          <div key={ti} className="mb-6">
            {turn.user && (
              <UserMessage
                text={turn.user.text ?? ""}
              />
            )}
            {turn.blocks.map((block, bi) => {
              if (block.kind === "tool_group") {
                return <ToolCallGroup key={`tg-${ti}-${bi}`} messages={block.msgs} />;
              }
              const msg = block.msg;
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
                case "notice":
                  return <NoticeBlock key={msg.id} text={msg.text ?? ""} />;
                case "error":
                  return <ErrorBlock key={msg.id} text={msg.text ?? ""} />;
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
          className="absolute bottom-4 left-1/2 -translate-x-1/2 flex items-center gap-1.5 px-4 py-2 rounded-full text-sm"
          style={{
            backgroundColor: "var(--color-accent)",
            color: "#fff",
            border: "none",
            cursor: "pointer",
            boxShadow: "0 2px 8px rgba(0,0,0,0.25)",
          }}
        >
          <ArrowDown size={14} color="#fff" />
          {t("chat.newMessages")}
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
        {connection === "connecting" && t("chat.connecting")}
        {connection === "reconnecting" && t("chat.reconnecting")}
        {connection === "open" && t("chat.connected")}
        {connection === "error" && t("chat.error")}
        {connection === "idle" && t("chat.idle")}
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
