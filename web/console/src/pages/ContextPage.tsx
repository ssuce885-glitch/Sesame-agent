import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import {
  useContextPreview,
  useCreateMemory,
  useDeleteMemory,
  useMemories,
  useProjectState,
  useSetSetting,
  useSetting,
  useUpdateProjectState,
} from "../api/queries";
import type { ContextPreview, ContextPreviewBlock, Memory } from "../api/types";
import { RefreshCw, Save, X } from "../components/Icon";
import { useI18n } from "../i18n";

interface ContextPageProps {
  workspaceRoot: string | null;
  sessionId: string | null;
}

type MemoryKind = "note" | "fact" | "decision" | "preference" | "pattern";
type Tone = "neutral" | "success" | "warning" | "error";

export function ContextPage({ workspaceRoot, sessionId }: ContextPageProps) {
  const { t } = useI18n();
  const projectState = useProjectState(workspaceRoot);
  const updateProjectState = useUpdateProjectState(workspaceRoot);
  const autoSetting = useSetting("project_state_auto");
  const setAutoSetting = useSetSetting("project_state_auto");
  const contextPreview = useContextPreview(sessionId);
  const [summary, setSummary] = useState("");

  useEffect(() => {
    setSummary(projectState.data?.summary ?? "");
  }, [projectState.data?.summary]);

  return (
    <section className="h-full overflow-y-auto p-5" style={{ backgroundColor: "var(--color-bg)" }}>
      <div className="mx-auto flex max-w-6xl flex-col gap-5">
        <header>
          <h1 className="m-0 text-2xl font-semibold" style={{ color: "var(--color-text)" }}>
            {t("context.title")}
          </h1>
          <p className="m-0 mt-1 text-sm" style={{ color: "var(--color-text-tertiary)" }}>
            {t("context.subtitle")}
          </p>
        </header>

        {!workspaceRoot ? (
          <StateBox tone="neutral" text={t("context.noWorkspace")} />
        ) : (
          <>
            <ContextPreviewPanel
              preview={contextPreview.data}
              isLoading={contextPreview.isLoading}
              isError={contextPreview.isError}
              sessionId={sessionId}
              onRefresh={() => void contextPreview.refetch()}
            />

            <section className="rounded-md p-4" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
              <div className="mb-3 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                <div>
                  <h2 className="m-0 text-sm font-semibold" style={{ color: "var(--color-text)" }}>
                    {t("context.projectState")}
                  </h2>
                  <p className="m-0 mt-1 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
                    {projectState.data?.updated_at ? t("context.updated", { time: formatDate(projectState.data.updated_at) }) : t("context.notSaved")}
                  </p>
                </div>
                <label className="flex items-center gap-2 text-xs" style={{ color: "var(--color-text-secondary)" }}>
                  <input
                    type="checkbox"
                    checked={(autoSetting.data?.value ?? "true") === "true"}
                    onChange={(event) => setAutoSetting.mutate(event.target.checked ? "true" : "false")}
                  />
                  {t("context.autoUpdate")}
                </label>
              </div>
              <textarea
                aria-label={t("context.projectState")}
                value={summary}
                onChange={(event) => setSummary(event.target.value)}
                rows={10}
                className="w-full resize-y rounded-md p-3 text-sm leading-6"
                style={{ ...inputStyle, fontFamily: "var(--font-mono)" }}
              />
              {updateProjectState.error ? (
                <div className="mt-3 rounded-md p-3 text-sm" style={{ backgroundColor: "var(--color-error-dim)", color: "var(--color-error)", border: "1px solid rgba(239,68,68,0.2)" }}>
                  {updateProjectState.error instanceof Error ? updateProjectState.error.message : String(updateProjectState.error)}
                </div>
              ) : null}
              <div className="mt-3 flex justify-end">
                <button
                  type="button"
                  disabled={updateProjectState.isPending}
                  onClick={() =>
                    updateProjectState.mutate({
                      workspace_root: workspaceRoot,
                      summary,
                      source_session_id: sessionId ?? "",
                    })
                  }
                  className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
                  style={{
                    border: "1px solid var(--color-accent)",
                    backgroundColor: "var(--color-accent)",
                    color: "white",
                    cursor: updateProjectState.isPending ? "default" : "pointer",
                    opacity: updateProjectState.isPending ? 0.7 : 1,
                  }}
                >
                  <Save size={14} />
                  {updateProjectState.isPending ? t("context.saving") : t("context.saveProjectState")}
                </button>
              </div>
            </section>

            <MemoryPanel workspaceRoot={workspaceRoot} />
          </>
        )}
      </div>
    </section>
  );
}

