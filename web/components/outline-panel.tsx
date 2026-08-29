'use client'

import { useEffect, useState } from 'react'
import { EditorView } from 'prosemirror-view'
import { TextSelection } from 'prosemirror-state'
import { outlineNodes } from '@/lib/editor/commands'

// Document outline (K.235): navigasi heading, auto-update real-time.
interface Props {
  view: EditorView | null
  onClose: () => void
}

export default function OutlinePanel({ view, onClose }: Props) {
  const [items, setItems] = useState<{ level: number; text: string; pos: number }[]>([])

  useEffect(() => {
    if (!view) return
    const update = () => setItems(outlineNodes(view.state))
    update()
    // Update pada setiap transaksi — outline selalu sinkron dengan dokumen.
    const orig = view.dispatch
    view.dispatch = (tr) => {
      orig(tr)
      update()
    }
    return () => {
      view.dispatch = orig
    }
  }, [view])

  const goTo = (pos: number) => {
    if (!view) return
    const tr = view.state.tr
    tr.setSelection(TextSelection.create(view.state.doc, pos))
    view.dispatch(tr.scrollIntoView())
    view.focus()
  }

  return (
    <div className="w-64 border-l bg-white flex flex-col">
      <div className="px-4 py-3 border-b flex items-center justify-between">
        <span className="text-sm font-medium">Outline</span>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-sm px-1">✕</button>
      </div>
      <div className="flex-1 overflow-y-auto p-3">
        {items.length === 0 && <p className="text-xs text-gray-400 text-center py-4">No headings yet</p>}
        {items.map((h, i) => (
          <button
            key={i}
            onClick={() => goTo(h.pos)}
            className="block w-full text-left text-[13px] py-1 px-2 rounded hover:bg-gray-100 text-gray-700 truncate"
            style={{ paddingLeft: `${(h.level - 1) * 14 + 8}px` }}
          >
            {h.text || 'Untitled'}
          </button>
        ))}
      </div>
    </div>
  )
}