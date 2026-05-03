import { useEffect, useRef, useState } from "react";
import type { ChatMessage } from "../api/events";
import { UserMessage } from "./blocks/UserMessage";
import { AssistantMessage } from "./blocks/AssistantMessage";
import { ToolCallGroup } from "./blocks/ToolCallGroup";
import { ErrorBlock } from "./blocks/ErrorBlock";
import { ArrowDown } from "./Icon";
import { useI18n } from "../i18n";

interface MessageListProps {
  messages: ChatMessage[];
  connection: "idle" | "connecting" | "open" | "reconnecting" | "error";
  onSuggestionClick?: (text: string) => void;
  suggestionsDisabled?: boolean;
}

type BlockItem =
  | { kind: "single"; msg: ChatMessage }
  | { kind: "tool_group"; msgs: ChatMessage[] };

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
        className="h-full overflow-y-auto px-4 py-5 md:px-8"
        style={{ backgroundColor: "var(--color-bg)" }}
      >
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full select-none">
            {/* Brand mark */}
            <div
              className="w-12 h-12 rounded-lg flex items-center justify-center mb-5"
              style={{ backgroundColor: "var(--color-accent-dim)" }}
            >
              <span className="text-xl font-bold" style={{ color: "var(--color-accent)" }}>
                S
              </span>
            </div>
            <p className="text-sm mb-1" style={{ color: "var(--color-text)" }}>
              {t("chat.emptyPrompt")}
            </p>
            <p className="text-xs mb-5" style={{ color: "var(--color-text-tertiary)" }}>
              Sesame personal assistant
            </p>
            {onSuggestionClick && (
              <div className="flex flex-wrap gap-2 justify-center">
                {suggestions.map((s) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => onSuggestionClick(s)}
                    disabled={suggestionsDisabled}
                    className="rounded-full px-3.5 py-1.5 text-xs font-medium"
                    style={{
                      backgroundColor: "var(--color-surface)",
                      border: "1px solid var(--color-border)",
                      color: "var(--color-text-secondary)",
                      cursor: suggestionsDisabled ? "not-allowed" : "pointer",
                      opacity: suggestionsDisabled ? 0.4 : 1,
                      transition: "border-color 0.15s, color 0.15s",
                    }}
                    onMouseEnter={(e) => {
                      if (suggestionsDisabled) return;
                      e.currentTarget.style.borderColor = "var(--color-accent)";
                      e.currentTarget.style.color = "var(--color-text)";
                    }}
                    onMouseLeave={(e) => {
                      if (suggestionsDisabled) return;
                      e.currentTarget.style.borderColor = "var(--color-border)";
                      e.currentTarget.style.color = "var(--color-text-secondary)";
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
          <div key={ti} className="mb-2">
            {turn.user && (
              <UserMessage text={turn.user.text ?? ""} />
            )}
            {renderBlocks(turn.blocks, ti)}
          </div>
        ))}

        <div ref={bottomRef} />
      </div>

      {/* New messages floating button */}
      {newMsg && !autoScroll && (
        <button
          onClick={scrollToBottom}
          className="absolute bottom-4 left-1/2 -translate-x-1/2 flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-medium"
          style={{
            backgroundColor: "var(--color-accent)",
            color: "#fff",
            border: "none",
            cursor: "pointer",
            boxShadow: "0 2px 10px rgba(0,0,0,0.3)",
          }}
        >
          <ArrowDown size={12} color="#fff" />
          {t("chat.newMessages")}
        </button>
      )}

      {/* Connection status */}
      <div
        className="absolute top-3 right-4 flex items-center gap-1.5 text-[11px] font-medium"
        style={{ color: "var(--color-text-tertiary)" }}
      >
        <span
          className="inline-block w-1.5 h-1.5 rounded-full"
          style={{
            backgroundColor:
              connection === "open"
                ? "var(--color-success)"
                : connection === "reconnecting" || connection === "connecting"
                  ? "var(--color-warning)"
                  : connection === "error"
                    ? "var(--color-error)"
                    : "var(--color-border-strong)",
            boxShadow:
              connection === "open"
                ? "0 0 6px rgba(34,197,94,0.5)"
                : connection === "reconnecting"
                  ? "0 0 6px rgba(245,158,11,0.5)"
                  : undefined,
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

function renderBlocks(blocks: BlockItem[], turnIdx: number): React.ReactNode[] {
  const out: React.ReactNode[] = [];
  let i = 0;
  while (i < blocks.length) {
    const block = blocks[i];
    if (block.kind === "tool_group") {
      out.push(
        <ToolCallGroup key={`tg-${turnIdx}-${i}`} messages={block.msgs} />
      );
      i++;
      continue;
    }
    const msg = block.msg;
    if (msg.kind === "notice") {
      const text = msg.text ?? "";
      let count = 1;
      let j = i + 1;
      while (j < blocks.length) {
        const next = blocks[j];
        if (
          next.kind === "single" &&
          next.msg.kind === "notice" &&
          (next.msg.text ?? "") === text
        ) {
          count++;
          j++;
        } else {
          break;
        }
      }
      out.push(
        <NoticeMessage
          key={`notice-${turnIdx}-${i}`}
          text={text}
          count={count > 1 ? count : undefined}
        />
      );
      i = j;
      continue;
    }
    if (msg.kind === "error") {
      const text = msg.text ?? "";
      let count = 1;
      let j = i + 1;
      while (j < blocks.length) {
        const next = blocks[j];
        if (
          next.kind === "single" &&
          next.msg.kind === "error" &&
          (next.msg.text ?? "") === text
        ) {
          count++;
          j++;
        } else {
          break;
        }
      }
      out.push(
        <ErrorBlock
          key={`error-${turnIdx}-${i}`}
          text={text}
          count={count > 1 ? count : undefined}
        />
      );
      i = j;
      continue;
    }
    if (msg.kind === "assistant_message") {
      out.push(
        <AssistantMessage
          key={msg.id}
          text={msg.text ?? ""}
          streaming={msg.streaming ?? false}
        />
      );
    }
    i++;
  }
  return out;
}

function NoticeMessage({ text, count }: { text: string; count?: number }) {
  return (
    <div
      className="mb-3 rounded-md px-3 py-2 text-xs"
      style={{
        backgroundColor: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        color: "var(--color-text-tertiary)",
      }}
    >
      {text}
      {count && count > 1 ? <span className="ml-2 opacity-70">x{count}</span> : null}
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
