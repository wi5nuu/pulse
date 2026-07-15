import { create } from 'zustand'
import { setAccessToken, getMe } from '@/lib/api-client'

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

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  loading: true,
  setUser: (user, token) => {
    if (token) setAccessToken(token)
    set({ user, loading: false })
  },
  restoreSession: async () => {
    try {
      const data = await getMe()
      set({ user: data, loading: false })
    } catch {
      set({ user: null, loading: false })
    }
  },
  clear: () => {
    setAccessToken(null)
    set({ user: null })
  },
}))
