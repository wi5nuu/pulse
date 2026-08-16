'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter, usePathname } from 'next/navigation'
import { AuthGuard } from '@/components/auth-guard'
import { useAuthStore } from '@/store/auth'
import { apiGet, apiPost, logout } from '@/lib/api-client'
import { DocSidebarItem } from '@/components/doc-sidebar-item'
import toast from 'react-hot-toast'

interface WorkspaceData {
  id: string
  name: string
}

interface DocSummary {
  id: string
  title: string
}

interface BoardSummary {
  id: string
  name: string
}

export default function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  const params = useParams()
  const router = useRouter()
  const pathname = usePathname()
  const workspaceId = params.workspaceId as string
  const clear = useAuthStore((s) => s.clear)

  const [ws, setWs] = useState<WorkspaceData | null>(null)
  const [docs, setDocs] = useState<DocSummary[]>([])
  const [boards, setBoards] = useState<BoardSummary[]>([])
  const [sharedDocs, setSharedDocs] = useState<DocSummary[]>([])

  useEffect(() => {
    if (!workspaceId) return
    apiGet<{ workspace: WorkspaceData }>(`/api/workspaces/${workspaceId}`)
      .then((data) => setWs(data.workspace))
      .catch(() => toast.error('Failed to load workspace'))

    apiGet<{ documents: DocSummary[] }>(`/api/workspaces/${workspaceId}/documents`)
      .then((data) => setDocs(data.documents))
      .catch(() => {})

    apiGet<{ boards: BoardSummary[] }>(`/api/workspaces/${workspaceId}/boards`)
      .then((data) => setBoards(data.boards))
      .catch(() => {})

    apiGet<{ documents: DocSummary[] }>('/api/documents/shared')
      .then((data) => setSharedDocs(data.documents))
      .catch(() => {})
  }, [workspaceId])

  const handleNewDoc = async () => {
    try {
      const data = await apiPost<{ document: DocSummary }>(
        `/api/workspaces/${workspaceId}/documents`,
        { title: 'Untitled' },
      )
      setDocs((prev) => [...prev, data.document])
      router.push(`/w/${workspaceId}/doc/${data.document.id}`)
    } catch (err: any) {
      toast.error(err.message)
    }
  }

  const handleNewBoard = async () => {
    try {
      const data = await apiPost<{ board: BoardSummary }>(
        `/api/workspaces/${workspaceId}/boards`,
        { name: 'New Board' },
      )
      setBoards((prev) => [...prev, data.board])
      router.push(`/w/${workspaceId}/board/${data.board.id}`)
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
      <div className="flex h-screen">
        <aside className="w-64 border-r bg-gray-50 flex flex-col">
          <div className="p-4 border-b">
            <div className="text-xs text-gray-400 mb-1">Workspace</div>
            <div className="font-semibold truncate">{ws?.name || '...'}</div>
          </div>

          <div className="flex-1 overflow-y-auto p-3 space-y-4">
            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Documents
                </span>
                <button className="text-xs text-accent-600 hover:underline" onClick={handleNewDoc}>
                  + New
                </button>
              </div>
              {docs.length === 0 && (
                <p className="text-xs text-gray-400 pl-2">No documents</p>
              )}
              {docs.map((d) => (
                <DocSidebarItem
                  key={d.id}
                  workspaceId={workspaceId}
                  doc={d}
                  onRenamed={(id, title) =>
                    setDocs((prev) => prev.map((x) => (x.id === id ? { ...x, title } : x)))
                  }
                />
              ))}
            </div>

            {sharedDocs.length > 0 && (
              <div>
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Shared with me
                  </span>
                </div>
                {sharedDocs.map((d) => (
                  <DocSidebarItem
                    key={d.id}
                    workspaceId={workspaceId}
                    doc={d}
                    onRenamed={(id, title) =>
                      setSharedDocs((prev) => prev.map((x) => (x.id === id ? { ...x, title } : x)))
                    }
                  />
                ))}
              </div>
            )}

            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Boards
                </span>
                <button className="text-xs text-accent-600 hover:underline" onClick={handleNewBoard}>
                  + New
                </button>
              </div>
              {boards.length === 0 && (
                <p className="text-xs text-gray-400 pl-2">No boards</p>
              )}
              {boards.map((b) => (
                <button
                  key={b.id}
                  className={`w-full text-left text-sm px-2 py-1.5 rounded hover:bg-gray-200 transition-colors truncate ${
                    pathname.includes(`/board/${b.id}`) ? 'bg-gray-200 font-medium' : ''
                  }`}
                  onClick={() => router.push(`/w/${workspaceId}/board/${b.id}`)}
                >
                  {b.name}
                </button>
              ))}
            </div>
          </div>

          <div className="p-3 border-t">
            <button
              className="w-full text-left text-sm px-2 py-1.5 rounded hover:bg-gray-200"
              onClick={() => router.push(`/w/${workspaceId}/settings`)}
            >
              Settings
            </button>
            <button
              className="w-full text-left text-sm px-2 py-1.5 rounded hover:bg-gray-200 text-gray-500"
              onClick={handleLogout}
            >
              Sign out
            </button>
          </div>
        </aside>
        <main className="flex-1 flex flex-col overflow-hidden">{children}</main>
      </div>
    </AuthGuard>
  )
}