function ContextPreviewPanel({
  preview,
  isLoading,
  isError,
  sessionId,
  onRefresh,
}: {
  preview?: ContextPreview;
  isLoading: boolean;
  isError: boolean;
  sessionId: string | null;
  onRefresh: () => void;
}) {
  const { t } = useI18n();
  const included = preview?.blocks.filter((block) => block.status === "included").length ?? 0;
  const available = preview?.blocks.filter((block) => block.status === "available").length ?? 0;
  const visibleBlocks = preview?.blocks.slice(0, 12) ?? [];

  return (
    <section className="rounded-md p-4" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="mb-4 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <h2 className="m-0 text-sm font-semibold" style={{ color: "var(--color-text)" }}>
            {t("context.inspector")}
          </h2>
          <p className="m-0 mt-1 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
            {preview?.generated_at ? t("context.generated", { time: formatDate(preview.generated_at) }) : t("context.inspectorSubtitle")}
          </p>
        </div>
        <button
          type="button"
          onClick={onRefresh}
          disabled={!sessionId || isLoading}
          className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
          style={{
            border: "1px solid var(--color-border)",
            backgroundColor: "var(--color-surface)",
            color: "var(--color-text)",
            cursor: !sessionId || isLoading ? "not-allowed" : "pointer",
            opacity: !sessionId || isLoading ? 0.65 : 1,
          }}
        >
          <RefreshCw size={14} />
          {t("context.refresh")}
        </button>
      </div>

      {!sessionId ? (
        <StateBox tone="neutral" text={t("context.noSession")} />
      ) : isLoading ? (
        <LoadingRows />
      ) : isError ? (
        <StateBox tone="error" text={t("context.previewLoadFailed")} />
      ) : preview ? (
        <div className="grid gap-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <Metric label={t("context.promptTokens")} value={String(preview.approx_tokens)} />
            <Metric label={t("context.includedBlocks")} value={String(included)} />
            <Metric label={t("context.availableBlocks")} value={String(available)} />
          </div>

          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(320px,0.8fr)]">
            <div className="min-w-0">
              <h3 className="m-0 mb-2 text-xs font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
                {t("context.promptPreview")}
              </h3>
              {preview.prompt.length ? (
                <div className="flex max-h-[360px] flex-col gap-2 overflow-y-auto pr-1">
                  {preview.prompt.map((item, index) => (
                    <article key={`${item.source_ref}-${index}`} className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg)", border: "1px solid var(--color-border)" }}>
                      <div className="mb-2 flex flex-wrap items-center gap-2">
                        <Badge value={item.role} tone="neutral" />
                        <span className="break-all text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
                          {item.source_ref}
                        </span>
                        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
                          {t("context.tokens", { count: item.approx_tokens })}
                        </span>
                      </div>
                      <p className="m-0 whitespace-pre-wrap text-xs leading-5" style={{ color: "var(--color-text-secondary)" }}>
                        {item.content_preview}
                      </p>
                    </article>
                  ))}
                </div>
              ) : (
                <StateBox tone="neutral" text={t("context.noPromptPreview")} />
              )}
            </div>

            <div className="min-w-0">
              <h3 className="m-0 mb-2 text-xs font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
                {t("context.contextBlocks")}
              </h3>
              {visibleBlocks.length ? (
                <div className="flex max-h-[360px] flex-col gap-2 overflow-y-auto pr-1">
                  {visibleBlocks.map((block) => (
                    <ContextBlockRow key={`${block.id}-${block.source_ref}`} block={block} />
                  ))}
                </div>
              ) : (
                <StateBox tone="neutral" text={t("context.noContextBlocks")} />
              )}
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md px-3 py-2" style={{ backgroundColor: "var(--color-bg)", border: "1px solid var(--color-border)" }}>
      <div className="text-[11px] font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </div>
      <div className="mt-1 text-lg font-semibold" style={{ color: "var(--color-text)" }}>
        {value}
      </div>
    </div>
  );
}

