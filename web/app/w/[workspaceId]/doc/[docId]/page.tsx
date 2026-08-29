'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'next/navigation'
import { EditorView } from 'prosemirror-view'
import { EditorState, type Command } from 'prosemirror-state'
import { keymap } from 'prosemirror-keymap'
import { baseKeymap, toggleMark } from 'prosemirror-commands'
import { splitListItem } from 'prosemirror-schema-list'
import { buildKeymap, buildInputRules } from 'prosemirror-example-setup'
import { dropCursor } from 'prosemirror-dropcursor'
import { gapCursor } from 'prosemirror-gapcursor'
import { columnResizing, tableEditing, goToNextCell } from 'prosemirror-tables'
import { undoCommand, redoCommand } from 'y-prosemirror'
import * as Y from 'yjs'
import { ySyncPlugin, yUndoPlugin, yCursorPlugin } from 'y-prosemirror'
import { PulseWSProvider, ConnectionStatus, type WSRole } from '@/lib/yjs-provider'
import { WS_BASE } from '@/lib/api-client'
import { useAuthStore } from '@/store/auth'
import { ShareDocumentModal } from '@/components/share-document-modal'
import EditorToolbar from '@/components/editor-toolbar'
import FindReplaceBar from '@/components/find-replace'
import OutlinePanel from '@/components/outline-panel'
import CommentsPanel from '@/components/comments-panel'
import { schema } from '@/lib/editor/schema'
import { docStats, ancestorsOf } from '@/lib/editor/commands'

const cursorColors = [
  '#6366f1', '#ef4444', '#22c55e', '#f59e0b', '#ec4899',
  '#14b8a6', '#8b5cf6', '#f97316', '#06b6d4', '#84cc16',
]

