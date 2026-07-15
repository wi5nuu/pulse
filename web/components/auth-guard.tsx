'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useAuthStore } from '@/store/auth'

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const { user, loading, restoreSession } = useAuthStore()

  useEffect(() => {
    if (!user && loading) {
      restoreSession()
    }
  }, [user, loading, restoreSession])

  useEffect(() => {
    if (!loading && !user) {
      router.push('/login')
    }
  }, [loading, user, router])

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="skeleton h-8 w-32" />
      </div>
    )
  }

  if (!user) return null

  return <>{children}</>
}
