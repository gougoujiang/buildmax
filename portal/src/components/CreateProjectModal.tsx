import { useState, useEffect } from "react"
import { BaseModal } from "./BaseModal"

interface CreateProjectModalProps {
  open: boolean
  loading: boolean
  error: string | null
  onClose: () => void
  onCreate: (name: string, description: string) => void
}

export function CreateProjectModal({
  open,
  loading,
  error,
  onClose,
  onCreate,
}: CreateProjectModalProps) {
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")

  useEffect(() => {
    if (open) {
      setName("")
      setDescription("")
    }
  }, [open])

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (name.trim()) onCreate(name.trim(), description.trim())
  }

  return (
    <BaseModal
      open={open}
      title="New Project"
      titleId="create-proj-title"
      onClose={onClose}
    >
      <form onSubmit={handleSubmit} className="modal__body">
        <label className="modal__label" htmlFor="proj-name">
          Project name
        </label>
        <input
          id="proj-name"
          type="text"
          className="modal__input"
          placeholder="e.g. Q1 Report, Website Redesign, Data Analysis"
          value={name}
          onChange={(e) => setName(e.target.value)}
          disabled={loading}
          autoComplete="off"
          maxLength={100}
        />

        <label className="modal__label" htmlFor="proj-desc">
          Description <span className="modal__optional">(optional)</span>
        </label>
        <textarea
          id="proj-desc"
          className="modal__textarea"
          placeholder="What is this project about?"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          disabled={loading}
          rows={3}
          maxLength={500}
        />

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
            {loading ? "Creating…" : "Create project"}
          </button>
        </div>
      </form>
    </BaseModal>
  )
}
