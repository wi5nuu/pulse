'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import { apiGet, apiPost } from '@/lib/api-client'
import { useAuthStore } from '@/store/auth'
import toast from 'react-hot-toast'

interface InviteData {
  workspaceId: string
  workspaceName: string
  role: string
  email: string
  accepted: boolean
  invitedByName: string | null
  expiresAt: string
}

export default function InvitePage() {
  const params = useParams()
  const router = useRouter()
  const token = params.token as string
  const { user, loading, restoreSession } = useAuthStore()
  const [invite, setInvite] = useState<InviteData | null>(null)
  const [fetching, setFetching] = useState(true)
  const [error, setError] = useState('')
  const [accepting, setAccepting] = useState(false)

  useEffect(() => {
    restoreSession()
  }, [restoreSession])

  useEffect(() => {
    async function load() {
      try {
        const data = await apiGet<{ invite: InviteData }>(`/invites/${token}`)
        setInvite(data.invite)
      } catch (err: any) {
        setError(err.message || 'Invite not found or expired')
      } finally {
        setFetching(false)
      }
    }
    load()
  }, [token])

  const handleAccept = async () => {
    setAccepting(true)
    try {
      await apiPost(`/invites/${token}/accept`)
      toast.success('Joined workspace!')
      router.push(`/w/${invite!.workspaceId}`)
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setAccepting(false)
    }
  }

  if (fetching) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="skeleton h-32 w-96" />
      </div>
    )
  }

  if (error || !invite) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="card max-w-sm w-full text-center">
          <h1 className="text-xl font-semibold mb-2">Invite Not Found</h1>
          <p className="text-sm text-gray-500 mb-4">{error || 'This invite link is invalid or has expired.'}</p>
          <button className="btn-primary" onClick={() => router.push('/dashboard')}>
            Go to Dashboard
          </button>
        </div>
      </div>
    )
  }

  if (invite.accepted) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="card max-w-sm w-full text-center">
          <h1 className="text-xl font-semibold mb-2">Already Accepted</h1>
          <p className="text-sm text-gray-500 mb-4">This invite has already been used.</p>
          <button className="btn-primary" onClick={() => router.push('/dashboard')}>
            Go to Dashboard
          </button>
        </div>
      </div>
    )
  }

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="skeleton h-32 w-96" />
      </div>
    )
  }

  if (!user) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="card max-w-sm w-full text-center">
          <h1 className="text-xl font-semibold mb-2">You're Invited!</h1>
          <p className="text-sm text-gray-600 mb-1">
            <strong>{invite.workspaceName}</strong>
          </p>
          {invite.invitedByName && (
            <p className="text-sm text-gray-500 mb-4">
              Invited by <strong>{invite.invitedByName}</strong> as <strong>{invite.role}</strong>
            </p>
          )}
          <p className="text-sm text-gray-400 mb-4">Sign in to accept this invitation.</p>
          <button
            className="btn-primary"
            onClick={() => router.push(`/login?redirect=/invite/${token}`)}
          >
            Sign in to Accept
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="card max-w-sm w-full text-center">
        <h1 className="text-xl font-semibold mb-2">You're Invited!</h1>
        <p className="text-sm text-gray-600 mb-1">
          Join <strong>{invite.workspaceName}</strong>
        </p>
        {invite.invitedByName && (
          <p className="text-sm text-gray-500 mb-4">
            Invited by <strong>{invite.invitedByName}</strong> as <strong>{invite.role}</strong>
          </p>
        )}
        <button className="btn-primary w-full" onClick={handleAccept} disabled={accepting}>
          {accepting ? 'Joining...' : 'Accept Invite'}
        </button>
      </div>
    </div>
  )
}
