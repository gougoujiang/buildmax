import { afterEach, describe, expect, it } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { FormModal, type FormModalFieldConfig, type FormModalGroup } from "./FormModal"

afterEach(cleanup)

const FIELDS: FormModalFieldConfig[] = [
  { key: "name", label: "Name", type: "text", group: "basics" },
  { key: "endpoint", label: "Endpoint", type: "text", optional: true, group: "advanced" },
]

const GROUPS: FormModalGroup[] = [
  { id: "basics", title: "Basics" },
  { id: "advanced", title: "Advanced", collapsible: true },
]

function base() {
  return {
    open: true,
    title: "Test",
    titleId: "test-title",
    submitLabel: "Save",
    onClose: () => {},
    onSubmit: () => {},
  }
}

describe("FormModal grouping", () => {
  it("renders every field flat when no groups are given", () => {
    render(<FormModal {...base()} fields={FIELDS} />)
    expect(screen.getByLabelText("Name")).toBeTruthy()
    expect(screen.getByLabelText(/Endpoint/)).toBeTruthy()
  })

  it("keeps a collapsible group closed by default, hiding its fields", () => {
    render(<FormModal {...base()} fields={FIELDS} groups={GROUPS} />)
    // The non-collapsible group's field is visible; the collapsed one's is not.
    expect(screen.getByLabelText("Name")).toBeTruthy()
    expect(screen.queryByLabelText(/Endpoint/)).toBeNull()
  })

  it("reveals a collapsed group's fields when its header is clicked", () => {
    render(<FormModal {...base()} fields={FIELDS} groups={GROUPS} />)
    fireEvent.click(screen.getByRole("button", { name: /Advanced/ }))
    expect(screen.getByLabelText(/Endpoint/)).toBeTruthy()
  })

  it("opens a collapsible group when defaultOpen is set", () => {
    const groups: FormModalGroup[] = [
      { id: "basics", title: "Basics" },
      { id: "advanced", title: "Advanced", collapsible: true, defaultOpen: true },
    ]
    render(<FormModal {...base()} fields={FIELDS} groups={groups} />)
    expect(screen.getByLabelText(/Endpoint/)).toBeTruthy()
  })

  it("renders a group's extra content only while the group is open", () => {
    const groups: FormModalGroup[] = [
      { id: "basics", title: "Basics" },
      {
        id: "advanced",
        title: "Advanced",
        collapsible: true,
        content: <p>extra content</p>,
      },
    ]
    render(<FormModal {...base()} fields={FIELDS} groups={groups} />)
    expect(screen.queryByText("extra content")).toBeNull()
    fireEvent.click(screen.getByRole("button", { name: /Advanced/ }))
    expect(screen.getByText("extra content")).toBeTruthy()
  })

  it("counts a required field inside a collapsed group, keeping submit disabled", () => {
    const fields: FormModalFieldConfig[] = [
      { key: "token", label: "Token", type: "text", group: "advanced" },
    ]
    const groups: FormModalGroup[] = [{ id: "advanced", title: "Advanced", collapsible: true }]
    render(<FormModal {...base()} fields={fields} groups={groups} />)
    const submit = screen.getByRole("button", { name: "Save" }) as HTMLButtonElement
    expect(submit.disabled).toBe(true)
  })
})

describe("FormModal tabs layout", () => {
  it("shows only the active tab's fields, hiding the others until selected", () => {
    render(<FormModal {...base()} fields={FIELDS} groups={GROUPS} layout="tabs" />)
    // First group is the active tab; the second group's field is not rendered.
    expect(screen.getByLabelText("Name")).toBeTruthy()
    expect(screen.queryByLabelText(/Endpoint/)).toBeNull()
  })

  it("switches the panel when a sidebar tab is clicked", () => {
    render(<FormModal {...base()} fields={FIELDS} groups={GROUPS} layout="tabs" />)
    fireEvent.click(screen.getByRole("tab", { name: "Advanced" }))
    expect(screen.getByLabelText(/Endpoint/)).toBeTruthy()
    // Leaving Basics hides its field — one tab at a time.
    expect(screen.queryByLabelText("Name")).toBeNull()
  })

  it("renders a tab's extra content when its tab is active", () => {
    const groups: FormModalGroup[] = [
      { id: "basics", title: "Basics" },
      { id: "history", title: "History", content: <p>revision list</p> },
    ]
    render(<FormModal {...base()} fields={FIELDS} groups={groups} layout="tabs" />)
    expect(screen.queryByText("revision list")).toBeNull()
    fireEvent.click(screen.getByRole("tab", { name: "History" }))
    expect(screen.getByText("revision list")).toBeTruthy()
  })

  it("exposes the ARIA tabs pattern: tablist, selected state, and panel linkage", () => {
    render(<FormModal {...base()} fields={FIELDS} groups={GROUPS} layout="tabs" />)
    expect(screen.getByRole("tablist", { name: "Test" })).toBeTruthy()
    const basicsTab = screen.getByRole("tab", { name: "Basics" })
    const advancedTab = screen.getByRole("tab", { name: "Advanced" })
    expect(basicsTab.getAttribute("aria-selected")).toBe("true")
    expect(advancedTab.getAttribute("aria-selected")).toBe("false")
    expect(basicsTab.getAttribute("tabIndex") ?? basicsTab.getAttribute("tabindex")).toBe("0")
    expect(advancedTab.getAttribute("tabIndex") ?? advancedTab.getAttribute("tabindex")).toBe("-1")
    const panel = screen.getByRole("tabpanel")
    expect(panel.getAttribute("aria-labelledby")).toBe(basicsTab.id)
    expect(basicsTab.getAttribute("aria-controls")).toBe(panel.id)
  })

  it("moves selection and focus with the arrow keys, wrapping at the ends", () => {
    render(<FormModal {...base()} fields={FIELDS} groups={GROUPS} layout="tabs" />)
    const basicsTab = screen.getByRole("tab", { name: "Basics" })
    const advancedTab = screen.getByRole("tab", { name: "Advanced" })
    basicsTab.focus()
    fireEvent.keyDown(basicsTab, { key: "ArrowRight" })
    expect(document.activeElement).toBe(advancedTab)
    expect(advancedTab.getAttribute("aria-selected")).toBe("true")
    expect(screen.getByLabelText(/Endpoint/)).toBeTruthy()
    fireEvent.keyDown(advancedTab, { key: "ArrowRight" })
    expect(document.activeElement).toBe(basicsTab)
    fireEvent.keyDown(basicsTab, { key: "End" })
    expect(document.activeElement).toBe(advancedTab)
    fireEvent.keyDown(advancedTab, { key: "Home" })
    expect(document.activeElement).toBe(basicsTab)
  })
})
