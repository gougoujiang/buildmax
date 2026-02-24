import { useState, useEffect } from "react"
import { BaseModal } from "./BaseModal"

export interface CreateEntityFieldConfig {
  key: string
  label: string
  type: "text" | "textarea"
  placeholder?: string
  optional?: boolean
  maxLength?: number
  rows?: number
}

export interface CreateEntityModalProps {
  open: boolean
  title: string
  titleId: string
  fields: CreateEntityFieldConfig[]
  hint?: string
  /** When provided and modal opens, form is prefilled with these values (key -> value per field). */
  initialValues?: Record<string, string>
  /** Optional danger action (e.g. Delete) shown as a button in the actions row. */
  dangerAction?: { label: string; onClick: () => void; disabled?: boolean }
  loading: boolean
  error: string | null
  submitLabel: string
  onClose: () => void
  onSubmit: (values: Record<string, string>) => void
}

export function CreateEntityModal({
  open,
  title,
  titleId,
  fields,
  hint,
  initialValues,
  dangerAction,
  loading,
  error,
  submitLabel,
  onClose,
  onSubmit,
}: CreateEntityModalProps) {
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(fields.map((f) => [f.key, ""]))
  )

  useEffect(() => {
    if (open) {
      if (initialValues != null) {
        setValues(
          Object.fromEntries(
            fields.map((f) => [f.key, initialValues[f.key] ?? ""])
          )
        )
      } else {
        setValues(Object.fromEntries(fields.map((f) => [f.key, ""])))
      }
    }
  }, [open, fields, initialValues])

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const firstRequired = fields.find((f) => !f.optional)
    if (firstRequired && !values[firstRequired.key]?.trim()) return
    onSubmit(values)
  }

  return (
    <BaseModal open={open} title={title} titleId={titleId} onClose={onClose}>
      <form onSubmit={handleSubmit} className="modal__body">
        {fields.map((field) => (
          <div key={field.key}>
            <label className="modal__label" htmlFor={field.key}>
              {field.label}
              {field.optional && <span className="modal__optional"> (optional)</span>}
            </label>
            {field.type === "textarea" ? (
              <textarea
                id={field.key}
                className="modal__textarea"
                placeholder={field.placeholder}
                value={values[field.key] ?? ""}
                onChange={(e) => setValues((v) => ({ ...v, [field.key]: e.target.value }))}
                disabled={loading}
                rows={field.rows ?? 3}
                maxLength={field.maxLength}
              />
            ) : (
              <input
                id={field.key}
                type="text"
                className="modal__input"
                placeholder={field.placeholder}
                value={values[field.key] ?? ""}
                onChange={(e) => setValues((v) => ({ ...v, [field.key]: e.target.value }))}
                disabled={loading}
                autoComplete="off"
                maxLength={field.maxLength}
              />
            )}
          </div>
        ))}
        {hint && <p className="modal__hint">{hint}</p>}
        {error && (
          <p className="modal__error" role="alert">
            {error}
          </p>
        )}
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
            Cancel
          </button>
          <button
            type="submit"
            className="modal__btn modal__btn--primary"
            disabled={loading || fields.some((f) => !f.optional && !values[f.key]?.trim())}
          >
            {loading ? `${submitLabel}…` : submitLabel}
          </button>
        </div>
      </form>
    </BaseModal>
  )
}
