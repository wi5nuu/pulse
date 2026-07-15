'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { AuthGuard } from '@/components/auth-guard'
import { useAuthStore } from '@/store/auth'
import { apiGet, apiPost, logout } from '@/lib/api-client'
import toast from 'react-hot-toast'

interface Workspace {
  id: string
  name: string
  slug: string
}

export default function DashboardPage() {
  const router = useRouter()
  const user = useAuthStore((s) => s.user)
  const clear = useAuthStore((s) => s.clear)
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState('')

  useEffect(() => {
    apiGet<{ workspaces: Workspace[] }>('/api/workspaces')
      .then((data) => setWorkspaces(data.workspaces))
      .catch(() => toast.error('Failed to load workspaces'))
      .finally(() => setLoading(false))
  }, [])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      const data = await apiPost<{ workspace: Workspace }>('/api/workspaces', { name: newName })
      setWorkspaces((prev) => [...prev, data.workspace])
      setNewName('')
      setShowCreate(false)
      toast.success('Workspace created')
    } catch (err: any) {
      toast.error(err.message)
    }
  }

  const handleLogout = async () => {
    await logout()
    clear()
    router.push('/login')
  }

  return (
    <AuthGuard>
      <div className="min-h-screen bg-gray-50">
        <header className="border-b bg-white px-6 py-3 flex items-center justify-between">
          <h1 className="text-lg font-semibold">Pulse</h1>
          <div className="flex items-center gap-3">
            <span className="text-sm text-gray-500">{user?.email}</span>
            <button className="btn-ghost text-sm" onClick={handleLogout}>
              Sign out
            </button>
          </div>
        </header>
        <main className="max-w-3xl mx-auto px-6 py-8">
          <div className="flex items-center justify-between mb-6">
            <h2 className="text-xl font-semibold">Workspaces</h2>
            <button className="btn-primary" onClick={() => setShowCreate(true)}>
              New workspace
            </button>
          </div>

          {showCreate && (
            <form onSubmit={handleCreate} className="card mb-4 flex gap-2">
              <input
                className="input flex-1"
                placeholder="Workspace name..."
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                required
                autoFocus
              />
              <button className="btn-primary" type="submit">
                Create
              </button>
              <button className="btn-secondary" type="button" onClick={() => setShowCreate(false)}>
                Cancel
              </button>
            </form>
          )}

          {loading ? (
            <div className="space-y-2">
              {[1, 2, 3].map((i) => (
                <div key={i} className="skeleton h-16 w-full" />
              ))}
            </div>
          ) : workspaces.length === 0 ? (
            <div className="card text-center py-12">
              <p className="text-gray-500 mb-2">No workspaces yet</p>
              <button className="btn-primary" onClick={() => setShowCreate(true)}>
                Create your first workspace
              </button>
            </div>
          ) : (
            <div className="space-y-2">
              {workspaces.map((ws) => (
                <button
                  key={ws.id}
                  className="card w-full text-left hover:border-accent-300 transition-colors"
                  onClick={() => router.push(`/w/${ws.id}`)}
                >
                  <div className="font-medium">{ws.name}</div>
                  <div className="text-xs text-gray-400">{ws.slug}</div>
                </button>
              ))}
            </div>
          )}
        </main>
      </div>
    </AuthGuard>
  )
}
