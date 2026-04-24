import { useEffect, useState } from "react";
import type { ChatMessage } from "../../api/events";
import { ToolCall } from "./ToolCall";
import { ChevronDown, ChevronUp } from "../Icon";

interface ToolCallGroupProps {
  messages: ChatMessage[];
}

export function ToolCallGroup({ messages }: ToolCallGroupProps) {
  const hasRunning = messages.some((m) => m.status === "running");
  const failedCount = messages.filter((m) => m.isError || m.status === "failed").length;
  const [expanded, setExpanded] = useState(hasRunning);

  // Auto-expand when any tool starts running
  useEffect(() => {
    if (hasRunning) setExpanded(true);
  }, [hasRunning]);

  const toolNames = messages.map((m) => m.toolName ?? "tool");
  const uniqueNames = [...new Set(toolNames)];
  const collapsedLabel =
    messages.length === 1
      ? toolNames[0]
      : `${messages.length} tool calls: ${uniqueNames.join(" · ")}`;

  return (
    <div className="mb-4">
      {/* Collapsed header */}
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="w-full cursor-pointer text-left rounded-lg px-3 py-2"
        style={{
          backgroundColor: "var(--color-surface)",
          border: "1px solid var(--color-border)",
          transition: "background-color 0.15s, border-color 0.15s",
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.backgroundColor = "var(--color-surface-2)";
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.backgroundColor = "var(--color-surface)";
        }}
      >
        <div className="flex items-center gap-2 text-sm">
          {/* Running pulse indicator */}
          {hasRunning && (
            <span
              className="inline-block w-2 h-2 rounded-full animate-pulse-amber shrink-0"
              style={{ backgroundColor: "var(--color-running)" }}
            />
          )}

          <span
            className="font-medium truncate"
            style={{ color: "var(--color-tool)" }}
          >
            {collapsedLabel}
          </span>

          {failedCount > 0 && (
            <span
              className="text-xs px-1.5 py-0.5 rounded-full shrink-0"
              style={{
                backgroundColor: "rgba(220,38,38,0.12)",
                color: "var(--color-error)",
              }}
            >
              {failedCount} failed
            </span>
          )}

          <span
            className="ml-auto shrink-0"
            style={{ color: "var(--color-text-muted)" }}
          >
            {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
          </span>
        </div>
      </button>

      {/* Expanded: individual tool calls */}
      {expanded && (
        <div className="mt-1 space-y-1 pl-2" style={{ borderLeft: "2px solid var(--color-border)" }}>
          {messages.map((msg) => (
            <ToolCall
              key={msg.id}
              toolName={msg.toolName ?? "tool"}
              argsPreview={msg.argsPreview}
              resultPreview={msg.resultPreview}
              status={msg.status ?? "running"}
              isError={msg.isError ?? false}
            />
          ))}
        </div>
      )}
    </div>
  );
}
