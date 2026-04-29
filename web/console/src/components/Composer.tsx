import { useState, useRef, useCallback } from "react";
import { useI18n } from "../i18n";
import { Terminal } from "./Icon";

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
    const ta = e.target;
    ta.style.height = "auto";
    ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`;
  }

  return (
    <div
      className="px-4 py-3"
      style={{
        borderTop: "1px solid var(--color-border)",
        backgroundColor: "var(--color-bg-elevated)",
      }}
    >
      <div
        className="flex items-end gap-2 rounded-lg px-3 py-2"
        style={{
          backgroundColor: "var(--color-surface)",
          border: "1px solid var(--color-border)",
          transition: "border-color 0.15s",
        }}
      >
        <Terminal size={16} color="var(--color-text-tertiary)" className="shrink-0 mb-2" />
        <textarea
          ref={textareaRef}
          value={value}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={t("composer.placeholder")}
          disabled={isDisabled}
          rows={1}
          className="flex-1 resize-none bg-transparent text-sm py-1.5 outline-none"
          style={{
            color: "var(--color-text)",
            fontFamily: "var(--font-sans)",
            lineHeight: 1.5,
            maxHeight: 200,
            overflowY: "auto",
          }}
        />
        <button
          onClick={handleSubmit}
          disabled={!canSend}
          className="shrink-0 rounded-md px-3 py-1.5 text-xs font-semibold"
          style={{
            backgroundColor: canSend ? "var(--color-accent)" : "transparent",
            color: canSend ? "#fff" : "var(--color-text-tertiary)",
            border: canSend ? "none" : "1px solid var(--color-border)",
            cursor: canSend ? "pointer" : "not-allowed",
            opacity: canSend ? 1 : 0.6,
            transition: "all 0.15s",
          }}
        >
          {isDisabled ? t("composer.sending") : t("composer.send")}
        </button>
      </div>
      <div className="flex items-center justify-between mt-1.5 px-1">
        <span className="text-[10px]" style={{ color: "var(--color-text-tertiary)" }}>
          Shift + Enter for new line
        </span>
        <span className="text-[10px]" style={{ color: "var(--color-text-tertiary)" }}>
          {value.length} chars
        </span>
      </div>
    </div>
  );
}
