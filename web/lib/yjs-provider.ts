import * as Y from 'yjs'
import * as awarenessProtocol from 'y-protocols/awareness'
import { getAccessToken } from '@/lib/api-client'

// Helper: encode a varUint (unsigned LEB128)
function encodeVarUint(num: number): Uint8Array {
  const bytes: number[] = []
  while (num >= 128) {
    bytes.push((num & 127) | 128)
    num >>= 7
  }
  bytes.push(num)
  return new Uint8Array(bytes)
}

// Helper: encode a varBuffer (varUint length + data)
function encodeVarBuffer(data: Uint8Array): Uint8Array {
  const len = encodeVarUint(data.length)
  const out = new Uint8Array(len.length + data.length)
  out.set(len, 0)
  out.set(data, len.length)
  return out
}

// Build a sync message: [MsgSync=0] [syncType] [varBuffer(data)]
function buildSyncMessage(syncType: number, data: Uint8Array): Uint8Array {
  const header = new Uint8Array([0])
  const syncTypeEncoded = encodeVarUint(syncType)
  const buf = encodeVarBuffer(data)
  const out = new Uint8Array(header.length + syncTypeEncoded.length + buf.length)
  let offset = 0
  out.set(header, offset); offset += header.length
  out.set(syncTypeEncoded, offset); offset += syncTypeEncoded.length
  out.set(buf, offset)
  return out
}

// Build awareness message: [MsgAwareness=1] [awareness bytes]
function buildAwarenessMessage(data: Uint8Array): Uint8Array {
  const out = new Uint8Array(1 + data.length)
  out[0] = 1
  out.set(data, 1)
  return out
}

export enum ConnectionStatus {
  Disconnected = 'disconnected',
  Connecting = 'connecting',
  Connected = 'connected',
  Reconnecting = 'reconnecting',
}

type StatusCallback = (status: ConnectionStatus) => void

export class PulseWSProvider {
  private doc: Y.Doc
  private url: string
  private ws: WebSocket | null = null
  private _awareness: awarenessProtocol.Awareness
  private _synced = false
  private shouldConnect = false
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private reconnectAttempts = 0
  private maxReconnectDelay = 30000
  private baseReconnectDelay = 1000
  private pingInterval: ReturnType<typeof setInterval> | null = null
  private _status: ConnectionStatus = ConnectionStatus.Disconnected
  private statusListeners: StatusCallback[] = []

  constructor(doc: Y.Doc, wsUrl: string) {
    this.doc = doc
    this.url = wsUrl
    this._awareness = new awarenessProtocol.Awareness(doc)
    this._awareness.setLocalState(null)
  }

  get awareness() {
    return this._awareness
  }

  get synced() {
    return this._synced
  }

  get status() {
    return this._status
  }

  onStatus(cb: StatusCallback) {
    this.statusListeners.push(cb)
    return () => {
      this.statusListeners = this.statusListeners.filter((l) => l !== cb)
    }
  }

  connect() {
    if (this.ws) return
    this.shouldConnect = true
    this._connect()
  }

  disconnect() {
    this.shouldConnect = false
    this._disconnect()
  }

  private _connect() {
    if (!this.shouldConnect) return
    this._updateStatus(ConnectionStatus.Connecting)

    const token = getAccessToken()
    const wsUrl = `${this.url}?token=${token}`
    this.ws = new WebSocket(wsUrl)
    this.ws.binaryType = 'arraybuffer'

    this.ws.onopen = () => {
      this._updateStatus(ConnectionStatus.Connected)
      this.reconnectAttempts = 0
      this._startPing()

      // Send SYNC_STEP1: request missing updates from server
      const sv = Y.encodeStateVector(this.doc)
      const syncMsg = buildSyncMessage(0, sv)
      this.send(syncMsg)

      // Send initial awareness
      this._sendAwareness()
    }

    this.ws.onmessage = (event) => {
      const data = new Uint8Array(event.data)
      const firstByte = data[0]

      if (firstByte <= 2) {
        this._handleSyncMessage(data)
      } else if (firstByte === 1) {
        awarenessProtocol.applyAwarenessUpdate(this._awareness, data.slice(1), this)
      }
    }

    this.ws.onclose = () => {
      this._stopPing()
      this.ws = null
      if (this.shouldConnect) {
        this._updateStatus(ConnectionStatus.Reconnecting)
        this._scheduleReconnect()
      } else {
        this._updateStatus(ConnectionStatus.Disconnected)
      }
    }

    this.ws.onerror = () => {
      this.ws?.close()
    }
  }

  private _disconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    this._stopPing()
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this._synced = false
    this._updateStatus(ConnectionStatus.Disconnected)
  }

  private _scheduleReconnect() {
    if (!this.shouldConnect) return
    const delay = Math.min(
      this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts),
      this.maxReconnectDelay,
    )
    this.reconnectAttempts++
    this.reconnectTimer = setTimeout(() => this._connect(), delay)
  }

  private _startPing() {
    this._stopPing()
    this.pingInterval = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        const ping = new Uint8Array([0x06])
        this.ws.send(ping)
      }
    }, 30000)
  }

  private _stopPing() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval)
      this.pingInterval = null
    }
  }

  private _lastAwarenessSent = 0
  private _pendingAwareness: ReturnType<typeof setTimeout> | null = null

  private _sendAwareness() {
    const now = Date.now()
    const throttleMs = 50 // max 20 updates/detik
    if (this._pendingAwareness) return // already queued

    const elapsed = now - this._lastAwarenessSent
    if (elapsed < throttleMs) {
      this._pendingAwareness = setTimeout(() => {
        this._pendingAwareness = null
        this._sendAwarenessNow()
      }, throttleMs - elapsed)
      return
    }
    this._sendAwarenessNow()
  }

  private _sendAwarenessNow() {
    this._lastAwarenessSent = Date.now()
    const state = this._awareness.getLocalState()
    if (state) {
      const encoded = awarenessProtocol.encodeAwarenessUpdate(this._awareness, [this._awareness.clientID])
      this.send(buildAwarenessMessage(encoded))
    }
  }

  private _handleSyncMessage(data: Uint8Array) {
    // data[0] = MsgSync = 0
    // then varUint(syncType), varBuffer(data)
    let offset = 1
    // read syncType varUint
    let syncType = 0
    let shift = 0
    while (offset < data.length) {
      const byte = data[offset]
      syncType |= (byte & 127) << shift
      shift += 7
      offset++
      if ((byte & 128) === 0) break
    }
    // read varBuffer length
    let bufLen = 0
    shift = 0
    while (offset < data.length) {
      const byte = data[offset]
      bufLen |= (byte & 127) << shift
      shift += 7
      offset++
      if ((byte & 128) === 0) break
    }
    const payload = data.slice(offset, offset + bufLen)

    switch (syncType) {
      case 0: // SyncStep1 - server requested state
        {
          const update = Y.encodeStateAsUpdate(this.doc)
          this.send(buildSyncMessage(1, update))
        }
        break
      case 1: // SyncStep2 - full state from server
        {
          Y.applyUpdate(this.doc, payload)
          this._synced = true
        }
        break
      case 2: // Update - incremental update
        {
          Y.applyUpdate(this.doc, payload)
        }
        break
    }
  }

  private _updateStatus(status: ConnectionStatus) {
    this._status = status
    this.statusListeners.forEach((cb) => cb(status))
  }

  send(data: Uint8Array, cb?: () => void) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(data)
      if (cb) cb()
    }
  }
}
