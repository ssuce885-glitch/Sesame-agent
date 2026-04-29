import { useId, useState } from "react";
import { Check, X, Circle, Play, ChevronDown, ChevronUp } from "../Icon";

export function ToolCall({
  toolName,
  argsPreview,
  resultPreview,
  status,
  isError,
}: {
  toolName: string;
  argsPreview?: string;
  resultPreview?: string;
  status: string;
  isError: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const detailsId = useId();

  const isRunning = status === "running";
  const isCompleted = status === "completed";
  const isFailed = isError || status === "failed";

  let statusIcon;
  let statusColor;
  if (isRunning) {
    statusIcon = <Play size={12} color="var(--color-running)" className="animate-pulse-amber" />;
    statusColor = "var(--color-running)";
  } else if (isFailed) {
    statusIcon = <X size={12} color="var(--color-error)" />;
    statusColor = "var(--color-error)";
  } else if (isCompleted) {
    statusIcon = <Check size={12} color="var(--color-success)" />;
    statusColor = "var(--color-success)";
  } else {
    statusIcon = <Circle size={12} color="var(--color-text-tertiary)" />;
    statusColor = "var(--color-text-tertiary)";
  }

  return (
    <div
      className="rounded-md overflow-hidden"
      style={{
        backgroundColor: "var(--color-bg-elevated)",
        border: "1px solid var(--color-border)",
        borderLeft: `2px solid ${isFailed ? "var(--color-error)" : isRunning ? "var(--color-running)" : "var(--color-tool)"}`,
      }}
    >
      {/* Header */}
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={detailsId}
        onClick={() => setExpanded((v) => !v)}
        className="w-full cursor-pointer text-left flex items-center gap-2 px-3 py-2"
        style={{ backgroundColor: "transparent", border: "none" }}
      >
        {statusIcon}
        <span className="text-xs font-semibold" style={{ color: statusColor }}>
          {toolName}
        </span>
        {argsPreview && (
          <span
            className="text-xs truncate flex-1"
            style={{ color: "var(--color-text-tertiary)", fontFamily: "var(--font-mono)" }}
          >
            {argsPreview.length > 55 ? argsPreview.slice(0, 55) + "…" : argsPreview}
          </span>
        )}
        <span style={{ color: "var(--color-text-tertiary)" }}>
          {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </span>
      </button>

      {/* Collapsed result preview */}
      {!expanded && resultPreview && (
        <div
          className="px-3 pb-2 text-xs truncate"
          style={{ color: isFailed ? "var(--color-error)" : "var(--color-text-tertiary)", paddingLeft: 30 }}
        >
          {resultPreview.length > 90 ? resultPreview.slice(0, 90) + "…" : resultPreview}
        </div>
      )}

      {/* Expanded details */}
      {expanded && (
        <div id={detailsId} className="px-3 pb-3 space-y-2" style={{ paddingLeft: 30 }}>
          {argsPreview && (
            <div>
              <div className="text-[11px] font-medium uppercase tracking-wide mb-1" style={{ color: "var(--color-text-tertiary)" }}>
                Arguments
              </div>
              <pre
                className="text-xs rounded px-2.5 py-2"
                style={{
                  backgroundColor: "var(--color-surface)",
                  border: "1px solid var(--color-border)",
                  color: "var(--color-text-secondary)",
                  fontFamily: "var(--font-mono)",
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-all",
                  lineHeight: 1.5,
                }}
              >
                {argsPreview}
              </pre>
            </div>
          )}
          {resultPreview && (
            <div>
              <div className="text-[11px] font-medium uppercase tracking-wide mb-1" style={{ color: "var(--color-text-tertiary)" }}>
                Result
              </div>
              <pre
                className="text-xs rounded px-2.5 py-2"
                style={{
                  backgroundColor: isFailed ? "var(--color-error-dim)" : "var(--color-surface)",
                  border: `1px solid ${isFailed ? "rgba(239,68,68,0.25)" : "var(--color-border)"}`,
                  color: isFailed ? "var(--color-error)" : "var(--color-text-secondary)",
                  fontFamily: "var(--font-mono)",
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-all",
                  lineHeight: 1.5,
                }}
              >
                {resultPreview}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
