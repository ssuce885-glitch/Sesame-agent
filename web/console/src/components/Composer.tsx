import { useState, useRef, useCallback } from "react";
import { useI18n } from "../i18n";

interface ComposerProps {
  onSend: (message: string) => void;
  disabled?: boolean;
}

export function Composer({ onSend, disabled }: ComposerProps) {
  const { t } = useI18n();
  const [value, setValue] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleSubmit = useCallback(() => {
    const trimmed = value.trim();
    if (!trimmed || disabled) return;
    setValue("");
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
    onSend(trimmed);
  }, [value, disabled, onSend]);

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
        disabled={disabled}
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
        disabled={disabled || !value.trim()}
        className="w-full rounded-lg px-4 py-2 text-sm font-medium sm:w-auto"
        style={{
          backgroundColor: disabled ? "var(--color-border)" : "var(--color-accent)",
          color: "#fff",
          border: "none",
          cursor: disabled ? "not-allowed" : "pointer",
          opacity: disabled ? 0.5 : 1,
        }}
      >
        {disabled ? t("composer.sending") : t("composer.send")}
      </button>
    </div>
  );
}
