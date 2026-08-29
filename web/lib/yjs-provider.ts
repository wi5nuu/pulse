import * as Y from 'yjs'
import * as awarenessProtocol from 'y-protocols/awareness'
import { getAccessToken, refreshAccessToken, notifySessionExpired } from '@/lib/api-client'

// Helper: encode a varUint (unsigned LEB128)
function encodeVarUint(num: number): Uint8Array {
  const bytes: number[] = []
  while (num >= 128) {
    // Pakai aritmatika (bukan bitwise) — bitwise di JS terbatas 32-bit dan
    // korup untuk panjang payload >= 2^31.
    bytes.push((num % 128) | 128)
    num = Math.floor(num / 128)
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

// Role dari server (MsgRole): owner/editor/viewer/view — dipakai client
// untuk render editor read-only.
export type WSRole = 'owner' | 'editor' | 'viewer' | 'view'

type StatusCallback = (status: ConnectionStatus) => void
type RoleCallback = (role: WSRole) => void
type DocEventCallback = (payload: string) => void

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
  private _role: WSRole | null = null
  private roleListeners: RoleCallback[] = []
  private docEventListeners: DocEventCallback[] = []

  constructor(doc: Y.Doc, wsUrl: string) {
    this.doc = doc
    this.url = wsUrl
    this._awareness = new awarenessProtocol.Awareness(doc)
    this._awareness.setLocalState(null)
    // Forward setiap perubahan awareness lokal (cursor move, selection, nama)
    // ke server. Tanpa listener ini, presence hanya terkirim sekali saat
    // koneksi pertama dan cursor tidak pernah bergerak untuk user lain.
    this._awareness.on('update', this._onAwarenessUpdate)
    // FIX C1 (kritis): forward setiap update dokumen (keystroke/edit) ke
    // server. Sebelumnya listener ini TIDAK ADA — edit lokal tidak pernah
    // dikirim, sehingga real-time relay & persistence tidak pernah terpicu.
    // Saat terhubung, update dikirim sebagai pesan Update (syncType 2).
    // Saat offline, update disimpan di Y.Doc dan dikirim saat reconnect.
    this.doc.on('update', this._onDocUpdate)
  }

  private _onDocUpdate = (update: Uint8Array, origin: unknown) => {
    if (origin === this) return
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.send(buildSyncMessage(2, update))
    } else {
      // Update saat offline: tandai sebagai unsent. Saat reconnect,
      // server minta state (fix C2) dan edit ini ikut terkirim.
      this._hasUnsentChanges = true
    }
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

  get role(): WSRole | null {
    return this._role
  }

  onRole(cb: RoleCallback) {
    this.roleListeners.push(cb)
    return () => {
      this.roleListeners = this.roleListeners.filter((l) => l !== cb)
    }
  }

  // onDocEvent: event dokumen realtime dari server (komentar dll) —
  // payload = string JSON, client menginterpretasi.
  onDocEvent(cb: DocEventCallback) {
    this.docEventListeners.push(cb)
    return () => {
      this.docEventListeners = this.docEventListeners.filter((l) => l !== cb)
    }
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
      this._lastMessageAt = Date.now()
      const firstByte = data[0]

      if (firstByte === 0) {
        // MsgSync — data = varUint(syncType) • varBuffer(payload)
        this._handleSyncMessage(data)
      } else if (firstByte === 1) {
        // MsgAwareness — presence/cursor dari user lain.
        awarenessProtocol.applyAwarenessUpdate(this._awareness, data.slice(1), this)
      } else if (firstByte === 5) {
        // MsgRole — role user dari server (owner/editor/viewer/view).
        const role = new TextDecoder().decode(data.slice(1)) as WSRole
        if (this._role !== role) {
          this._role = role
          this.roleListeners.forEach((cb) => cb(role))
        }
      } else if (firstByte === 8) {
        // MsgDocEvent — event dokumen realtime (komentar dll) dari server.
        const payload = new TextDecoder().decode(data.slice(1))
        this.docEventListeners.forEach((cb) => cb(payload))
      }
      // Byte lain (ping/pong/auth) diabaikan.
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

  // Reconnect dengan refresh token jika diperlukan (awaited, bukan
  // fire-and-forget — fix race: reconnect sebelumnya bisa jalan dengan
  // token basi karena refresh belum selesai). Return true jika siap
  // reconnect, false jika session mati (harus berhenti, jangan loop 401).
  private async _refreshTokenIfNeeded(): Promise<boolean> {
    if (this.reconnectAttempts > 0 && this.reconnectAttempts % 3 === 0) {
      const token = await refreshAccessToken()
      if (!token) return false
    }
    return true
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

  destroy() {
    this.disconnect()
    if (this._pendingAwareness) {
      clearTimeout(this._pendingAwareness)
      this._pendingAwareness = null
    }
    this._awareness.off('update', this._onAwarenessUpdate)
    this.doc.off('update', this._onDocUpdate)
    this.roleListeners = []
    this.statusListeners = []
    this.docEventListeners = []
  }

  private _onAwarenessUpdate = () => {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this._sendAwareness()
    }
  }

  private _scheduleReconnect() {
    if (!this.shouldConnect) return
    const delay = Math.min(
      this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts),
      this.maxReconnectDelay,
    )
    this.reconnectAttempts++
    this.reconnectTimer = setTimeout(async () => {
      const canReconnect = await this._refreshTokenIfNeeded()
      if (!canReconnect) {
        // Session mati (refresh gagal) — berhenti reconnect + trigger logout
        // global (redirect ke /login) supaya tidak stuck di halaman.
        this.shouldConnect = false
        this._updateStatus(ConnectionStatus.Disconnected)
        notifySessionExpired()
        return
      }
      this._connect()
    }, delay)
  }

  private _startPing() {
    this._stopPing()
    this._lastMessageAt = Date.now()
    // Heartbeat detection (fix): server memutus koneksi setelah 90s diam.
    // Client harus mendeteksi koneksi "zombie" (network drop tanpa close
    // frame) — jika tidak ada pesan apa pun dari server dalam 75s, paksa
    // reconnect.
    this._heartbeatCheck = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        const ping = new Uint8Array([0x06])
        this.ws.send(ping)
        if (Date.now() - this._lastMessageAt > 75000) {
          this.ws.close()
        }
      }
    }, 30000)
  }

  private _stopPing() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval)
      this.pingInterval = null
    }
    if (this._heartbeatCheck) {
      clearInterval(this._heartbeatCheck)
      this._heartbeatCheck = null
    }
  }

  private _lastMessageAt = 0
  private _heartbeatCheck: ReturnType<typeof setInterval> | null = null

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
          // Dokumen baru (server tidak punya state) — setelah mengirim state
          // penuh, dokumen ini sudah sinkron dengan server.
          this._synced = true
        }
        break
      case 1: // SyncStep2 - full state from server
        {
          Y.applyUpdate(this.doc, payload, this)
          this._synced = true
          // FIX C2: setelah menerima state server, jika kita punya edit
          // lokal yang dibuat saat offline (belum terkirim), kirim state
          // penuh kita ke server supaya tidak hilang. Server akan merge
          // secara CRDT di sisi client saat relay.
          if (this._hasUnsentChanges) {
            const update = Y.encodeStateAsUpdate(this.doc)
            this.send(buildSyncMessage(1, update))
            this._hasUnsentChanges = false
          }
        }
        break
      case 2: // Update - incremental update
        {
          Y.applyUpdate(this.doc, payload, this)
        }
        break
    }
  }

  private _hasUnsentChanges = false

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
