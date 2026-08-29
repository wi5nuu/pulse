'use client'

import { useEffect, useRef, useState } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { apiPatch } from '@/lib/api-client'
import toast from 'react-hot-toast'

interface DocSidebarItemProps {
  workspaceId: string
  doc: { id: string; title: string }
  canRename?: boolean
  onRenamed?: (id: string, title: string) => void
}

export function DocSidebarItem({ workspaceId, doc, canRename = true, onRenamed }: DocSidebarItemProps) {
  const router = useRouter()
  const pathname = usePathname()
  const [editing, setEditing] = useState(false)
  const [title, setTitle] = useState(doc.title)
  const [saving, setSaving] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing) {
      inputRef.current?.focus()
      inputRef.current?.select()
    }
  }, [editing])

  const active = pathname.includes(`/doc/${doc.id}`)

  const startEdit = () => {
    setTitle(doc.title)
    setEditing(true)
  }

  const cancelEdit = () => {
    setEditing(false)
    setTitle(doc.title)
  }

  const saveRename = async () => {
    const trimmed = title.trim()
    if (!trimmed) {
      cancelEdit()
      return
    }
    if (trimmed === doc.title) {
      setEditing(false)
      return
    }
    setSaving(true)
    try {
      await apiPatch(`/api/documents/${doc.id}`, { title: trimmed })
      setEditing(false)
      onRenamed?.(doc.id, trimmed)
      toast.success('Document renamed')
    } catch (err: any) {
      toast.error(err.message || 'Failed to rename')
      cancelEdit()
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className={`group flex items-center w-full text-left text-sm rounded hover:bg-gray-200 transition-colors ${
        active ? 'bg-gray-200 font-medium' : ''
      }`}
    >
      {editing ? (
        <input
          ref={inputRef}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          onBlur={saveRename}
          onKeyDown={(e) => {
            if (e.key === 'Enter') saveRename()
            if (e.key === 'Escape') cancelEdit()
          }}
          disabled={saving}
          className="flex-1 min-w-0 px-2 py-1.5 bg-white border border-blue-400 rounded focus:outline-none text-sm"
        />
      ) : (
        <button
          className="flex-1 min-w-0 px-2 py-1.5 truncate text-left"
          onClick={() => router.push(`/w/${workspaceId}/doc/${doc.id}`)}
          onDoubleClick={startEdit}
          title={doc.title}
        >
          {doc.title}
        </button>
      )}
      {canRename && !editing && (
        <button
          className="px-1.5 py-1 text-gray-400 hover:text-gray-700 opacity-0 group-hover:opacity-100 transition-opacity"
          onClick={startEdit}
          title="Rename"
        >
          <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
            />
          </svg>
        </button>
      )}
    </div>
  )
}