function ContextBlockRow({ block }: { block: ContextPreviewBlock }) {
  const tone = block.status === "included" ? "success" : block.status === "available" ? "warning" : "neutral";
  return (
    <article className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg)", border: "1px solid var(--color-border)" }}>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Badge value={block.status} tone={tone} />
        <Badge value={block.type} tone="neutral" />
        <span className="break-all text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {block.source_ref}
        </span>
      </div>
      <p className="m-0 text-sm font-medium" style={{ color: "var(--color-text)" }}>
        {block.title || block.id}
      </p>
      {block.summary ? (
        <p className="m-0 mt-1 whitespace-pre-wrap text-xs leading-5" style={{ color: "var(--color-text-secondary)" }}>
          {block.summary}
        </p>
      ) : null}
      {block.reason ? (
        <p className="m-0 mt-2 text-[11px] leading-4" style={{ color: "var(--color-text-tertiary)" }}>
          {block.reason}
        </p>
      ) : null}
    </article>
  );
}

function MemoryPanel({ workspaceRoot }: { workspaceRoot: string }) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const memories = useMemories(workspaceRoot, query);
  const createMemory = useCreateMemory(workspaceRoot, query);
  const deleteMemory = useDeleteMemory(workspaceRoot, query);
  const [kind, setKind] = useState<MemoryKind>("note");
  const [content, setContent] = useState("");
  const [source, setSource] = useState("");
  const [confidence, setConfidence] = useState(1);

  function submitMemory() {
    if (!content.trim()) {
      return;
    }
    createMemory.mutate(
      {
        workspace_root: workspaceRoot,
        kind,
        content: content.trim(),
        source: source.trim(),
        confidence,
      },
      {
        onSuccess: () => {
          setContent("");
          setSource("");
          setConfidence(1);
        },
      },
    );
  }

  return (
    <section className="rounded-md p-4" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="mb-4 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <h2 className="m-0 text-sm font-semibold" style={{ color: "var(--color-text)" }}>
            {t("context.memory")}
          </h2>
          <p className="m-0 mt-1 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
            {t("context.memorySubtitle")}
          </p>
        </div>
        <button
          type="button"
          onClick={() => void memories.refetch()}
          className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
          style={{ border: "1px solid var(--color-border)", backgroundColor: "var(--color-surface)", color: "var(--color-text)", cursor: "pointer" }}
        >
          <RefreshCw size={14} />
          {t("context.refresh")}
        </button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
        <div className="flex min-w-0 flex-col gap-3">
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("context.searchPlaceholder")}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
          {memories.isLoading ? (
            <LoadingRows />
          ) : memories.isError ? (
            <StateBox tone="error" text={t("context.memoryLoadFailed")} />
          ) : memories.data?.length ? (
            <div className="flex flex-col gap-2">
              {memories.data.map((memory) => (
                <MemoryRow key={memory.id} memory={memory} isDeleting={deleteMemory.isPending} onDelete={() => deleteMemory.mutate(memory.id)} />
              ))}
            </div>
          ) : (
            <StateBox tone="neutral" text={t("context.noMemories")} />
          )}
        </div>

        <div className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg)", border: "1px solid var(--color-border)" }}>
          <h3 className="m-0 mb-3 text-xs font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
            {t("context.addMemory")}
          </h3>
          <div className="grid gap-3">
            <FormField label={t("context.kind")}>
              <select value={kind} onChange={(event) => setKind(event.target.value as MemoryKind)} className="h-10 w-full rounded-md px-3 text-sm" style={inputStyle}>
                {(["note", "fact", "decision", "preference", "pattern"] as MemoryKind[]).map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </FormField>
            <FormField label={t("context.content")}>
              <textarea value={content} onChange={(event) => setContent(event.target.value)} rows={6} className="w-full resize-y rounded-md p-3 text-sm leading-6" style={inputStyle} />
            </FormField>
            <FormField label={t("context.source")}>
              <input value={source} onChange={(event) => setSource(event.target.value)} className="h-10 w-full rounded-md px-3 text-sm" style={inputStyle} />
            </FormField>
            <FormField label={t("context.confidence", { value: confidence.toFixed(2) })}>
              <input type="range" min={0.1} max={1} step={0.05} value={confidence} onChange={(event) => setConfidence(Number(event.target.value))} />
            </FormField>
            {createMemory.error ? (
              <StateBox tone="error" text={createMemory.error instanceof Error ? createMemory.error.message : String(createMemory.error)} />
            ) : null}
            <button
              type="button"
              disabled={createMemory.isPending || !content.trim()}
              onClick={submitMemory}
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
              style={{
                border: "1px solid var(--color-accent)",
                backgroundColor: "var(--color-accent)",
                color: "white",
                cursor: createMemory.isPending || !content.trim() ? "not-allowed" : "pointer",
                opacity: createMemory.isPending || !content.trim() ? 0.65 : 1,
              }}
            >
              <Save size={14} />
              {createMemory.isPending ? t("context.saving") : t("context.saveMemory")}
            </button>
          </div>
        </div>
      </div>
    </section>
  );
}

