'use client'

import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import { apiGet, apiPost, apiPatch, apiDelete } from '@/lib/api-client'
import toast from 'react-hot-toast'

interface Member {
  userId: string
  name: string
  email: string
  role: string
}

interface WorkspaceInvite {
  id: string
  workspaceId: string
  workspaceName: string
  email: string
  role: string
  token: string
  invitedByName: string | null
  accepted: boolean
  expiresAt: string
  createdAt: string
}

type TabType = 'members' | 'invites'

export default function WorkspaceSettingsPage() {
  const params = useParams()
  const workspaceId = params.workspaceId as string
  const [activeTab, setActiveTab] = useState<TabType>('members')
  const [members, setMembers] = useState<Member[]>([])
  const [invites, setInvites] = useState<WorkspaceInvite[]>([])
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<'editor' | 'viewer'>('editor')
  const [loading, setLoading] = useState(true)
  const [invitesLoading, setInvitesLoading] = useState(false)
  const [removeConfirmId, setRemoveConfirmId] = useState<string | null>(null)
  const [removing, setRemoving] = useState(false)
  const [deletingInviteId, setDeletingInviteId] = useState<string | null>(null)

  useEffect(() => {
    apiGet<{ members: Member[] }>(`/api/workspaces/${workspaceId}/members`)
      .then((data) => setMembers(data.members))
      .catch(() => toast.error('Failed to load members'))
      .finally(() => setLoading(false))
  }, [workspaceId])

  const loadInvites = useCallback(async () => {
    setInvitesLoading(true)
    try {
      const data = await apiGet<{ invites: WorkspaceInvite[] }>(`/api/workspaces/${workspaceId}/invites`)
      setInvites(data.invites || [])
    } catch {
      toast.error('Failed to load invites')
    } finally {
      setInvitesLoading(false)
    }
  }, [workspaceId])

  useEffect(() => {
    if (activeTab === 'invites') {
      loadInvites()
    }
  }, [activeTab, loadInvites])

  const handleInvite = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!inviteEmail.trim()) return
    try {
      const data = await apiPost<{ invite: { token: string } }>(`/api/workspaces/${workspaceId}/invites`, {
        email: inviteEmail,
        role: inviteRole,
      })
      const link = `${window.location.origin}/invite/${data.invite.token}`
      await navigator.clipboard.writeText(link)
      toast.success('Invite link copied to clipboard!')
      setInviteEmail('')
      // Reload invites if on invites tab
      if (activeTab === 'invites') {
        loadInvites()
      }
    } catch (err: any) {
      toast.error(err.message)
    }
  }

  const handleChangeRole = async (userId: string, role: string) => {
    try {
      await apiPatch(`/api/workspaces/${workspaceId}/members/${userId}`, { role })
      setMembers((prev) => prev.map((m) => (m.userId === userId ? { ...m, role } : m)))
      toast.success('Role updated')
    } catch (err: any) {
      toast.error(err.message)
    }
  }

  const handleRemove = async () => {
    if (!removeConfirmId) return
    setRemoving(true)
    try {
      await apiDelete(`/api/workspaces/${workspaceId}/members/${removeConfirmId}`)
      setMembers((prev) => prev.filter((m) => m.userId !== removeConfirmId))
      toast.success('Member removed')
      setRemoveConfirmId(null)
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setRemoving(false)
    }
  }

  const handleDeleteInvite = async (inviteId: string) => {
    setDeletingInviteId(inviteId)
    try {
      await apiDelete(`/api/workspaces/${workspaceId}/invites/${inviteId}`)
      setInvites((prev) => prev.filter((i) => i.id !== inviteId))
      toast.success('Invitation cancelled')
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setDeletingInviteId(null)
    }
  }

  const handleCopyInviteLink = async (token: string) => {
    const link = `${window.location.origin}/invite/${token}`
    await navigator.clipboard.writeText(link)
    toast.success('Invite link copied to clipboard!')
  }

  const formatDate = (dateString: string) => {
    const date = new Date(dateString)
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
  }

  const pendingInvites = invites.filter((i) => !i.accepted && new Date(i.expiresAt) > new Date())
  const expiredInvites = invites.filter((i) => !i.accepted && new Date(i.expiresAt) <= new Date())
  const acceptedInvites = invites.filter((i) => i.accepted)

  return (
    <div className="flex-1 overflow-y-auto px-8 py-6 max-w-2xl">
      <h2 className="text-lg font-semibold mb-4">Workspace Settings</h2>

      {/* Tabs */}
      <div className="flex gap-1 mb-6 border-b">
        <button
          className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
            activeTab === 'members'
              ? 'border-accent-600 text-accent-600'
              : 'border-transparent text-gray-500 hover:text-gray-700'
          }`}
          onClick={() => setActiveTab('members')}
        >
          Members
        </button>
        <button
          className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
            activeTab === 'invites'
              ? 'border-accent-600 text-accent-600'
              : 'border-transparent text-gray-500 hover:text-gray-700'
          }`}
          onClick={() => setActiveTab('invites')}
        >
          Invitations
        </button>
      </div>

      {/* Invite form - shown on both tabs */}
      <div className="card mb-6">
        <h3 className="text-sm font-medium mb-3">Invite Member</h3>
        <form onSubmit={handleInvite} className="flex gap-2">
          <input
            className="input flex-1"
            type="email"
            placeholder="Email address"
            value={inviteEmail}
            onChange={(e) => setInviteEmail(e.target.value)}
            required
          />
          <select
            className="input w-28"
            value={inviteRole}
            onChange={(e) => setInviteRole(e.target.value as 'editor' | 'viewer')}
          >
            <option value="editor">Editor</option>
            <option value="viewer">Viewer</option>
          </select>
          <button className="btn-primary" type="submit">
            Invite
          </button>
        </form>
      </div>

      {/* Members Tab */}
      {activeTab === 'members' && (
        <div className="card">
          <h3 className="text-sm font-medium mb-3">Members ({members.length})</h3>
          {loading ? (
            <div className="space-y-2">
              {[1, 2].map((i) => (
                <div key={i} className="skeleton h-12 w-full" />
              ))}
            </div>
          ) : members.length === 0 ? (
            <p className="text-sm text-gray-400">No members</p>
          ) : (
            <div className="space-y-2">
              {members.map((m) => (
                <div key={m.userId} className="flex items-center justify-between py-2 border-b last:border-0">
                  <div>
                    <div className="text-sm font-medium">{m.name}</div>
                    <div className="text-xs text-gray-400">{m.email}</div>
                  </div>
                  <div className="flex items-center gap-2">
                    <select
                      className="text-xs border rounded px-2 py-1"
                      value={m.role}
                      onChange={(e) => handleChangeRole(m.userId, e.target.value)}
                      disabled={m.role === 'owner'}
                    >
                      <option value="owner" disabled>
                        Owner
                      </option>
                      <option value="editor">Editor</option>
                      <option value="viewer">Viewer</option>
                    </select>
                    {m.role !== 'owner' && (
                      <button
                        className="text-xs text-red-500 hover:underline"
                        onClick={() => setRemoveConfirmId(m.userId)}
                      >
                        Remove
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Invites Tab */}
      {activeTab === 'invites' && (
        <div className="space-y-6">
          {/* Pending Invites */}
          <div className="card">
            <h3 className="text-sm font-medium mb-3">Pending Invitations ({pendingInvites.length})</h3>
            {invitesLoading ? (
              <div className="space-y-2">
                {[1, 2].map((i) => (
                  <div key={i} className="skeleton h-16 w-full" />
                ))}
              </div>
            ) : pendingInvites.length === 0 ? (
              <p className="text-sm text-gray-400">No pending invitations</p>
            ) : (
              <div className="space-y-3">
                {pendingInvites.map((invite) => (
                  <div key={invite.id} className="flex items-start justify-between py-2 border-b last:border-0">
                    <div className="flex-1">
                      <div className="text-sm font-medium">{invite.email}</div>
                      <div className="text-xs text-gray-500">
                        Role: <span className="capitalize">{invite.role}</span> • Sent on{' '}
                        {formatDate(invite.createdAt)}
                      </div>
                      <div className="text-xs text-gray-400">Expires on {formatDate(invite.expiresAt)}</div>
                    </div>
                    <div className="flex gap-2 ml-4">
                      <button
                        className="text-xs text-accent-600 hover:underline"
                        onClick={() => handleCopyInviteLink(invite.token)}
                      >
                        Copy Link
                      </button>
                      <button
                        className="text-xs text-red-500 hover:underline"
                        onClick={() => handleDeleteInvite(invite.id)}
                        disabled={deletingInviteId === invite.id}
                      >
                        {deletingInviteId === invite.id ? 'Cancelling...' : 'Cancel'}
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Accepted Invites */}
          {acceptedInvites.length > 0 && (
            <div className="card">
              <h3 className="text-sm font-medium mb-3">Accepted Invitations ({acceptedInvites.length})</h3>
              <div className="space-y-2">
                {acceptedInvites.map((invite) => (
                  <div key={invite.id} className="flex items-start justify-between py-2 border-b last:border-0">
                    <div>
                      <div className="text-sm font-medium">{invite.email}</div>
                      <div className="text-xs text-gray-500">
                        Role: <span className="capitalize">{invite.role}</span> • Accepted
                      </div>
                    </div>
                    <span className="text-xs text-green-600 font-medium">Joined</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Expired Invites */}
          {expiredInvites.length > 0 && (
            <div className="card">
              <h3 className="text-sm font-medium mb-3">Expired Invitations ({expiredInvites.length})</h3>
              <div className="space-y-2">
                {expiredInvites.map((invite) => (
                  <div key={invite.id} className="flex items-start justify-between py-2 border-b last:border-0">
                    <div>
                      <div className="text-sm font-medium text-gray-400">{invite.email}</div>
                      <div className="text-xs text-gray-400">
                        Role: <span className="capitalize">{invite.role}</span> • Expired on{' '}
                        {formatDate(invite.expiresAt)}
                      </div>
                    </div>
                    <button
                      className="text-xs text-red-500 hover:underline"
                      onClick={() => handleDeleteInvite(invite.id)}
                      disabled={deletingInviteId === invite.id}
                    >
                      {deletingInviteId === invite.id ? 'Removing...' : 'Remove'}
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Remove confirmation modal */}
      {removeConfirmId !== null && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
          onClick={() => setRemoveConfirmId(null)}
        >
          <div
            className="bg-white rounded shadow-lg w-full max-w-sm p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 className="text-base font-semibold mb-3">Remove Member</h3>
            <p className="text-sm text-gray-600 mb-4">
              Are you sure you want to remove this member from the workspace?
            </p>
            <div className="flex justify-end gap-2">
              <button className="btn-secondary" onClick={() => setRemoveConfirmId(null)}>
                Cancel
              </button>
              <button className="btn-danger" onClick={handleRemove} disabled={removing}>
                {removing ? 'Removing...' : 'Remove'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
