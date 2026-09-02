import { useState } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { ChatComposer } from "./ChatComposer"

afterEach(cleanup)

// The composer is controlled, so typing only shows up if the value travels back
// in. This is the caller every consumer of the component already is.
type Props = Parameters<typeof ChatComposer>[0]

function Controlled({ value: initial = "", ...props }: Partial<Props> = {}) {
  const [value, setValue] = useState(initial)
  return <ChatComposer onSubmit={() => {}} {...props} value={value} onChange={setValue} />
}

function input() {
  return screen.getByRole("textbox") as HTMLTextAreaElement
}

describe("ChatComposer ghost suggestion", () => {
  it("offers the suggestion as the placeholder, without touching the value", () => {
    render(<Controlled ghost="yes, ship it" onAcceptGhost={() => {}} />)
    expect(input().placeholder).toBe("yes, ship it")
    expect(input().value).toBe("")
  })

  it("accepts the suggestion on Tab instead of moving focus", () => {
    const onAcceptGhost = vi.fn()
    render(<Controlled ghost="yes, ship it" onAcceptGhost={onAcceptGhost} />)

    const event = createTabEvent()
    fireEvent(input(), event)
    expect(onAcceptGhost).toHaveBeenCalledOnce()
    expect(event.defaultPrevented).toBe(true)
  })

  // Tab is how a keyboard user leaves a field. Swallowing it when there is
  // nothing to accept would trap them in the composer.
  it("leaves Tab alone when nothing is on offer", () => {
    const onAcceptGhost = vi.fn()
    render(<Controlled onAcceptGhost={onAcceptGhost} />)

    const event = createTabEvent()
    fireEvent(input(), event)
    expect(onAcceptGhost).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })

  it("withdraws the offer once the user has typed", () => {
    const onAcceptGhost = vi.fn()
    render(<Controlled ghost="yes, ship it" onAcceptGhost={onAcceptGhost} />)
    fireEvent.change(input(), { target: { value: "no, hold on" } })

    expect(input().placeholder).not.toBe("yes, ship it")
    const event = createTabEvent()
    fireEvent(input(), event)
    expect(onAcceptGhost).not.toHaveBeenCalled()
    expect(input().value).toBe("no, hold on")
  })

  // A suggestion predicts what to say next; there is no next while the turn it
  // would answer is still running.
  it("withdraws the offer while a run is in flight", () => {
    render(
      <Controlled ghost="yes, ship it" onAcceptGhost={() => {}} loading queueWhileLoading />,
    )
    expect(input().placeholder).not.toBe("yes, ship it")
  })

  it("says nothing about Tab when no suggestion is offered", () => {
    render(<Controlled />)
    expect(screen.queryByText("Tab")).toBeNull()
    render(<Controlled ghost="yes" onAcceptGhost={() => {}} />)
    expect(screen.getAllByText("Tab").length).toBe(1)
  })

  // Without a handler there is nothing Tab could do, so the offer is not made.
  it("does not offer a suggestion nobody can accept", () => {
    render(<Controlled ghost="yes, ship it" />)
    expect(input().placeholder).not.toBe("yes, ship it")
  })
})

describe("ChatComposer submit", () => {
  it("still submits on Enter with a suggestion on offer", () => {
    const onSubmit = vi.fn()
    render(<Controlled value="typed by hand" onSubmit={onSubmit} ghost="yes" onAcceptGhost={() => {}} />)
    fireEvent.keyDown(input(), { key: "Enter" })
    expect(onSubmit).toHaveBeenCalledOnce()
  })

  // The parent sees the key first and may claim it — a slash-command popup
  // navigating with the arrow keys, say.
  it("lets the parent claim a key before the ghost does", () => {
    const onAcceptGhost = vi.fn()
    render(
      <Controlled
        ghost="yes"
        onAcceptGhost={onAcceptGhost}
        onKeyDown={(e) => e.preventDefault()}
      />,
    )
    fireEvent(input(), createTabEvent())
    expect(onAcceptGhost).not.toHaveBeenCalled()
  })
})

// The buttons are icon-only, so the state's word lives on the accessible name.
describe("ChatComposer action button", () => {
  it("names the send button and submits on click", () => {
    const onSubmit = vi.fn()
    render(<Controlled value="hi" onSubmit={onSubmit} />)
    const send = screen.getByRole("button", { name: "Send" })
    fireEvent.click(send)
    expect(onSubmit).toHaveBeenCalledOnce()
  })

  it("shows a Stop button while a cancellable run is in flight", () => {
    const onCancel = vi.fn()
    render(<Controlled loading onCancel={onCancel} />)
    expect(screen.queryByRole("button", { name: "Send" })).toBeNull()
    fireEvent.click(screen.getByRole("button", { name: "Stop" }))
    expect(onCancel).toHaveBeenCalledOnce()
  })

  it("names the button Queue when a message is typed mid-run", () => {
    render(<Controlled value="later" loading queueWhileLoading />)
    expect(screen.getByRole("button", { name: "Queue" })).toBeTruthy()
  })
})

// fireEvent.keyDown builds its own event, and the object it returns says only
// whether the event was cancelled by a handler — which is exactly what these
// tests assert on, so the event is built here and kept.
function createTabEvent() {
  return new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true })
}
