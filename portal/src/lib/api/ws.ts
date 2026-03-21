import { getApiBase, UNAUTHORIZED_EVENT } from "./client"

export interface WsEnvelope {
  type: string
  payload: unknown
}

type EventHandler = (payload: any) => void

const RECONNECT_MIN = 1000
const RECONNECT_MAX = 30000
const STABLE_CONNECTION_MS = 10000

/**
 * BuildMaxWebSocket manages a persistent WebSocket connection to the server.
 * Supports typed events, auto-reconnect with exponential backoff.
 */
export class BuildMaxWebSocket {
  private ws: WebSocket | null = null
  private token: string | null = null
  private handlers = new Map<string, Set<EventHandler>>()
  private intentionalClose = false
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private reconnectDelay = RECONNECT_MIN
  private connectedAt = 0
  private sendQueue: string[] = []

  /** Lifecycle callback: called when the connection opens (or re-opens). */
  onOpen: (() => void) | null = null
  /** Lifecycle callback: called when the connection closes unexpectedly. */
  onClose: (() => void) | null = null

  connect(token: string): void {
    this.token = token
    this.intentionalClose = false
    this.openSocket()
  }

  private openSocket(): void {
    if (!this.token) return

    const httpBase = getApiBase()
    const wsBase = httpBase.replace(/^http/, "ws")
    const url = `${wsBase}/api/ws?token=${encodeURIComponent(this.token)}`

    console.log("[ws] connecting", wsBase)
    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      console.log("[ws] connected")
      this.connectedAt = Date.now()
      this.reconnectDelay = RECONNECT_MIN
      this.flushQueue()
      this.onOpen?.()
    }

    this.ws.onmessage = (event) => {
      try {
        const env = JSON.parse(event.data as string) as WsEnvelope
        console.debug("[ws] recv", env.type, env.payload)
        this.dispatch(env.type, env.payload)
      } catch {
        console.warn("[ws] recv unparseable message", event.data)
      }
    }

    this.ws.onclose = (event) => {
      console.log("[ws] closed", { code: event.code, reason: event.reason, wasClean: event.wasClean })
      if (event.code === 4001 || event.code === 1008) {
        window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT))
        return
      }
      if (!this.intentionalClose) {
        this.onClose?.()
        this.scheduleReconnect()
      }
    }

    this.ws.onerror = (err) => {
      console.warn("[ws] error", err)
    }
  }

  send(type: string, payload: unknown): void {
    const msg = JSON.stringify({ type, payload })
    if (this.ws?.readyState === WebSocket.OPEN) {
      console.debug("[ws] send", type, payload)
      this.ws.send(msg)
    } else {
      console.debug("[ws] queued (not open)", type, payload)
      this.sendQueue.push(msg)
    }
  }

  on(type: string, cb: EventHandler): void {
    let set = this.handlers.get(type)
    if (!set) {
      set = new Set()
      this.handlers.set(type, set)
    }
    set.add(cb)
  }

  off(type: string, cb: EventHandler): void {
    this.handlers.get(type)?.delete(cb)
  }

  close(): void {
    console.log("[ws] closing (intentional)")
    this.intentionalClose = true
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    this.ws?.close(1000)
    this.ws = null
    this.sendQueue = []
  }

  get connected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  private dispatch(type: string, payload: unknown): void {
    const set = this.handlers.get(type)
    if (set) {
      for (const cb of set) {
        try {
          cb(payload)
        } catch {
          // handler error; ignore to keep dispatching
        }
      }
    }
  }

  private flushQueue(): void {
    while (this.sendQueue.length > 0 && this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(this.sendQueue.shift()!)
    }
  }

  private scheduleReconnect(): void {
    if (this.intentionalClose) return
    if (this.reconnectTimer) return

    const wasStable = Date.now() - this.connectedAt > STABLE_CONNECTION_MS
    if (wasStable) {
      this.reconnectDelay = RECONNECT_MIN
    }

    console.log("[ws] reconnecting in", this.reconnectDelay, "ms")
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      console.log("[ws] reconnecting now")
      this.openSocket()
    }, this.reconnectDelay)

    this.reconnectDelay = Math.min(this.reconnectDelay * 2, RECONNECT_MAX)
  }
}
