import type { KeyboardEvent } from "react"

export interface ChatComposerProps {
  value: string
  onChange: (value: string) => void
  onSubmit: () => void
  disabled?: boolean
  loading?: boolean
  error?: string | null
  placeholder?: string
  ariaLabel?: string
  submitLabel?: string
  loadingLabel?: string
  rows?: number
  allowShiftEnter?: boolean
}

const DEFAULT_PLACEHOLDER = "Type a message… (Enter to send, Shift+Enter for new line)"

export function ChatComposer({
  value,
  onChange,
  onSubmit,
  disabled = false,
  loading = false,
  error,
  placeholder = DEFAULT_PLACEHOLDER,
  ariaLabel = "Message",
  submitLabel = "Send",
  loadingLabel = "Sending…",
  rows = 2,
  allowShiftEnter = true,
}: ChatComposerProps) {
  const isSubmitDisabled = disabled || loading || !value.trim()

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
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
        <button
          type="button"
          className="bm-chat-composer__button"
          onClick={onSubmit}
          disabled={isSubmitDisabled}
        >
          {loading ? loadingLabel : submitLabel}
        </button>
      </div>
      {error ? (
        <p className="bm-chat-composer__error" role="alert">
          {error}
        </p>
      ) : null}
    </>
  )
}
