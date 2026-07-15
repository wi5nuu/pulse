'use client'

import { useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import { apiGet, apiPost, apiPatch, apiDelete } from '@/lib/api-client'
import toast from 'react-hot-toast'

interface Member {
  userId: string
  name: string
  email: string
  role: string
}

export default function WorkspaceSettingsPage() {
  const params = useParams()
  const workspaceId = params.workspaceId as string
  const [members, setMembers] = useState<Member[]>([])
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<'editor' | 'viewer'>('editor')
  const [loading, setLoading] = useState(true)
  const [removeConfirmId, setRemoveConfirmId] = useState<string | null>(null)
  const [removing, setRemoving] = useState(false)

  useEffect(() => {
    apiGet<{ members: Member[] }>(`/api/workspaces/${workspaceId}/members`)
      .then((data) => setMembers(data.members))
      .catch(() => toast.error('Failed to load members'))
      .finally(() => setLoading(false))
  }, [workspaceId])

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

  return (
    <div className="flex-1 overflow-y-auto px-8 py-6 max-w-2xl">
      <h2 className="text-lg font-semibold mb-4">Workspace Settings</h2>

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
              <button className="btn-secondary" onClick={() => setRemoveConfirmId(null)}>Cancel</button>
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
