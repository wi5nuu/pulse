const API_BASE = process.env.NEXT_PUBLIC_API_URL || ''

interface FetchOptions extends RequestInit {
  skipAuth?: boolean
}

let accessToken: string | null = null
let refreshPromise: Promise<string | null> | null = null

// Global handler "session expired": didaftarkan oleh auth store. Dipanggil
// ketika refresh token gagal (session sudah mati di server) — client harus
// logout & redirect ke /login. Tanpa ini, halaman yang sudah login akan
// terus polling/reconnect 401 selamanya (bug: sesi mati tapi client tidak tahu).
let unauthorizedHandler: (() => void) | null = null

export function setUnauthorizedHandler(cb: () => void) {
  unauthorizedHandler = cb
}

function notifyUnauthorized() {
  unauthorizedHandler?.()
}

// Export untuk dipanggil dari luar apiFetch (mis. WS provider saat refresh
// gagal di tengah reconnect) — supaya session-expired selalu memicu logout.
export function notifySessionExpired() {
  notifyUnauthorized()
}

export function setAccessToken(token: string | null) {
  accessToken = token
}

export function getAccessToken(): string | null {
  return accessToken
}

async function refreshAccessToken(): Promise<string | null> {
  try {
    const res = await fetch(`${API_BASE}/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    })
    if (!res.ok) return null
    const data = await res.json()
    if (!data.accessToken || typeof data.accessToken !== 'string') return null
    accessToken = data.accessToken
    return data.accessToken
  } catch {
    return null
  }
}

export { refreshAccessToken }

async function apiFetch(path: string, opts: FetchOptions = {}): Promise<Response> {
  const { skipAuth, ...init } = opts
  const headers: Record<string, string> = {
    ...(init.headers as Record<string, string>),
  }

  if (!skipAuth && accessToken) {
    headers['Authorization'] = `Bearer ${accessToken}`
  }

  if (!headers['Content-Type'] && init.body) {
    headers['Content-Type'] = 'application/json'
  }

  let res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers,
    credentials: 'include',
  })

  // 401 → coba refresh dulu. Refresh dijalankan BAIK accessToken ada maupun
  // tidak: setelah hard reload token in-memory kosong, tapi refresh cookie
  // masih valid — tanpa ini session restore /getme gagal dan user di-bounce
  // ke /login padahal masih login.
  if (res.status === 401 && !skipAuth) {
    if (!refreshPromise) {
      refreshPromise = refreshAccessToken().finally(() => {
        refreshPromise = null
      })
    }
    const newToken = await refreshPromise
    if (newToken) {
      headers['Authorization'] = `Bearer ${newToken}`
      res = await fetch(`${API_BASE}${path}`, {
        ...init,
        headers,
        credentials: 'include',
      })
      // Second 401 after successful refresh → session invalid. Trigger logout.
      if (res.status === 401) {
        notifyUnauthorized()
      }
    } else {
      // Refresh gagal → session mati di server. Beri tahu global handler
      // supaya logout + redirect (jangan diam-diam lanjut polling/reconnect).
      notifyUnauthorized()
    }
  }

  return res
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await apiFetch(path)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body?.error?.message || `GET ${path} failed: ${res.status}`)
  }
  return res.json()
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const res = await apiFetch(path, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const b = await res.json().catch(() => ({}))
    throw new Error(b?.error?.message || `POST ${path} failed: ${res.status}`)
  }
  return res.json()
}

export async function apiPatch<T>(path: string, body: unknown): Promise<T> {
  const res = await apiFetch(path, {
    method: 'PATCH',
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const b = await res.json().catch(() => ({}))
    throw new Error(b?.error?.message || `PATCH ${path} failed: ${res.status}`)
  }
  return res.json()
}

export async function apiDelete<T>(path: string): Promise<T> {
  const res = await apiFetch(path, { method: 'DELETE' })
  if (!res.ok) {
    const b = await res.json().catch(() => ({}))
    throw new Error(b?.error?.message || `DELETE ${path} failed: ${res.status}`)
  }
  return res.json()
}

export async function login(email: string, password: string) {
  const data = await apiPost<{ accessToken: string; user: { id: string; email: string; name: string } }>(
    '/auth/login',
    { email, password },
  )
  accessToken = data.accessToken
  return data
}

export async function register(email: string, name: string, password: string) {
  const data = await apiPost<{ accessToken: string; user: { id: string; email: string; name: string } }>(
    '/auth/register',
    { email, name, password },
  )
  accessToken = data.accessToken
  return data
}

export async function logout() {
  await apiFetch('/auth/logout', { method: 'POST' })
  accessToken = null
}

export async function getMe() {
  return apiGet<{ id: string; email: string; name: string }>('/me')
}

export const WS_BASE = process.env.NEXT_PUBLIC_WS_URL || 'ws://localhost:8080'
