import { afterEach, describe, expect, it, vi } from 'vitest'
import { EventsOff, EventsOn } from './wailsRuntime.js'

afterEach(() => {
  delete globalThis.window
})

describe('Wails event adapter', () => {
  it('is safe before the Wails runtime is available', () => {
    expect(EventsOn('event', vi.fn())).toEqual(expect.any(Function))
    expect(() => EventsOff('event')).not.toThrow()
  })

  it('delegates subscriptions and unsubscriptions to Wails', () => {
    const unsubscribe = vi.fn()
    const eventsOn = vi.fn(() => unsubscribe)
    const eventsOff = vi.fn()
    globalThis.window = { runtime: { EventsOn: eventsOn, EventsOff: eventsOff } }
    const callback = vi.fn()

    expect(EventsOn('desktop/test', callback)).toBe(unsubscribe)
    expect(eventsOn).toHaveBeenCalledWith('desktop/test', callback)
    EventsOff('desktop/test', 'desktop/other')
    expect(eventsOff).toHaveBeenCalledWith('desktop/test', 'desktop/other')
  })
})
