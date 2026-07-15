'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useAuthStore } from '@/store/auth'

export default function HomePage() {
  const router = useRouter()
  const { user, loading, restoreSession } = useAuthStore()

  useEffect(() => {
    restoreSession()
  }, [restoreSession])

  useEffect(() => {
    if (!loading) {
      if (user) {
        router.replace('/dashboard')
      } else {
        router.replace('/login')
      }
    }
  }, [user, loading, router])

  return (
    <div className="min-h-screen flex items-center justify-center">
      <div className="skeleton w-8 h-8 rounded-full" />
    </div>
  )
}
