'use client'

import { useEffect, useState } from 'react'
import { EditorView } from 'prosemirror-view'
import { TextSelection } from 'prosemirror-state'
import toast from 'react-hot-toast'

// Find & replace (R.304 Ctrl+H): cari teks, replace satu/semua.
interface Props {
  view: EditorView | null
  onClose: () => void
}

interface Match {
  from: number
  to: number
}

function collectMatches(view: EditorView, query: string, caseSensitive: boolean): Match[] {
  const matches: Match[] = []
  if (!query) return matches
  const q = caseSensitive ? query : query.toLowerCase()
  view.state.doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return
    const text = caseSensitive ? node.text : node.text.toLowerCase()
    let idx = text.indexOf(q)
    while (idx !== -1) {
      matches.push({ from: pos + idx, to: pos + idx + query.length })
      idx = text.indexOf(q, idx + 1)
    }
  })
  return matches
}

export default function FindReplaceBar({ view, onClose }: Props) {
  const [query, setQuery] = useState('')
  const [replacement, setReplacement] = useState('')
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [matches, setMatches] = useState<Match[]>([])
  const [current, setCurrent] = useState(0)

  const refresh = () => {
    if (!view) return
    setMatches(collectMatches(view, query, caseSensitive))
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, caseSensitive, view?.state.doc])

  const selectMatch = (idx: number) => {
    if (!view || matches.length === 0) return
    const m = matches[((idx % matches.length) + matches.length) % matches.length]
    const tr = view.state.tr
    tr.setSelection(TextSelection.create(view.state.doc, m.from, m.to))
    view.dispatch(tr)
    view.focus()
    setCurrent(idx)
  }

  const next = () => selectMatch(current + 1)
  const prev = () => selectMatch(current - 1)

  const replaceOne = () => {
    if (!view || matches.length === 0) return
    const m = matches[current % matches.length]
    view.dispatch(view.state.tr.insertText(replacement, m.from, m.to))
    refresh()
    if (matches.length > 1) selectMatch(current)
  }

  const replaceAll = () => {
    if (!view || matches.length === 0) return
    const count = matches.length
    let tr = view.state.tr
    // Replace dari belakang supaya offset tidak bergeser.
    for (let i = matches.length - 1; i >= 0; i--) {
      tr = tr.insertText(replacement, matches[i].from, matches[i].to)
    }
    view.dispatch(tr)
    refresh()
    toast.success(`${count} replacement(s)`)
  }

  return (
    <div className="border-b bg-white px-4 py-2 flex items-center gap-2 flex-wrap">
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Find…"
        autoFocus
        className="text-sm border rounded px-2 py-1 w-48"
      />
      <input
        value={replacement}
        onChange={(e) => setReplacement(e.target.value)}
        placeholder="Replace with…"
        className="text-sm border rounded px-2 py-1 w-48"
      />
      <label className="text-xs text-gray-500 flex items-center gap-1">
        <input type="checkbox" checked={caseSensitive} onChange={(e) => setCaseSensitive(e.target.checked)} />
        Match case
      </label>
      <span className="text-xs text-gray-500">{matches.length > 0 ? `${current + 1}/${matches.length}` : 'No results'}</span>
      <div className="flex gap-1">
        <button onClick={prev} className="text-xs px-2 py-1 border rounded hover:bg-gray-100" title="Previous (Shift+Enter)">↑</button>
        <button onClick={next} className="text-xs px-2 py-1 border rounded hover:bg-gray-100" title="Next (Enter)">↓</button>
        <button onClick={replaceOne} disabled={matches.length === 0} className="text-xs px-2 py-1 border rounded hover:bg-gray-100 disabled:opacity-40">Replace</button>
        <button onClick={replaceAll} disabled={matches.length === 0} className="text-xs px-2 py-1 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-40">Replace all</button>
        <button onClick={onClose} className="text-xs px-2 py-1 text-gray-400 hover:text-gray-600">✕</button>
      </div>
    </div>
  )
}