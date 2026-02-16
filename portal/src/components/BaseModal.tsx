import { useRef, useEffect, type ReactNode } from "react"

interface BaseModalProps {
  open: boolean
  title: string
  titleId: string
  onClose: () => void
  children: ReactNode
}

export function BaseModal({
  open,
  title,
  titleId,
  onClose,
  children,
}: BaseModalProps) {
  const focusRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (open) {
      setTimeout(() => {
        const first = focusRef.current?.querySelector("input, textarea") as HTMLInputElement | HTMLTextAreaElement | null
        if (first) first.focus()
      }, 0)
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

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        ref={focusRef}
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal__header">
          <h2 className="modal__title" id={titleId}>
            {title}
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
        {children}
      </div>
    </div>
  )
}
