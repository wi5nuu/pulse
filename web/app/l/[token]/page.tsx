'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import { apiGet, getAccessToken } from '@/lib/api-client'
import { useAuthStore } from '@/store/auth'

// Halaman pembuka link share "Anyone with the link" (fiturwajibada H.168).
// Tanpa auth: resolve token → info dokumen. Dengan login: buka dokumen.
interface LinkInfo {
  documentId: string
  title: string
  workspaceId: string
  permission: 'view' | 'edit'
  expiresAt: string | null
}

export default function LinkSharePage() {
  const params = useParams()
  const router = useRouter()
  const token = params.token as string
  const user = useAuthStore((s) => s.user)
  const [info, setInfo] = useState<LinkInfo | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!token) return
    setInfo(null)
    setError('')
    const controller = new AbortController()
    apiGet<LinkInfo>(`/api/linkshare/${token}`)
      .then((data) => {
        if (!controller.signal.aborted) setInfo(data)
      })
      .catch((err: any) => {
        if (!controller.signal.aborted) setError(err.message ?? 'Link tidak valid atau sudah kadaluarsa')
      })
    return () => controller.abort()
  }, [token])

  const openDocument = () => {
    if (!info) return
    if (getAccessToken() && user) {
      router.push(`/w/${info.workspaceId}/doc/${info.documentId}?ls=${token}`)
    } else {
      router.push(`/login?redirect=${encodeURIComponent(`/l/${token}`)}`)
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center p-4">
      <div className="bg-white rounded-xl shadow-lg max-w-md w-full p-8 text-center">
        <div className="text-4xl mb-3">📄</div>
        <h1 className="text-xl font-semibold text-gray-900 mb-1">Shared document</h1>

        {error && (
          <div>
            <p className="text-sm text-red-600 mt-4">{error}</p>
            <a href="/login" className="inline-block mt-4 text-sm text-blue-600 hover:underline">
              Go to Pulse
            </a>
          </div>
        )}

        {info && (
          <div>
            <p className="text-sm text-gray-500 mt-2">“{info.title}”</p>
            <p className="text-xs text-gray-400 mt-1">
              {info.permission === 'edit' ? 'Anyone with the link can edit' : 'Anyone with the link can view'}
              {info.expiresAt ? ` · expires ${new Date(info.expiresAt).toLocaleDateString()}` : ''}
            </p>
            <button
              onClick={openDocument}
              className="mt-6 w-full px-4 py-2.5 bg-blue-600 text-white rounded-md hover:bg-blue-700"
            >
              {user ? 'Open document' : 'Login to open'}
            </button>
          </div>
        )}

        {!info && !error && <p className="text-sm text-gray-400 mt-4">Loading…</p>}
      </div>
    </div>
  )
}