import type { KeyboardEvent } from "react"

export interface ChatComposerProps {
  value: string
  onChange: (value: string) => void
  onSubmit: () => void
  onKeyDown?: (e: KeyboardEvent<HTMLTextAreaElement>) => void
  disabled?: boolean
  loading?: boolean
  error?: string | null
  placeholder?: string
  ariaLabel?: string
  submitLabel?: string
  loadingLabel?: string
  rows?: number
  allowShiftEnter?: boolean
  // onCancel, when set, replaces the loading-state Send button with an active
  // Stop button. Callers wire this to a cooperative cancellation of the in-flight
  // run. When omitted, loading-state behavior is unchanged.
  onCancel?: () => void
  cancelLabel?: string
}

const DEFAULT_PLACEHOLDER = "Type a message… (Enter to send, Shift+Enter for new line)"

export function ChatComposer({
  value,
  onChange,
  onSubmit,
  onKeyDown: onKeyDownProp,
  disabled = false,
  loading = false,
  error,
  placeholder = DEFAULT_PLACEHOLDER,
  ariaLabel = "Message",
  submitLabel = "Send",
  loadingLabel = "Sending…",
  rows = 2,
  allowShiftEnter = true,
  onCancel,
  cancelLabel = "Stop",
}: ChatComposerProps) {
  const showCancel = loading && !!onCancel
  const isSubmitDisabled = disabled || loading || !value.trim()

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    // Let the parent intercept first (e.g. for slash command navigation).
    if (onKeyDownProp) {
      onKeyDownProp(e)
      if (e.defaultPrevented) return
    }
    if (e.key !== "Enter") return
    if (allowShiftEnter && e.shiftKey) return

    e.preventDefault()
    if (!isSubmitDisabled) onSubmit()
  }

  return (
    <>
      <div className="bm-chat-composer">
        <textarea
          className="bm-chat-composer__input"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          rows={rows}
          disabled={disabled || loading}
          aria-label={ariaLabel}
        />
        {showCancel ? (
          <button
            type="button"
            className="bm-chat-composer__button bm-chat-composer__button--cancel"
            onClick={onCancel}
          >
            {cancelLabel}
          </button>
        ) : (
          <button
            type="button"
            className="bm-chat-composer__button"
            onClick={onSubmit}
            disabled={isSubmitDisabled}
          >
            {loading ? loadingLabel : submitLabel}
          </button>
        )}
      </div>
      {error ? (
        <p className="bm-chat-composer__error" role="alert">
          {error}
        </p>
      ) : null}
    </>
  )
}
