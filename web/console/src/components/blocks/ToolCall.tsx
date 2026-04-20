import { useId, useState } from "react";

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

  function StatusDot() {
    if (isRunning) {
      return (
        <span
          className="inline-block w-2 h-2 rounded-full animate-pulse-amber"
          style={{ backgroundColor: "var(--color-running)" }}
        />
      );
    }
    if (isFailed) {
      return (
        <span style={{ color: "var(--color-error)", fontSize: 12 }}>✗</span>
      );
    }
    if (isCompleted) {
      return (
        <span style={{ color: "var(--color-success)", fontSize: 12 }}>✓</span>
      );
    }
    return <span style={{ color: "var(--color-text-muted)", fontSize: 12 }}>○</span>;
  }

  return (
    <div
      className="mb-1 rounded-lg px-3 py-2 text-sm"
      style={{
        backgroundColor: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderLeft: `3px solid ${
          isFailed
            ? "var(--color-error)"
            : isRunning
            ? "var(--color-running)"
            : "var(--color-tool)"
        }`,
      }}
    >
      {/* Inline header */}
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={detailsId}
        aria-label={`${toolName} tool call`}
        onClick={() => setExpanded((v) => !v)}
        className="w-full cursor-pointer text-left"
        style={{ backgroundColor: "transparent", border: "none" }}
      >
        <div className="flex items-center gap-2">
          <StatusDot />
          <span
            className="font-medium"
            style={{ color: "var(--color-tool)" }}
          >
            {toolName}
          </span>
          {argsPreview && (
            <span
              className="text-xs truncate flex-1"
              style={{ color: "var(--color-text-muted)", fontFamily: "var(--font-mono)" }}
            >
              {argsPreview.length > 60 ? argsPreview.slice(0, 60) + "…" : argsPreview}
            </span>
          )}
          <span
            className="text-xs"
            style={{ color: "var(--color-text-muted)" }}
          >
            {expanded ? "▲" : "▼"}
          </span>
        </div>

        {!expanded && resultPreview && (
          <div
            className="mt-1 pl-4 text-xs truncate"
            style={{ color: isFailed ? "var(--color-error)" : "var(--color-text-muted)" }}
          >
            {resultPreview.length > 80 ? resultPreview.slice(0, 80) + "…" : resultPreview}
          </div>
        )}
      </button>

      {/* Expanded: full details */}
      {expanded && (
        <div id={detailsId} className="mt-2 space-y-2 pl-4">
          {argsPreview && (
            <div>
              <div className="text-xs font-medium mb-0.5" style={{ color: "var(--color-text-muted)" }}>
                Arguments
              </div>
              <pre
                className="text-xs rounded px-2 py-1"
                style={{
                  backgroundColor: "var(--color-surface-2)",
                  border: "1px solid var(--color-border)",
                  color: "var(--color-text)",
                  fontFamily: "var(--font-mono)",
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-all",
                }}
              >
                {argsPreview}
              </pre>
            </div>
          )}
          {resultPreview && (
            <div>
              <div className="text-xs font-medium mb-0.5" style={{ color: "var(--color-text-muted)" }}>
                Result
              </div>
              <pre
                className="text-xs rounded px-2 py-1"
                style={{
                  backgroundColor: isFailed ? "rgba(220,38,38,0.05)" : "var(--color-surface-2)",
                  border: `1px solid ${isFailed ? "var(--color-error)" : "var(--color-border)"}`,
                  color: isFailed ? "var(--color-error)" : "var(--color-text)",
                  fontFamily: "var(--font-mono)",
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-all",
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
