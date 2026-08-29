import { create } from 'zustand'
import { setAccessToken, setUnauthorizedHandler, getMe } from '@/lib/api-client'

interface User {
  id: string
  email: string
  name: string
}

interface AuthState {
  user: User | null
  loading: boolean
  setUser: (user: User | null, token?: string) => void
  restoreSession: () => Promise<void>
  clear: () => void
}

// Dedup: beberapa caller (AuthGuard + halaman) bisa memanggil restoreSession
// bersamaan — jalankan sekali saja, yang lain menunggu promise yang sama.
// Restore version: monotonically incrementing ID supaya request lama (stale)
// tidak bisa overwrite state setelah clear() atau restoreSession baru.
let pendingRestore: Promise<void> | null = null
let restoreVersion = 0

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  loading: true,
  setUser: (user, token) => {
    if (token) setAccessToken(token)
    set({ user, loading: false })
  },
  restoreSession: () => {
    if (pendingRestore) return pendingRestore
    const version = ++restoreVersion
    pendingRestore = (async () => {
      try {
        const data = await getMe()
        // Stale: clear() or another restoreSession() was called while we were
        // fetching. Do not overwrite the newer state.
        if (version !== restoreVersion) return
        set({ user: data, loading: false })
      } catch {
        if (version !== restoreVersion) return
        set({ user: null, loading: false })
      } finally {
        if (version === restoreVersion) {
          pendingRestore = null
        }
      }
    })()
    return pendingRestore
  },
  clear: () => {
    setAccessToken(null)
    pendingRestore = null
    restoreVersion++
    set({ user: null, loading: false })
  },
}))

// Daftarkan global handler session-expired: saat refresh token gagal (sesi
// mati di server, mis. DB di-reset / refresh token di-revoke), semua halaman
// yang sudah login harus langsung logout — tidak boleh terus polling/
// reconnect 401 dalam loop.
setUnauthorizedHandler(() => {
  useAuthStore.getState().clear()
})
