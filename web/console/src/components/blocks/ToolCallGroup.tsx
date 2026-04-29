import { useEffect, useState } from "react";
import type { ChatMessage } from "../../api/events";
import { ToolCall } from "./ToolCall";
import { Wrench, ChevronDown, ChevronUp } from "../Icon";

interface ToolCallGroupProps {
  messages: ChatMessage[];
}

export function ToolCallGroup({ messages }: ToolCallGroupProps) {
  const hasRunning = messages.some((m) => m.status === "running");
  const failedCount = messages.filter((m) => m.isError || m.status === "failed").length;
  const completedCount = messages.filter((m) => m.status === "completed").length;
  const [expanded, setExpanded] = useState(hasRunning);

  useEffect(() => {
    if (hasRunning) setExpanded(true);
  }, [hasRunning]);

  const toolNames = messages.map((m) => m.toolName ?? "tool");
  const uniqueNames = [...new Set(toolNames)];

  return (
    <div className="mb-4">
      {/* Group header */}
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="w-full cursor-pointer text-left flex items-center gap-2 px-3 py-2 rounded-md"
        style={{
          backgroundColor: "var(--color-tool-dim)",
          border: "1px solid rgba(245,158,11,0.15)",
          transition: "background-color 0.15s",
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.backgroundColor = "rgba(245,158,11,0.18)";
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.backgroundColor = "var(--color-tool-dim)";
        }}
      >
        <Wrench size={14} color="var(--color-tool)" />
        <span className="text-xs font-medium" style={{ color: "var(--color-tool)" }}>
          {messages.length === 1
            ? toolNames[0]
            : `${messages.length} tool calls`}
        </span>
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          ({uniqueNames.join(" · ")})
        </span>

        {failedCount > 0 && (
          <span
            className="text-[11px] px-1.5 py-0.5 rounded font-medium ml-auto"
            style={{
              backgroundColor: "var(--color-error-dim)",
              color: "var(--color-error)",
            }}
          >
            {failedCount} failed
          </span>
        )}
        {failedCount === 0 && completedCount === messages.length && messages.length > 1 && (
          <span
            className="text-[11px] px-1.5 py-0.5 rounded font-medium ml-auto"
            style={{
              backgroundColor: "var(--color-success-dim)",
              color: "var(--color-success)",
            }}
          >
            done
          </span>
        )}

        <span style={{ color: "var(--color-text-tertiary)", marginLeft: expanded ? 0 : "auto" }}>
          {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </span>
      </button>

      {/* Individual tool calls */}
      {expanded && (
        <div className="mt-1.5 space-y-1.5 pl-2" style={{ borderLeft: "2px solid var(--color-border)" }}>
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
