import { useState, useRef, useCallback } from "react";
import { useI18n } from "../i18n";

interface ComposerProps {
  onSend: (message: string) => void | Promise<void>;
  disabled?: boolean;
}

export function Composer({ onSend, disabled }: ComposerProps) {
  const { t } = useI18n();
  const [value, setValue] = useState("");
  const [sending, setSending] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const isDisabled = disabled || sending;
  const canSend = value.trim() !== "" && !isDisabled;

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed || isDisabled) return;
    setSending(true);
    try {
      await onSend(trimmed);
      setValue("");
      if (textareaRef.current) {
        textareaRef.current.style.height = "auto";
      }
    } catch {
      textareaRef.current?.focus();
    } finally {
      setSending(false);
    }
  }, [value, isDisabled, onSend]);

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  }

  function handleChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    setValue(e.target.value);
    // Auto-resize
    const ta = e.target;
    ta.style.height = "auto";
    ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`;
  }

  return (
    <div
      className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-end"
      style={{
        borderTop: "1px solid var(--color-border)",
        backgroundColor: "var(--color-surface)",
      }}
    >
      <textarea
        ref={textareaRef}
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        placeholder={t("composer.placeholder")}
        disabled={isDisabled}
        rows={1}
        className="flex-1 resize-none rounded-lg px-4 py-3 text-sm"
        style={{
          backgroundColor: "var(--color-surface-2)",
          border: "1px solid var(--color-border)",
          color: "var(--color-text)",
          outline: "none",
          fontFamily: "var(--font-sans)",
          lineHeight: 1.5,
          maxHeight: 200,
          overflowY: "auto",
        }}
      />
      <button
        onClick={handleSubmit}
        disabled={!canSend}
        className="w-full rounded-lg px-4 py-2 text-sm font-medium sm:w-auto"
        style={{
          backgroundColor: canSend ? "var(--color-accent)" : "var(--color-border)",
          color: "#fff",
          border: "none",
          cursor: canSend ? "pointer" : "not-allowed",
          opacity: canSend ? 1 : 0.5,
          transition: "opacity 0.15s, filter 0.15s",
        }}
        onMouseEnter={(e) => { if (canSend) e.currentTarget.style.filter = "brightness(0.9)"; }}
        onMouseLeave={(e) => { e.currentTarget.style.filter = "none"; }}
      >
        {isDisabled ? t("composer.sending") : t("composer.send")}
      </button>
    </div>
  );
}
