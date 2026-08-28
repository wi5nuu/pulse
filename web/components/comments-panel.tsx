'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { EditorView } from 'prosemirror-view'
import toast from 'react-hot-toast'
import { apiGet, apiPost, apiPatch, apiDelete } from '@/lib/api-client'
import type { PulseWSProvider } from '@/lib/yjs-provider'
import { textAtRange } from '@/lib/editor/commands'

export interface CommentItem {
  id: string
  documentId: string
  authorId: string
  authorName: string
  authorEmail: string
  anchor: string
  body: string
  parentId: string | null
  resolved: boolean
  createdAt: string
  updatedAt: string
}

interface Props {
  docId: string
  view: EditorView | null
  userId: string
  readOnly: boolean
  provider: PulseWSProvider | null
  onClose: () => void
}

function parseAnchor(anchor: string): { from: number; to: number } | null {
  try {
    const a = JSON.parse(anchor)
    if (typeof a.from === 'number' && typeof a.to === 'number') return a
  } catch {
    /* ignore */
  }
  return null
}

export default function CommentsPanel({ docId, view, userId, readOnly, provider, onClose }: Props) {
  const [comments, setComments] = useState<CommentItem[]>([])
  const [loading, setLoading] = useState(true)
  const [composing, setComposing] = useState(false)
  const [draft, setDraft] = useState('')
  const [replyDrafts, setReplyDrafts] = useState<Record<string, string>>({})
  const [replyingTo, setReplyingTo] = useState<string | null>(null)
  const loaded = useRef(false)

  const load = useCallback(async () => {
    try {
      const data = await apiGet<{ comments: CommentItem[] }>(`/api/documents/${docId}/comments`)
      setComments(data.comments ?? [])
    } catch {
      toast.error('Gagal memuat komentar')
    } finally {
      setLoading(false)
    }
  }, [docId])

  useEffect(() => {
    if (loaded.current) return
    loaded.current = true
    void load()
  }, [load])

  // Realtime: event komentar dari kolaborator lain via WS (MsgDocEvent).
  useEffect(() => {
    if (!provider) return
    return provider.onDocEvent((payload: string) => {
      try {
        const evt = JSON.parse(payload) as { event: string; comment: CommentItem }
        if (!evt.comment?.documentId || evt.comment.documentId !== docId) return
        setComments((prev) => {
          if (evt.event === 'comment-deleted') {
            return prev.filter((c) => c.id !== evt.comment.id && c.parentId !== evt.comment.id)
          }
          const exists = prev.some((c) => c.id === evt.comment.id)
          if (exists) return prev.map((c) => (c.id === evt.comment.id ? evt.comment : c))
          return [...prev, evt.comment]
        })
      } catch {
        /* payload bukan JSON komentar — abaikan */
      }
    })
  }, [provider, docId])

  const selectionSnippet = (() => {
    if (!view) return null
    const { from, to, empty } = view.state.selection
    if (empty) return null
    return textAtRange(view.state, from, to)
  })()

  const startComment = () => {
    if (!view || !selectionSnippet) {
      toast.error('Select some text first to comment')
      return
    }
    setComposing(true)
  }

  const submitComment = async () => {
    if (!view || !draft.trim()) return
    const { from, to } = view.state.selection
    try {
      await apiPost<{ comment: CommentItem }>(`/api/documents/${docId}/comments`, {
        anchor: JSON.stringify({ from, to }),
        body: draft.trim(),
      })
      setDraft('')
      setComposing(false)
      void load()
    } catch (err: any) {
      toast.error(err.message ?? 'Gagal menambah komentar')
    }
  }

  const submitReply = async (parentId: string) => {
    const body = (replyDrafts[parentId] ?? '').trim()
    if (!body) return
    const parentComment = comments.find(c => c.id === parentId)
    if (!parentComment) {
      toast.error('Parent comment not found')
      return
    }
    try {
      await apiPost<{ comment: CommentItem }>(`/api/documents/${docId}/comments`, {
        anchor: parentComment.anchor,
        body,
        parentId,
      })
      setReplyDrafts((d) => ({ ...d, [parentId]: '' }))
      setReplyingTo(null)
      void load()
    } catch (err: any) {
      toast.error(err.message ?? 'Gagal membalas')
    }
  }

  const toggleResolve = async (c: CommentItem) => {
    try {
      await apiPatch<{ comment: CommentItem }>(`/api/documents/${docId}/comments/${c.id}`, {
        resolved: !c.resolved,
      })
      void load()
    } catch (err: any) {
      toast.error(err.message ?? 'Gagal update komentar')
    }
  }

  const removeComment = async (c: CommentItem) => {
    if (!window.confirm('Hapus komentar ini?')) return
    try {
      await apiDelete(`/api/documents/${docId}/comments/${c.id}`)
      void load()
    } catch (err: any) {
      toast.error(err.message ?? 'Gagal hapus komentar')
    }
  }

  const roots = comments.filter((c) => !c.parentId)
  const repliesOf = (id: string) => comments.filter((c) => c.parentId === id)

  return (
    <div className="w-80 border-l bg-white flex flex-col">
      <div className="px-4 py-3 border-b flex items-center justify-between">
        <span className="text-sm font-medium">Comments</span>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-sm px-1" title="Close">✕</button>
      </div>

      <div className="px-4 py-2 border-b">
        <button
          onClick={startComment}
          disabled={readOnly || composing}
          className="w-full py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-40"
        >
          {selectionSnippet ? '+ Comment on selection' : '+ Select text to comment'}
        </button>
        {composing && (
          <div className="mt-2">
            <div className="text-[11px] text-gray-500 mb-1 bg-gray-100 rounded px-2 py-1 line-clamp-2">“{selectionSnippet}”</div>
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              rows={3}
              autoFocus
              placeholder="Write a comment…"
              className="w-full text-sm border rounded p-2 resize-none"
            />
            <div className="flex gap-2 mt-1">
              <button onClick={submitComment} disabled={!draft.trim()} className="text-xs px-3 py-1 bg-blue-600 text-white rounded disabled:opacity-40">Comment</button>
              <button onClick={() => { setComposing(false); setDraft('') }} className="text-xs px-3 py-1 border rounded">Cancel</button>
            </div>
          </div>
        )}
      </div>

      <div className="flex-1 overflow-y-auto p-3 space-y-3">
        {loading && <p className="text-xs text-gray-400 text-center py-4">Loading…</p>}
        {!loading && roots.length === 0 && (
          <p className="text-xs text-gray-400 text-center py-4">No comments yet</p>
        )}
        {roots.map((c) => (
          <CommentThread
            key={c.id}
            comment={c}
            view={view}
            replies={repliesOf(c.id)}
            userId={userId}
            readOnly={readOnly}
            onToggleResolve={() => toggleResolve(c)}
            onDelete={() => removeComment(c)}
            onDeleteReply={(reply) => removeComment(reply)}
            replyDraft={replyDrafts[c.id] ?? ''}
            replying={replyingTo === c.id}
            onToggleReply={() => setReplyingTo(replyingTo === c.id ? null : c.id)}
            onReplyDraftChange={(v) => setReplyDrafts((d) => ({ ...d, [c.id]: v }))}
            onSubmitReply={() => submitReply(c.id)}
          />
        ))}
      </div>
    </div>
  )
}

