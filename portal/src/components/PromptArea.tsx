interface PromptAreaProps {
  value: string
  onChange: (v: string) => void
  onRun: () => void
  /** Optional heading; default "What would you like to accomplish?" */
  heading?: string
  /** Optional placeholder for the input */
  placeholder?: string
  /** Optional aria-label for the input */
  ariaLabel?: string
}

const DEFAULT_HEADING = "What would you like to accomplish?"
const DEFAULT_PLACEHOLDER = "Help me prepare this month's sales analysis"
const DEFAULT_ARIA_LABEL = "Intent or goal"

export function PromptArea({
  value,
  onChange,
  onRun,
  heading = DEFAULT_HEADING,
  placeholder = DEFAULT_PLACEHOLDER,
  ariaLabel = DEFAULT_ARIA_LABEL,
}: PromptAreaProps) {
  return (
    <section className="prompt-area">
      <h2 className="prompt-area__heading">{heading}</h2>
      <input
        type="text"
        className="prompt-area__input"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label={ariaLabel}
      />
      <button type="button" className="prompt-area__button" onClick={onRun}>
        Run
      </button>
    </section>
  )
}
