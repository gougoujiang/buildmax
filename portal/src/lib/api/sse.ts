export interface SSECallbacks {
  onData: (data: string) => boolean | void
  onDone: () => void
  onError: (err: Error) => void
  /**
   * Called for a frame that carries an `event:` name. Return false to stop
   * reading, the same as onData. Frames with a name are never passed to onData:
   * a named event is protocol, and onData carries agent output.
   *
   * The one the server sends today is `draining`, meaning this instance is
   * stopping and the stream should be reopened elsewhere.
   */
  onEvent?: (event: string, data: string) => boolean | void
}

export function parseSSEEventPayload(event: string): string | null {
  const lines = event.split("\n").filter((line) => line.startsWith("data: "))
  if (lines.length === 0) return null
  return lines.map((line) => line.slice(6)).join("\n")
}

/** Reads the `event:` name of a frame, or null for an unnamed one. */
export function parseSSEEventName(event: string): string | null {
  const line = event.split("\n").find((l) => l.startsWith("event: "))
  return line ? line.slice(7).trim() : null
}

export async function readSSEStream(
  res: Response,
  callbacks: SSECallbacks
): Promise<void> {
  const reader = res.body?.getReader()
  if (!reader) {
    callbacks.onDone()
    return
  }

  const decoder = new TextDecoder()
  let buffer = ""

  // Returns false when the caller asked to stop reading.
  const dispatch = (event: string): boolean => {
    const data = parseSSEEventPayload(event)
    if (data === null) return true
    const name = parseSSEEventName(event)
    if (name !== null) {
      return callbacks.onEvent?.(name, data) !== false
    }
    return callbacks.onData(data) !== false
  }

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const events = buffer.split("\n\n")
      buffer = events.pop() ?? ""
      for (const event of events) {
        if (!dispatch(event)) return
      }
    }

    if (buffer.trim() && !dispatch(buffer)) return

    callbacks.onDone()
  } catch (err) {
    callbacks.onError(err instanceof Error ? err : new Error(String(err)))
  }
}