function MemoryRow({ memory, isDeleting, onDelete }: { memory: Memory; isDeleting: boolean; onDelete: () => void }) {
  const { t } = useI18n();
  return (
    <article className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg)", border: "1px solid var(--color-border)" }}>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Badge value={memory.kind} tone="neutral" />
        <Badge value={memory.confidence.toFixed(2)} tone={memory.confidence >= 0.8 ? "success" : "warning"} />
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {formatDate(memory.updated_at)}
        </span>
      </div>
      <p className="m-0 whitespace-pre-wrap text-sm leading-6" style={{ color: "var(--color-text-secondary)" }}>
        {memory.content}
      </p>
      <div className="mt-3 flex items-center justify-between gap-3">
        <span className="min-w-0 break-all text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {memory.source || memory.id}
        </span>
        <button
          type="button"
          disabled={isDeleting}
          onClick={onDelete}
          className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium"
          style={{ border: "1px solid rgba(239,68,68,0.35)", backgroundColor: "var(--color-error-dim)", color: "var(--color-error)", cursor: isDeleting ? "not-allowed" : "pointer" }}
        >
          <X size={12} />
          {t("context.delete")}
        </button>
      </div>
    </article>
  );
}

function FormField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-xs font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </span>
      {children}
    </label>
  );
}

function Badge({ value, tone }: { value: string; tone: Tone }) {
  const styles = {
    neutral: ["var(--color-surface)", "var(--color-text-tertiary)"],
    success: ["var(--color-success-dim)", "var(--color-success)"],
    warning: ["var(--color-warning-dim)", "var(--color-warning)"],
    error: ["var(--color-error-dim)", "var(--color-error)"],
  }[tone];
  return (
    <span className="inline-flex rounded px-1.5 py-0.5 text-[11px] font-medium" style={{ backgroundColor: styles[0], color: styles[1] }}>
      {value || "-"}
    </span>
  );
}

function StateBox({ text, tone }: { text: string; tone: "neutral" | "error" }) {
  return (
    <div
      className="rounded-md p-4 text-sm"
      style={{
        backgroundColor: tone === "error" ? "var(--color-error-dim)" : "var(--color-bg)",
        border: tone === "error" ? "1px solid rgba(239,68,68,0.2)" : "1px solid var(--color-border)",
        color: tone === "error" ? "var(--color-error)" : "var(--color-text-tertiary)",
      }}
    >
      {text}
    </div>
  );
}

function LoadingRows() {
  return (
    <div className="space-y-2">
      {[0, 1, 2].map((item) => (
        <div key={item} className="animate-shimmer rounded-md" style={{ height: 92, backgroundColor: "var(--color-surface)" }} />
      ))}
    </div>
  );
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

const inputStyle = {
  backgroundColor: "var(--color-bg)",
  border: "1px solid var(--color-border)",
  color: "var(--color-text)",
} as const;
