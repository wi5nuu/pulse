'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { AuthGuard } from '@/components/auth-guard'
import { useAuthStore } from '@/store/auth'
import { apiGet, apiPost } from '@/lib/api-client'
import toast from 'react-hot-toast'

interface PendingInvite {
  id: string
  workspaceId: string
  workspaceName: string
  role: string
  token: string
  invitedByName: string | null
  expiresAt: string
  createdAt: string
}

export default function InvitesPage() {
  const router = useRouter()
  const user = useAuthStore((s) => s.user)
  const [invites, setInvites] = useState<PendingInvite[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [processingId, setProcessingId] = useState<string | null>(null)

  const fetchInvites = async () => {
    try {
      setLoadError('')
      const data = await apiGet<{ invites: PendingInvite[] }>('/invites/pending')
      setInvites(data.invites || [])
    } catch (err: any) {
      // FIX: gagal fetch tidak boleh tampil sebagai "No Pending Invitations".
      setLoadError(err.message || 'Failed to load invitations')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchInvites()
  }, [])

  const handleAccept = async (invite: PendingInvite) => {
    setProcessingId(invite.id)
    try {
      await apiPost(`/invites/${invite.token}/accept`)
      toast.success('Joined workspace!')
      router.push(`/w/${invite.workspaceId}`)
    } catch (err: any) {
      toast.error(err.message)
      setProcessingId(null)
    }
  }

  const handleReject = async (invite: PendingInvite) => {
    setProcessingId(invite.id)
    try {
      await apiPost(`/invites/${invite.token}/reject`)
      toast.success('Invitation rejected')
      setInvites((prev) => prev.filter((i) => i.id !== invite.id))
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setProcessingId(null)
    }
  }

  const formatDate = (dateString: string) => {
    const date = new Date(dateString)
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
  }

  return (
    <AuthGuard>
      <div className="min-h-screen bg-gray-50">
        <header className="border-b bg-white px-6 py-3 flex items-center justify-between">
          <div className="flex items-center gap-4">
            <button
              onClick={() => router.push('/dashboard')}
              className="text-gray-500 hover:text-gray-700"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
              </svg>
            </button>
            <h1 className="text-lg font-semibold">Workspace Invitations</h1>
          </div>
          <span className="text-sm text-gray-500">{user?.email}</span>
        </header>

        <main className="max-w-4xl mx-auto px-6 py-8">
          {loading ? (
            <div className="space-y-3">
              {[1, 2, 3].map((i) => (
                <div key={i} className="skeleton h-24 w-full" />
              ))}
            </div>
          ) : loadError ? (
            <div className="card text-center py-12 border-red-200">
              <p className="text-red-600 mb-2">{loadError}</p>
              <button className="btn-secondary" onClick={fetchInvites}>
                Retry
              </button>
            </div>
          ) : invites.length === 0 ? (
            <div className="card text-center py-12">
              <svg
                className="w-16 h-16 mx-auto text-gray-300 mb-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
                />
              </svg>
              <h2 className="text-lg font-semibold text-gray-700 mb-2">No Pending Invitations</h2>
              <p className="text-sm text-gray-500 mb-4">
                You don&apos;t have any pending workspace invitations at the moment.
              </p>
              <button className="btn-primary" onClick={() => router.push('/dashboard')}>
                Go to Dashboard
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {invites.map((invite) => (
                <div key={invite.id} className="card hover:border-accent-300 transition-colors">
                  <div className="flex items-start justify-between">
                    <div className="flex-1">
                      <h3 className="font-semibold text-lg mb-1">{invite.workspaceName}</h3>
                      <div className="space-y-1 text-sm text-gray-600">
                        {invite.invitedByName && (
                          <p>
                            Invited by <strong>{invite.invitedByName}</strong>
                          </p>
                        )}
                        <p>
                          Role: <strong className="capitalize">{invite.role}</strong>
                        </p>
                        <p className="text-xs text-gray-500">
                          Received on {formatDate(invite.createdAt)} • Expires on{' '}
                          {formatDate(invite.expiresAt)}
                        </p>
                      </div>
                    </div>
                    <div className="flex gap-2 ml-4">
                      <button
                        className="btn-ghost"
                        onClick={() => handleReject(invite)}
                        disabled={processingId === invite.id}
                      >
                        {processingId === invite.id ? 'Processing...' : 'Decline'}
                      </button>
                      <button
                        className="btn-primary"
                        onClick={() => handleAccept(invite)}
                        disabled={processingId === invite.id}
                      >
                        {processingId === invite.id ? 'Processing...' : 'Accept'}
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </main>
      </div>
    </AuthGuard>
  )
}
