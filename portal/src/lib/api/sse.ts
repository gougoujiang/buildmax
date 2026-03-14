export interface SSECallbacks {
  onData: (data: string) => boolean | void
  onDone: () => void
  onError: (err: Error) => void
}

export function parseSSEEventPayload(event: string): string | null {
  const lines = event.split("\n").filter((line) => line.startsWith("data: "))
  if (lines.length === 0) return null
  return lines.map((line) => line.slice(6)).join("\n")
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

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const events = buffer.split("\n\n")
      buffer = events.pop() ?? ""
      for (const event of events) {
        const data = parseSSEEventPayload(event)
        if (data === null) continue
        if (callbacks.onData(data) === false) return
      }
    }

    if (buffer.trim()) {
      const data = parseSSEEventPayload(buffer)
      if (data !== null && callbacks.onData(data) === false) return
    }

    callbacks.onDone()
  } catch (err) {
    callbacks.onError(err instanceof Error ? err : new Error(String(err)))
  }
}
