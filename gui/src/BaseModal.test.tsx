import { useState } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { BaseModal } from "./BaseModal"

afterEach(cleanup)

function Harness({ onClose }: { onClose: () => void }) {
  const [open, setOpen] = useState(false)
  return (
    <div>
      <button type="button" onClick={() => setOpen(true)}>
        opener
      </button>
      <BaseModal
        open={open}
        title="Test modal"
        titleId="test-modal-title"
        onClose={() => {
          setOpen(false)
          onClose()
        }}
      >
        <input placeholder="field" />
        <button type="button">confirm</button>
      </BaseModal>
    </div>
  )
}

describe("BaseModal", () => {
  it("renders nothing when closed", () => {
    render(<Harness onClose={() => {}} />)
    expect(screen.queryByRole("dialog")).toBeNull()
  })

  it("labels the dialog by its title", async () => {
    render(<Harness onClose={() => {}} />)
    fireEvent.click(screen.getByText("opener"))
    const dialog = await screen.findByRole("dialog")
    expect(dialog.getAttribute("aria-labelledby")).toBe("test-modal-title")
  })

  it("traps Tab focus inside the dialog", async () => {
    render(<Harness onClose={() => {}} />)
    fireEvent.click(screen.getByText("opener"))
    await screen.findByRole("dialog")
    const confirm = screen.getByText("confirm")
    confirm.focus()
    // confirm is the last focusable element inside the dialog (close button,
    // field, confirm) — Tab from here must wrap back to the close button.
    fireEvent.keyDown(document, { key: "Tab" })
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Close")
  })

  it("closes on Escape and restores focus to the opener", async () => {
    const onClose = vi.fn()
    render(<Harness onClose={onClose} />)
    const opener = screen.getByText("opener")
    opener.focus()
    fireEvent.click(opener)
    await screen.findByRole("dialog")
    fireEvent.keyDown(document, { key: "Escape" })
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole("dialog")).toBeNull()
    expect(document.activeElement).toBe(opener)
  })

  it("closes when the overlay backdrop is clicked but not the dialog itself", async () => {
    const onClose = vi.fn()
    render(<Harness onClose={onClose} />)
    fireEvent.click(screen.getByText("opener"))
    const dialog = await screen.findByRole("dialog")
    fireEvent.click(dialog)
    expect(onClose).not.toHaveBeenCalled()
    fireEvent.click(dialog.parentElement as HTMLElement)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it("locks body scroll while open and releases it on close", async () => {
    render(<Harness onClose={() => {}} />)
    fireEvent.click(screen.getByText("opener"))
    await screen.findByRole("dialog")
    expect(document.body.style.overflow).toBe("hidden")
    fireEvent.keyDown(document, { key: "Escape" })
    expect(document.body.style.overflow).not.toBe("hidden")
  })
})
