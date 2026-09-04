import { useEffect, useState, type FormEvent, type ReactNode } from "react"
import { BaseModal } from "./BaseModal"

export interface FormModalSelectOption {
  value: string
  label: string
  /** Shown below the control when this option is selected. */
  description?: string
}

export interface FormModalFieldConfig {
  key: string
  label: string
  type: "text" | "textarea" | "select"
  placeholder?: string
  optional?: boolean
  maxLength?: number
  rows?: number
  /** Required when type is "select". */
  options?: FormModalSelectOption[]
  /**
   * Id of the group this field belongs to. Fields without a group render at the
   * top of the form; grouped fields render inside their group's section in the
   * order the groups are declared. Ignored unless the modal is given `groups`.
   */
  group?: string
}

export interface FormModalGroup {
  id: string
  /** Section heading. Omit to render the group's fields without a header. */
  title?: string
  /** Short explanatory line shown at the top of the group's body. */
  description?: string
  /** When true the group shows a toggle and can be collapsed. */
  collapsible?: boolean
  /** Initial open state for a collapsible group. Defaults to closed. */
  defaultOpen?: boolean
  /**
   * Extra content rendered inside the group, below its fields — for sub-forms a
   * plain field cannot express (a secret editor, a preview). Keep required
   * inputs out of a collapsed group: submit stays disabled while a required
   * field is empty, and a collapsed group hides the reason.
   */
  content?: ReactNode
}

export interface FormModalProps {
  open: boolean
  title: string
  titleId: string
  fields: FormModalFieldConfig[]
  /**
   * Optional section grouping. When present, fields are placed into the group
   * named by their `group`; ungrouped fields render first. Without it the form
   * is one flat stack, as before.
   */
  groups?: FormModalGroup[]
  hint?: string
  initialValues?: Record<string, string>
  dangerAction?: { label: string; onClick: () => void; disabled?: boolean }
  className?: string
  loading?: boolean
  error?: string | null
  submitLabel: string
  cancelLabel?: string
  onClose: () => void
  onSubmit: (values: Record<string, string>) => void
  // children render below the fields and hint, for content a form alone cannot
  // express — a revision history, a preview, a related list.
  children?: ReactNode
}

