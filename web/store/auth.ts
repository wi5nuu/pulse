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
let pendingRestore: Promise<void> | null = null

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  loading: true,
  setUser: (user, token) => {
    if (token) setAccessToken(token)
    set({ user, loading: false })
  },
  restoreSession: () => {
    if (pendingRestore) return pendingRestore
    pendingRestore = (async () => {
      try {
        const data = await getMe()
        set({ user: data, loading: false })
      } catch {
        set({ user: null, loading: false })
      } finally {
        pendingRestore = null
      }
    })()
    return pendingRestore
  },
  clear: () => {
    setAccessToken(null)
    set({ user: null })
  },
}))

// Daftarkan global handler session-expired: saat refresh token gagal (sesi
// mati di server, mis. DB di-reset / refresh token di-revoke), semua halaman
// yang sudah login harus langsung logout — tidak boleh terus polling/
// reconnect 401 dalam loop.
setUnauthorizedHandler(() => {
  useAuthStore.getState().clear()
})
