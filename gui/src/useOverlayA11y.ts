import { useEffect, type RefObject } from "react"
import { lockBodyScroll, unlockBodyScroll } from "./bodyScrollLock"

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
}

export interface UseOverlayA11yOptions {
  open: boolean
  onClose: () => void
  containerRef: RefObject<HTMLElement | null>
}

/**
 * Shared dialog/drawer behavior: traps Tab focus inside the container, moves
 * focus in on open and restores it to the opener on close, locks body scroll
 * while open, and closes on Escape. Used by BaseModal and Drawer so both get
 * the same accessible overlay contract from one implementation.
 */
export function useOverlayA11y({ open, onClose, containerRef }: UseOverlayA11yOptions) {
  useEffect(() => {
    if (!open) return

    const opener = document.activeElement as HTMLElement | null
    lockBodyScroll()

    // A timer, not requestAnimationFrame: rAF is paused for a backgrounded or
    // not-yet-visible tab, which would leave focus management silently
    // inert — the container has just been rendered, so waiting a tick for it
    // to exist in the DOM is enough either way.
    const timer = setTimeout(() => {
      const container = containerRef.current
      if (!container) return
      const [first] = focusableElements(container)
      if (first) {
        first.focus()
      } else {
        container.focus()
      }
    }, 0)

    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        onClose()
        return
      }
      if (e.key !== "Tab") return
      const container = containerRef.current
      if (!container) return
      const focusable = focusableElements(container)
      if (focusable.length === 0) {
        e.preventDefault()
        return
      }
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const active = document.activeElement
      if (e.shiftKey) {
        if (active === first || !container.contains(active)) {
          e.preventDefault()
          last.focus()
        }
      } else {
        if (active === last || !container.contains(active)) {
          e.preventDefault()
          first.focus()
        }
      }
    }

    document.addEventListener("keydown", handleKey)

    return () => {
      clearTimeout(timer)
      document.removeEventListener("keydown", handleKey)
      unlockBodyScroll()
      opener?.focus?.()
    }
  }, [open, onClose, containerRef])
}