export function FormModal({
  open,
  title,
  titleId,
  fields,
  groups,
  hint,
  initialValues,
  dangerAction,
  className,
  loading = false,
  error,
  submitLabel,
  cancelLabel = "Cancel",
  onClose,
  onSubmit,
  children,
}: FormModalProps) {
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(fields.map((field) => [field.key, ""]))
  )
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({})

  useEffect(() => {
    if (!open) return

    if (initialValues != null) {
      setValues(Object.fromEntries(fields.map((field) => [field.key, initialValues[field.key] ?? ""])))
      return
    }

    setValues(Object.fromEntries(fields.map((field) => [field.key, ""])))
  }, [open, fields, initialValues])

  // Reset collapsible groups to their default each time the modal opens. Keyed
  // on a primitive signature so a freshly-built `groups` array (its `content` is
  // JSX, new every render) does not retrigger this and clobber the user's
  // toggles while the modal is open.
  const groupsKey = (groups ?? [])
    .map((g) => `${g.id}:${g.collapsible ? 1 : 0}:${g.defaultOpen ? 1 : 0}`)
    .join("|")
  useEffect(() => {
    if (!open) return
    const next: Record<string, boolean> = {}
    for (const g of groups ?? []) {
      if (g.collapsible) next[g.id] = g.defaultOpen ?? false
    }
    setOpenGroups(next)
    // groups intentionally excluded; groupsKey captures the parts that matter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, groupsKey])

  const hasMissingRequiredField = fields.some((field) => !field.optional && !values[field.key]?.trim())

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (hasMissingRequiredField) return
    onSubmit(values)
  }

  function renderField(field: FormModalFieldConfig) {
    return (
      <div key={field.key}>
        <label className="modal__label" htmlFor={field.key}>
          {field.label}
          {field.optional ? <span className="modal__optional"> (optional)</span> : null}
        </label>
        {field.type === "textarea" ? (
          <textarea
            id={field.key}
            className="modal__textarea"
            placeholder={field.placeholder}
            value={values[field.key] ?? ""}
            onChange={(e) => setValues((prev) => ({ ...prev, [field.key]: e.target.value }))}
            disabled={loading}
            rows={field.rows ?? 3}
            maxLength={field.maxLength}
          />
        ) : field.type === "select" ? (
          <>
            <select
              id={field.key}
              className="modal__input"
              value={values[field.key] ?? ""}
              onChange={(e) => setValues((prev) => ({ ...prev, [field.key]: e.target.value }))}
              disabled={loading}
            >
              {(field.options ?? []).map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            {field.options?.find((option) => option.value === (values[field.key] ?? ""))?.description ? (
              <p className="modal__hint">
                {field.options.find((option) => option.value === (values[field.key] ?? ""))?.description}
              </p>
            ) : null}
          </>
        ) : (
          <input
            id={field.key}
            type="text"
            className="modal__input"
            placeholder={field.placeholder}
            value={values[field.key] ?? ""}
            onChange={(e) => setValues((prev) => ({ ...prev, [field.key]: e.target.value }))}
            disabled={loading}
            autoComplete="off"
            maxLength={field.maxLength}
          />
        )}
      </div>
    )
  }

  const ungroupedFields = groups ? fields.filter((field) => !field.group) : fields

  return (
    <BaseModal open={open} title={title} titleId={titleId} onClose={onClose} className={className}>
      <form onSubmit={handleSubmit} className="modal__body">
        {ungroupedFields.map(renderField)}
        {(groups ?? []).map((group) => {
          const groupFields = fields.filter((field) => field.group === group.id)
          const isOpen = !group.collapsible || (openGroups[group.id] ?? group.defaultOpen ?? false)
          return (
            <section key={group.id} className="modal__group">
              {group.title ? (
                group.collapsible ? (
                  <button
                    type="button"
                    className="modal__group-header"
                    aria-expanded={isOpen}
                    onClick={() =>
                      setOpenGroups((prev) => ({
                        ...prev,
                        [group.id]: !(prev[group.id] ?? group.defaultOpen ?? false),
                      }))
                    }
                  >
                    <span className="modal__group-title">{group.title}</span>
                    <span className="modal__group-chevron" aria-hidden>
                      {isOpen ? "▾" : "▸"}
                    </span>
                  </button>
                ) : (
                  <div className="modal__group-header modal__group-header--static">
                    <span className="modal__group-title">{group.title}</span>
                  </div>
                )
              ) : null}
              {isOpen ? (
                <div className="modal__group-body">
                  {group.description ? <p className="modal__hint">{group.description}</p> : null}
                  {groupFields.map(renderField)}
                  {group.content}
                </div>
              ) : null}
            </section>
          )
        })}
        {hint ? <p className="modal__hint">{hint}</p> : null}
        {children}
        {error ? (
          <p className="modal__error" role="alert">
            {error}
          </p>
        ) : null}
        <div className="modal__actions">
          {dangerAction ? (
            <button
              type="button"
              className="modal__btn modal__btn--danger"
              onClick={dangerAction.onClick}
              disabled={loading || dangerAction.disabled}
            >
              {dangerAction.disabled ? `${dangerAction.label}…` : dangerAction.label}
            </button>
          ) : null}
          <button
            type="button"
            className="modal__btn modal__btn--secondary"
            onClick={onClose}
            disabled={loading}
          >
            {cancelLabel}
          </button>
          <button
            type="submit"
            className="modal__btn modal__btn--secondary"
            disabled={loading || hasMissingRequiredField}
          >
            {loading ? `${submitLabel}…` : submitLabel}
          </button>
        </div>
      </form>
    </BaseModal>
  )
}
