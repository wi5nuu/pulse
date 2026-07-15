'use client'

import { useEffect, useRef, useState } from 'react'
import { useParams } from 'next/navigation'
import { EditorView } from 'prosemirror-view'
import { EditorState } from 'prosemirror-state'
import { Schema } from 'prosemirror-model'
import { schema as basicSchema } from 'prosemirror-schema-basic'
import { addListNodes } from 'prosemirror-schema-list'
import { exampleSetup } from 'prosemirror-example-setup'
import * as Y from 'yjs'
import { ySyncPlugin, yUndoPlugin, yCursorPlugin } from 'y-prosemirror'
import { PulseWSProvider, ConnectionStatus } from '@/lib/yjs-provider'
import { WS_BASE } from '@/lib/api-client'
import { useAuthStore } from '@/store/auth'

const schema = new Schema({
  nodes: addListNodes(basicSchema.spec.nodes, 'paragraph block*', 'block'),
  marks: basicSchema.spec.marks,
})

const cursorColors = [
  '#6366f1', '#ef4444', '#22c55e', '#f59e0b', '#ec4899',
  '#14b8a6', '#8b5cf6', '#f97316', '#06b6d4', '#84cc16',
]

export default function DocEditorPage() {
  const params = useParams()
  const docId = params.docId as string
  const user = useAuthStore((s) => s.user)
  const editorRef = useRef<HTMLDivElement>(null)
  const providerRef = useRef<PulseWSProvider | null>(null)
  const ydocRef = useRef<Y.Doc | null>(null)
  const viewRef = useRef<EditorView | null>(null)
  const [connStatus, setConnStatus] = useState(ConnectionStatus.Disconnected)
  const [users, setUsers] = useState<{ name: string; color: string }[]>([])

  useEffect(() => {
    if (!user || !docId) return

    const ydoc = new Y.Doc()
    ydocRef.current = ydoc

    const wsUrl = `${WS_BASE}/ws/doc/${docId}`
    const provider = new PulseWSProvider(ydoc, wsUrl)
    providerRef.current = provider

    provider.onStatus((status) => {
      setConnStatus(status)
    })

    provider.connect()

    const awareness = provider.awareness
    awareness.setLocalState({
      user: {
        name: user.name,
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

    const plugins = [
      ...exampleSetup({ schema }),
      ySyncPlugin(yXmlFragment),
      yUndoPlugin({ trackedOrigins: [user.id] }),
      yCursorPlugin(awareness),
    ]

    const state = EditorState.create({
      schema,
      plugins,
    })

    if (editorRef.current) {
      let view: EditorView | undefined
      view = new EditorView(editorRef.current, {
        state,
        dispatchTransaction(tr) {
          if (!view) return
          const newState = view.state.apply(tr)
          view.updateState(newState)
        },
      })
      viewRef.current = view
    }

    return () => {
      provider.disconnect()
      viewRef.current?.destroy()
      ydoc.destroy()
    }
  }, [docId, user])

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
      <header className="border-b px-4 py-2 flex items-center justify-between bg-white">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium">Document</span>
          {users.map((u, i) => (
            <span key={i} className="text-xs px-2 py-0.5 rounded-full text-white" style={{ backgroundColor: u.color }}>
              {u.name}
            </span>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <span className={`inline-block w-2 h-2 rounded-full ${statusColors[connStatus]}`} />
          <span className="text-xs text-gray-500">{statusLabels[connStatus]}</span>
        </div>
      </header>
      <div className="flex-1 overflow-y-auto px-8 py-4">
        <div ref={editorRef} className="ProseMirror-container max-w-3xl mx-auto" />
      </div>
    </div>
  )
}
