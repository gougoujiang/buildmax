import { useState, useRef, useEffect } from "react"
import { createPortal } from "react-dom"
import { cn } from "../lib/cn"

export interface WorkspaceOption {
  id: string
  name: string
}

interface WorkspaceSelectProps {
  value: string
  options: WorkspaceOption[]
  onChange: (id: string) => void
  ariaLabel: string
  className?: string
  triggerClassName?: string
}

export function WorkspaceSelect({
  value,
  options,
  onChange,
  ariaLabel,
  className,
  triggerClassName,
}: WorkspaceSelectProps) {
  const SPACE_THRESHOLD = 220

  const [open, setOpen] = useState(false)
  const [highlightIndex, setHighlightIndex] = useState(-1)
  const [dropdownRect, setDropdownRect] = useState<{
    top?: number
    bottom?: number
    left: number
    width: number
    openUp: boolean
  } | null>(null)
  const wrapperRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLUListElement>(null)

  const selectedOption = options.find((o) => o.id === value)
  const displayName = selectedOption?.name ?? ""

  useEffect(() => {
    if (!open || !wrapperRef.current) return
    const trigger = wrapperRef.current.querySelector(".sidebar__workspace-select-trigger") as HTMLElement
    if (trigger) {
      const r = trigger.getBoundingClientRect()
      const spaceBelow = window.innerHeight - r.bottom
      const openUp = spaceBelow < SPACE_THRESHOLD && r.top > spaceBelow
      if (openUp) {
        setDropdownRect({
          bottom: window.innerHeight - r.top + 2,
          left: r.left,
          width: r.width,
          openUp: true,
        })
      } else {
        setDropdownRect({
          top: r.bottom + 2,
          left: r.left,
          width: r.width,
          openUp: false,
        })
      }
    }
    setHighlightIndex(options.findIndex((o) => o.id === value))
  }, [open, options, value])

  useEffect(() => {
    if (!open) return
    function handleClickOutside(e: MouseEvent) {
      const target = e.target as Node
      const inWrapper = wrapperRef.current?.contains(target)
      const inList = listRef.current?.contains(target)
      if (!inWrapper && !inList) setOpen(false)
    }
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        setOpen(false)
        return
      }
      if (!open) return
      if (e.key === "ArrowDown") {
        e.preventDefault()
        setHighlightIndex((i) => (i < options.length - 1 ? i + 1 : 0))
        return
      }
      if (e.key === "ArrowUp") {
        e.preventDefault()
        setHighlightIndex((i) => (i > 0 ? i - 1 : options.length - 1))
        return
      }
      if (e.key === "Enter" && highlightIndex >= 0 && options[highlightIndex]) {
        e.preventDefault()
        onChange(options[highlightIndex].id)
        setOpen(false)
      }
    }
    document.addEventListener("mousedown", handleClickOutside)
    document.addEventListener("keydown", handleKeyDown)
    return () => {
      document.removeEventListener("mousedown", handleClickOutside)
      document.removeEventListener("keydown", handleKeyDown)
    }
  }, [open, onChange, options, highlightIndex])

  useEffect(() => {
    if (!open || highlightIndex < 0) return
    const el = listRef.current?.children[highlightIndex] as HTMLElement | undefined
    el?.scrollIntoView({ block: "nearest" })
  }, [open, highlightIndex])

  function handleSelect(id: string) {
    onChange(id)
    setOpen(false)
  }

  const dropdown = open && dropdownRect && createPortal(
    <ul
      ref={listRef}
      className={cn(
        "sidebar__workspace-dropdown-list",
        dropdownRect.openUp && "sidebar__workspace-dropdown-list--up"
      )}
      role="listbox"
      aria-label={ariaLabel}
      style={{
        position: "fixed",
        ...(dropdownRect.openUp
          ? { bottom: dropdownRect.bottom, left: dropdownRect.left, width: dropdownRect.width }
          : { top: dropdownRect.top, left: dropdownRect.left, width: dropdownRect.width }),
        zIndex: 1000,
      }}
    >
      {options.map((opt, i) => (
        <li key={opt.id} role="option" aria-selected={opt.id === value}>
          <button
            type="button"
            className={cn(
              "sidebar__workspace-dropdown-option",
              opt.id === value && "sidebar__workspace-dropdown-option--selected",
              i === highlightIndex && "sidebar__workspace-dropdown-option--highlight"
            )}
            onClick={() => handleSelect(opt.id)}
            onMouseEnter={() => setHighlightIndex(i)}
          >
            {opt.name}
          </button>
        </li>
      ))}
    </ul>,
    document.body
  )

  return (
    <div className={cn("sidebar__workspace-select-wrap", className)} ref={wrapperRef}>
      <button
        type="button"
        className={cn("sidebar__workspace-select-trigger", triggerClassName)}
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
        aria-activedescendant={open && options[highlightIndex] ? options[highlightIndex].id : undefined}
      >
        <span className="sidebar__workspace-select-value">{displayName}</span>
        <span className="sidebar__workspace-select-chevron" aria-hidden>
          {open && dropdownRect?.openUp ? "▼" : open ? "▲" : "▼"}
        </span>
      </button>
      {dropdown}
    </div>
  )
}
