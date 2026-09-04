/**
 * Editor for the catalog plugins an agent loads for its background runs. It is
 * a checklist, not a text field, so the agent can only name plugins the team
 * can actually use: `available` is the nameable set (see nameablePlugins).
 *
 * A plugin already on the agent but no longer nameable (an activation was
 * suspended, or the catalog changed) still shows, checked and flagged, so
 * editing something else never silently drops it. The server checks the final
 * selection again and is the authority.
 */
interface PluginSelectionEditorProps {
  value: string[]
  onChange: (value: string[]) => void
  available: string[]
}

export function PluginSelectionEditor({ value, onChange, available }: PluginSelectionEditorProps) {
  // Union of what the team offers and what the agent already names, sorted, so
  // a stale name stays visible instead of vanishing from the list.
  const names = [...new Set([...available, ...value])].sort()
  const availableSet = new Set(available)

  function toggle(name: string, checked: boolean) {
    if (checked) {
      if (value.includes(name)) return
      onChange([...value, name].sort())
    } else {
      onChange(value.filter((n) => n !== name))
    }
  }

  if (names.length === 0) {
    return (
      <p className="modal__hint">
        No plugins are available to this team yet. A team owner can activate plugins, or open the
        catalog, under Team settings.
      </p>
    )
  }

  return (
    <div className="agent-plugins">
      {names.map((name) => {
        const checked = value.includes(name)
        const stale = checked && !availableSet.has(name)
        return (
          <label key={name} className="agent-plugins__row">
            <input
              type="checkbox"
              checked={checked}
              onChange={(e) => toggle(name, e.target.checked)}
            />
            <span className="agent-plugins__name">{name}</span>
            {stale ? (
              <span className="agent-plugins__note">not available to this team</span>
            ) : null}
          </label>
        )
      })}
    </div>
  )
}