function CommentThread({
  comment, view, replies, userId, readOnly, onToggleResolve, onDelete, onDeleteReply,
  replyDraft, replying, onToggleReply, onReplyDraftChange, onSubmitReply,
}: {
  comment: CommentItem
  view: EditorView | null
  replies: CommentItem[]
  userId: string
  readOnly: boolean
  onToggleResolve: () => void
  onDelete: () => void
  onDeleteReply: (reply: CommentItem) => void
  replyDraft: string
  replying: boolean
  onToggleReply: () => void
  onReplyDraftChange: (v: string) => void
  onSubmitReply: () => void
}) {
  const anchor = parseAnchor(comment.anchor)
  const snippet = anchor && view ? textAtRange(view.state, anchor.from, anchor.to) : null
  const isMine = comment.authorId === userId

  return (
    <div className={`border rounded-lg p-3 ${comment.resolved ? 'bg-gray-50 opacity-70' : 'bg-white'}`}>
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs font-medium text-gray-700">{comment.authorName}</span>
        <span className="text-[10px] text-gray-400">
          {comment.resolved ? 'Resolved' : new Date(comment.createdAt).toLocaleString()}
        </span>
      </div>
      {snippet && (
        <div className="text-[11px] text-gray-500 bg-gray-100 rounded px-2 py-1 mb-1 line-clamp-2">“{snippet}”</div>
      )}
      <p className="text-sm text-gray-800 whitespace-pre-wrap">{comment.body}</p>
      {!readOnly && (
        <div className="flex gap-3 mt-2 text-[11px]">
          <button className="text-blue-600 hover:underline" onClick={onToggleReply}>{replies.length > 0 ? `Reply (${replies.length})` : 'Reply'}</button>
          <button className="text-gray-500 hover:underline" onClick={onToggleResolve}>
            {comment.resolved ? 'Reopen' : 'Resolve'}
          </button>
          {isMine && <button className="text-red-500 hover:underline" onClick={onDelete}>Delete</button>}
        </div>
      )}
      {replying && (
        <div className="mt-2">
          <textarea
            value={replyDraft}
            onChange={(e) => onReplyDraftChange(e.target.value)}
            rows={2}
            autoFocus
            placeholder="Reply…"
            className="w-full text-sm border rounded p-2 resize-none"
          />
          <button onClick={onSubmitReply} disabled={!replyDraft.trim()} className="mt-1 text-xs px-3 py-1 bg-blue-600 text-white rounded disabled:opacity-40">Reply</button>
        </div>
      )}
      {replies.map((r) => (
        <div key={r.id} className="mt-2 ml-3 border-l-2 pl-3">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-gray-600">{r.authorName}</span>
            <span className="text-[10px] text-gray-400">{new Date(r.createdAt).toLocaleString()}</span>
          </div>
          <p className="text-sm text-gray-700 whitespace-pre-wrap">{r.body}</p>
          {!readOnly && r.authorId === userId && (
            <button className="mt-1 text-[11px] text-red-500 hover:underline" onClick={() => onDeleteReply(r)}>Delete</button>
          )}
        </div>
      ))}
    </div>
  )
}