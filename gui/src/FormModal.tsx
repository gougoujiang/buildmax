import { useEffect, useState, type FormEvent, type ReactNode } from "react"
import { BaseModal } from "./BaseModal"

export interface FormModalFieldConfig {
  key: string
  label: string
  type: "text" | "textarea"
  placeholder?: string
  optional?: boolean
  maxLength?: number
  rows?: number
}

export interface FormModalProps {
  open: boolean
  title: string
  titleId: string
  fields: FormModalFieldConfig[]
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

  useEffect(() => {
    if (!open) return

    if (initialValues != null) {
      setValues(Object.fromEntries(fields.map((field) => [field.key, initialValues[field.key] ?? ""])))
      return
    }

    setValues(Object.fromEntries(fields.map((field) => [field.key, ""])))
  }, [open, fields, initialValues])

  const hasMissingRequiredField = fields.some((field) => !field.optional && !values[field.key]?.trim())

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (hasMissingRequiredField) return
    onSubmit(values)
  }

  return (
    <BaseModal open={open} title={title} titleId={titleId} onClose={onClose} className={className}>
      <form onSubmit={handleSubmit} className="modal__body">
        {fields.map((field) => (
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
        ))}
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