export default function DocEditorPage() {
  const params = useParams()
  const searchParams = useSearchParams()
  const docId = params.docId as string
  const lsToken = searchParams.get('ls')
  const user = useAuthStore((s) => s.user)
  const editorRef = useRef<HTMLDivElement>(null)
  const providerRef = useRef<PulseWSProvider | null>(null)
  const ydocRef = useRef<Y.Doc | null>(null)
  const viewRef = useRef<EditorView | null>(null)
  const [connStatus, setConnStatus] = useState(ConnectionStatus.Disconnected)
  const [users, setUsers] = useState<{ name: string; color: string }[]>([])
  const [isShareModalOpen, setIsShareModalOpen] = useState(false)
  const [wsRole, setWsRole] = useState<WSRole | null>(null)
  const [showFind, setShowFind] = useState(false)
  const [showOutline, setShowOutline] = useState(false)
  const [showComments, setShowComments] = useState(false)
  const [stats, setStats] = useState({ words: 0, chars: 0, charsNoSpaces: 0, paragraphs: 0 })
  const [, forceRender] = useState(0)
  const toggleFindRef = useRef<() => void>(() => {})
  const isReadOnly = wsRole === 'viewer' || wsRole === 'view'
  const readOnlyRef = useRef(false)
  readOnlyRef.current = isReadOnly
  const userRef = useRef(user)
  userRef.current = user

  const bump = useCallback(() => forceRender((n) => n + 1), [])

  useEffect(() => {
    toggleFindRef.current = () => setShowFind((v) => !v)
  }, [])

  useEffect(() => {
    const currentUser = userRef.current
    if (!currentUser || !docId) return

    const ydoc = new Y.Doc()
    ydocRef.current = ydoc

    const wsUrl = `${WS_BASE}/ws/doc/${docId}${lsToken ? `?ls=${lsToken}` : ''}`
    const provider = new PulseWSProvider(ydoc, wsUrl)
    providerRef.current = provider

    provider.onStatus((status) => {
      setConnStatus(status)
    })

    const unsubRole = provider.onRole((role) => {
      setWsRole(role)
    })

    provider.connect()

    const awareness = provider.awareness
    awareness.setLocalState({
      user: {
        name: currentUser.name,
        color: cursorColors[Math.floor(Math.random() * cursorColors.length)],
      },
    })

    const onAwarenessChange = () => {
      const states = awareness.getStates()
      const userList: { name: string; color: string }[] = []
      states.forEach((state) => {
        if (state.user) {
          userList.push(state.user)
        }
      })
      setUsers(userList)
    }
    awareness.on('change', onAwarenessChange)

    const yXmlFragment = ydoc.getXmlFragment('prosemirror')

    // Toggle link via prompt (E.98 / R.301 Ctrl+K).
    const toggleLinkCommand = () => {
      const view = viewRef.current
      if (!view) return false
      const mark = view.state.schema.marks.link
      if (!mark) return false
      const current = view.state.selection.$from.marks().find((m) => m.type.name === 'link')?.attrs.href as string | undefined
      const url = window.prompt('Link URL (kosongkan untuk hapus):', current ?? 'https://')
      if (url === null) return true
      const trimmed = url.trim()
      if (trimmed === '' || trimmed === 'https://') {
        toggleMark(mark, null)(view.state, view.dispatch)
      } else {
        // Block dangerous URI schemes (XSS prevention)
        try {
          const parsed = new URL(trimmed)
          const allowedSchemes = ['http:', 'https:', 'mailto:', 'tel:']
          if (!allowedSchemes.includes(parsed.protocol)) {
            window.prompt('Only http, https, mailto, and tel links are allowed.')
            view.focus()
            return true
          }
        } catch {
          // Not a valid URL — treat as relative path
        }
        toggleMark(mark, { href: trimmed, title: null })(view.state, view.dispatch)
      }
      view.focus()
      return true
    }

    const keymaps = buildKeymap(schema, { 'Mod-z': false, 'Mod-y': false, 'Mod-Shift-z': false })

    const toggleMarkWith = (markName: string): Command => (state, dispatch) => {
      const mark = schema.marks[markName]
      if (!mark) return false
      return toggleMark(mark)(state, dispatch)
    }

    const plugins = [
      ySyncPlugin(yXmlFragment),
      yUndoPlugin({ trackedOrigins: [currentUser.id] }),
      yCursorPlugin(awareness),
      buildInputRules(schema),
      keymap({
        ...keymaps,
        'Mod-z': undoCommand,
        'Mod-y': redoCommand,
        'Mod-Shift-z': redoCommand,
        'Mod-f': () => { toggleFindRef.current(); return true },
        'Mod-h': () => { toggleFindRef.current(); return true },
        'Mod-k': toggleLinkCommand,
        'Ctrl-.': toggleMarkWith('superscript'),
        'Ctrl-,': toggleMarkWith('subscript'),
        'Alt-Shift-5': toggleMarkWith('strike'),
        'Tab': goToNextCell(1),
        'Shift-Tab': goToNextCell(-1),
        'Enter': splitListItemCmd,
      }),
      keymap(baseKeymap),
      dropCursor(),
      gapCursor(),
      columnResizing(),
      tableEditing(),
    ]

    const state = EditorState.create({ schema, plugins })

    if (editorRef.current) {
      let view: EditorView | undefined
      view = new EditorView(editorRef.current, {
        state,
        editable: () => !readOnlyRef.current,
        handleClick(_v, pos, event) {
          // Checklist interaktif (C.57): klik checkbox → toggle attr.
          const target = event.target as HTMLElement
          if (target.classList.contains('pm-task-checkbox')) {
            event.preventDefault()
            const $pos = view!.state.doc.resolve(pos)
            const ancestors = ancestorsOf($pos)
            const idx = ancestors.findIndex((n) => n.type.name === 'list_item')
            if (idx >= 0 && ancestors[idx].attrs.checked !== null) {
              const tr = view!.state.tr
              tr.setNodeMarkup($pos.before(idx + 1), undefined, { ...ancestors[idx].attrs, checked: !ancestors[idx].attrs.checked })
              view!.dispatch(tr)
            }
            return true
          }
          return false
        },
        dispatchTransaction(tr) {
          if (!view) return
          const newState = view.state.apply(tr)
          view.updateState(newState)
          // Word count live (K.215) + toolbar state update.
          setStats(docStats(newState))
          bump()
        },
      })
      viewRef.current = view
      setStats(docStats(view.state))
    }

    return () => {
      awareness.off('change', onAwarenessChange)
      unsubRole()
      viewRef.current?.destroy()
      viewRef.current = null
      provider.disconnect()
      provider.destroy()
      ydoc.destroy()
      ydocRef.current = null
      providerRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [docId, lsToken])

  const statusColors: Record<ConnectionStatus, string> = {
    [ConnectionStatus.Connected]: 'bg-green-500',
    [ConnectionStatus.Connecting]: 'bg-yellow-500',
    [ConnectionStatus.Reconnecting]: 'bg-yellow-500',
    [ConnectionStatus.Disconnected]: 'bg-red-500',
  }

  const statusLabels: Record<ConnectionStatus, string> = {
    [ConnectionStatus.Connected]: 'Connected',
    [ConnectionStatus.Connecting]: 'Connecting...',
    [ConnectionStatus.Reconnecting]: 'Reconnecting...',
    [ConnectionStatus.Disconnected]: 'Offline',
  }

  return (
    <div className="flex-1 flex flex-col">
      {isReadOnly && (
        <div className="bg-amber-50 border-b border-amber-200 px-4 py-1.5 text-xs text-amber-700 text-center">
          View only — you don&apos;t have edit access to this document.
        </div>
      )}
      <header className="border-b px-4 py-2 flex items-center justify-between bg-white">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium">Document</span>
          {users.map((u, i) => (
            <span key={i} className="text-xs px-2 py-0.5 rounded-full text-white" style={{ backgroundColor: u.color }}>
              {u.name}
            </span>
          ))}
        </div>
        <div className="flex items-center gap-3">
          {!isReadOnly && (
            <button
              onClick={() => setIsShareModalOpen(true)}
              className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 transition-colors"
            >
              Share
            </button>
          )}
          <div className="flex items-center gap-2">
            <span className={`inline-block w-2 h-2 rounded-full ${statusColors[connStatus]}`} />
            <span className="text-xs text-gray-500">{statusLabels[connStatus]}</span>
          </div>
        </div>
      </header>

      <EditorToolbar
        view={viewRef.current}
        readOnly={isReadOnly}
        onToggleFind={() => setShowFind((v) => !v)}
        onToggleOutline={() => setShowOutline((v) => !v)}
        onToggleComments={() => setShowComments((v) => !v)}
        commentsActive={showComments}
        outlineActive={showOutline}
        findActive={showFind}
      />

      {showFind && <FindReplaceBar view={viewRef.current} onClose={() => setShowFind(false)} />}

      <div className="flex-1 flex overflow-hidden">
        <div className="flex-1 overflow-y-auto px-8 py-4">
          <div ref={editorRef} className="ProseMirror-container max-w-3xl mx-auto" />
        </div>
        {showOutline && <OutlinePanel view={viewRef.current} onClose={() => setShowOutline(false)} />}
        {showComments && (
          <CommentsPanel
            docId={docId}
            view={viewRef.current}
            userId={user?.id ?? ''}
            readOnly={isReadOnly}
            provider={providerRef.current}
            onClose={() => setShowComments(false)}
          />
        )}
      </div>

      <footer className="border-t bg-white px-4 py-1.5 flex items-center gap-4 text-xs text-gray-500">
        <span>{stats.words} words</span>
        <span>{stats.chars} chars</span>
        <span>{stats.charsNoSpaces} chars (no spaces)</span>
        <span>{stats.paragraphs} paragraphs</span>
        <span className="flex-1" />
        <span>{users.length} online</span>
      </footer>

      <ShareDocumentModal
        documentId={docId}
        isOpen={isShareModalOpen}
        onClose={() => setIsShareModalOpen(false)}
      />
    </div>
  )
}

function splitListItemCmd(state: EditorState, dispatch?: (tr: import('prosemirror-state').Transaction) => void): boolean {
  return splitListItem(state.schema.nodes.list_item as never)(state, dispatch as never)
}