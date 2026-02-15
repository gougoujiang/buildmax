interface PromptAreaProps {
  value: string
  onChange: (v: string) => void
  onRun: () => void
}

export function PromptArea({ value, onChange, onRun }: PromptAreaProps) {
  return (
    <section className="prompt-area">
      <h2 className="prompt-area__heading">What would you like to accomplish?</h2>
      <input
        type="text"
        className="prompt-area__input"
        placeholder="Help me prepare this month's sales analysis"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label="Intent or goal"
      />
      <button type="button" className="prompt-area__button" onClick={onRun}>
        Run
      </button>
    </section>
  )
}
