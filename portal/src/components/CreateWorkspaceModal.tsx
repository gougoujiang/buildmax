import { useState, useEffect } from "react"
import { BaseModal } from "./BaseModal"

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

  useEffect(() => {
    if (open) setName("")
  }, [open])

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (name.trim()) onCreate(name.trim())
  }

  return (
    <BaseModal
      open={open}
      title="New Workspace"
      titleId="create-ws-title"
      onClose={onClose}
    >
      <form onSubmit={handleSubmit} className="modal__body">
        <label className="modal__label" htmlFor="ws-name">
          Workspace name
        </label>
        <input
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
    </BaseModal>
  )
}
