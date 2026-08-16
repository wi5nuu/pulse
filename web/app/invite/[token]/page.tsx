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
  const [rejecting, setRejecting] = useState(false)

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

  const handleReject = async () => {
    setRejecting(true)
    try {
      await apiPost(`/invites/${token}/reject`)
      toast.success('Invitation rejected')
      router.push('/dashboard')
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setRejecting(false)
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
          <p className="text-sm text-gray-500 mb-4">This invitation has already been accepted.</p>
          <button className="btn-primary" onClick={() => router.push(`/w/${invite.workspaceId}`)}>
            Go to Workspace
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
          <h1 className="text-xl font-semibold mb-2">Sign In Required</h1>
          <p className="text-sm text-gray-500 mb-4">
            You need to be signed in to accept this invitation.
          </p>
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
        <h1 className="text-xl font-semibold mb-2">You&apos;re Invited!</h1>
        <p className="text-sm text-gray-600 mb-1">
          Join <strong>{invite.workspaceName}</strong>
        </p>
        {invite.invitedByName && (
          <p className="text-sm text-gray-500 mb-4">
            Invited by <strong>{invite.invitedByName}</strong> as <strong>{invite.role}</strong>
          </p>
        )}
        <div className="flex gap-2 mt-4">
          <button 
            className="btn-ghost flex-1" 
            onClick={handleReject} 
            disabled={rejecting || accepting}
          >
            {rejecting ? 'Rejecting...' : 'Decline'}
          </button>
          <button 
            className="btn-primary flex-1" 
            onClick={handleAccept} 
            disabled={accepting || rejecting}
          >
            {accepting ? 'Joining...' : 'Accept Invite'}
          </button>
        </div>
      </div>
    </div>
  )
}
