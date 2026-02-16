import { useState, useRef, useEffect } from "react"

interface CreateWorkspaceModalProps {
  open: boolean
  loading: boolean
  error: string | null
  onClose: () => void
  onCreate: (name: string) => void
}

export function CreateWorkspaceModal({
  open,
  loading,
  error,
  onClose,
  onCreate,
}: CreateWorkspaceModalProps) {
  const [name, setName] = useState("")
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setName("")
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose()
    }
    window.addEventListener("keydown", handleKey)
    return () => window.removeEventListener("keydown", handleKey)
  }, [open, onClose])

  if (!open) return null

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (name.trim()) onCreate(name.trim())
  }

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-ws-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal__header">
          <h2 className="modal__title" id="create-ws-title">
            New Workspace
          </h2>
          <button
            type="button"
            className="modal__close"
            onClick={onClose}
            aria-label="Close"
          >
            &times;
          </button>
        </div>

        <form onSubmit={handleSubmit} className="modal__body">
          <label className="modal__label" htmlFor="ws-name">
            Workspace name
          </label>
          <input
            ref={inputRef}
            id="ws-name"
            type="text"
            className="modal__input"
            placeholder="e.g. Marketing, Engineering, Personal"
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={loading}
            autoComplete="off"
            maxLength={100}
          />
          <p className="modal__hint">
            Give your workspace a name that describes its purpose. You can always create more later.
          </p>

          {error && (
            <p className="modal__error" role="alert">
              {error}
            </p>
          )}

          <div className="modal__actions">
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
              disabled={loading || !name.trim()}
            >
              {loading ? "Creating…" : "Create workspace"}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
