import { useRef, useEffect, type ReactNode } from "react"

export interface BaseModalProps {
  open: boolean
  title: string
  titleId: string
  onClose: () => void
  /** Optional class name(s) applied to the modal container (e.g. modal--large). */
  className?: string
  /** When true, do not render the header (title + close). Caller renders close elsewhere. */
  hideHeader?: boolean
  children: ReactNode
}

export function BaseModal({
  open,
  title,
  titleId,
  onClose,
  className,
  hideHeader,
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
        className={className ? `modal ${className}` : "modal"}
        role="dialog"
        aria-modal="true"
        aria-labelledby={hideHeader ? undefined : titleId}
        aria-label={hideHeader ? title : undefined}
        onClick={(e) => e.stopPropagation()}
      >
        {!hideHeader && (
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
        )}
        {children}
      </div>
    </div>
  )
}
