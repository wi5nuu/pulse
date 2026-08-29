'use client'

import { useState } from 'react'
import { EditorView } from 'prosemirror-view'
import { EditorState } from 'prosemirror-state'
import { undo, redo, undoCommand, redoCommand } from 'y-prosemirror'
import {
  FONT_FAMILIES, FONT_SIZES, LINE_SPACINGS, TEXT_COLORS,
  isActiveMark, toggleMarkCommand, setTextStyleCommand, clearTextStyleCommand,
  setTextAlign, setLineHeight, setHeading, toggleTaskList, toggleListCmd,
  indentList, outdentList, ancestorsOf,
  insertTable, insertImage, setLink, addRowAfterCmd, addRowBeforeCmd, deleteRowCmd,
  addColumnAfterCmd, addColumnBeforeCmd, deleteColumnCmd, deleteTableCmd,
  mergeCellsCmd, splitCellCmd, isInTable,
} from '@/lib/editor/commands'
import { exportMarkdown, exportTxt, exportHtml, printDoc } from '@/lib/editor/export'

interface Props {
  view: EditorView | null
  readOnly: boolean
  onToggleFind: () => void
  onToggleOutline: () => void
  onToggleComments: () => void
  commentsActive: boolean
  outlineActive: boolean
  findActive: boolean
}

function ToolBtn({
  label, title, onClick, active, disabled,
}: { label: string; title: string; onClick: () => void; active?: boolean; disabled?: boolean }) {
  return (
    <button
      type="button"
      title={title}
      disabled={disabled}
      onClick={onClick}
      className={`px-1.5 py-1 text-[13px] leading-none rounded hover:bg-gray-200 disabled:opacity-40 disabled:cursor-not-allowed transition-colors ${
        active ? 'bg-blue-100 text-blue-700' : 'text-gray-700'
      }`}
    >
      {label}
    </button>
  )
}

function Select({
  value, onChange, options, title, disabled,
}: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[]; title: string; disabled?: boolean }) {
  return (
    <select
      title={title}
      disabled={disabled}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="text-[12px] px-1 py-1 rounded border border-gray-300 bg-white text-gray-700 disabled:opacity-40 max-w-[110px]"
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>{o.label}</option>
      ))}
    </select>
  )
}

export default function EditorToolbar({
  view, readOnly, onToggleFind, onToggleOutline, onToggleComments,
  commentsActive, outlineActive, findActive,
}: Props) {
  const [tableMenuOpen, setTableMenuOpen] = useState(false)
  const [exportOpen, setExportOpen] = useState(false)
  const [imgMenuOpen, setImgMenuOpen] = useState(false)

  if (!view) return null
  const state: EditorState = view.state
  const inTable = isInTable(state)
  const headingLevel = (() => {
    const node = state.selection.$from.parent
    if (node.type.name === 'heading') return node.attrs.level as number
    return 0
  })()

  const disabled = readOnly
  const run = (fn: (v: EditorView) => void) => (disabled ? undefined : fn(view))

  const handleImage = () => {
    const url = window.prompt('Image URL:')
    if (url) insertImage(view, url)
  }

  const handleLink = () => {
    const current = state.selection.$from.marks().find((m) => m.type.name === 'link')?.attrs.href as string | undefined
    const url = window.prompt('Link URL (kosongkan untuk hapus):', current ?? 'https://')
    if (url !== null) {
      const trimmed = url.trim()
      if (trimmed === '' || trimmed === 'https://') {
        setLink(view, null)
      } else {
        // Block dangerous URI schemes (XSS prevention)
        try {
          const parsed = new URL(trimmed)
          const allowedSchemes = ['http:', 'https:', 'mailto:', 'tel:']
          if (!allowedSchemes.includes(parsed.protocol)) {
            window.prompt('Only http, https, mailto, and tel links are allowed.')
            return
          }
        } catch {
          // Not a valid URL — treat as relative path
        }
        setLink(view, trimmed)
      }
    }
  }

  return (
    <div className="border-b bg-gray-50 px-3 py-1.5 flex flex-wrap items-center gap-1 select-none">
      <ToolBtn label="↶" title="Undo (Ctrl+Z)" onClick={() => { if (!disabled) undoCommand(view.state, view.dispatch) }} disabled={disabled || !undo(view.state)} />
      <ToolBtn label="↷" title="Redo (Ctrl+Y)" onClick={() => { if (!disabled) redoCommand(view.state, view.dispatch) }} disabled={disabled || !redo(view.state)} />

      <div className="w-px h-5 bg-gray-300 mx-1" />

      <Select
        title="Font family (B.30)"
        disabled={disabled}
        value=""
        onChange={(v) => setTextStyleCommand(view, { fontFamily: v })}
        options={[{ value: '', label: 'Font' }, ...FONT_FAMILIES]}
      />
      <Select
        title="Text style (heading)"
        disabled={disabled}
        value={String(headingLevel)}
        onChange={(v) => setHeading(view, v === '0' ? null : Number(v))}
        options={[
          { value: '0', label: 'Normal' },
          { value: '1', label: 'Title' },
          { value: '2', label: 'Heading' },
          { value: '3', label: 'Subheading' },
        ]}
      />
      <Select
        title="Font size (B.32)"
        disabled={disabled}
        value=""
        onChange={(v) => setTextStyleCommand(view, { fontSize: v })}
        options={[{ value: '', label: 'Size' }, ...FONT_SIZES.map((s) => ({ value: s, label: s.replace('pt', '') }))]}
      />

      <div className="w-px h-5 bg-gray-300 mx-1" />

      <ToolBtn label="B" title="Bold (Ctrl+B)" active={isActiveMark(state, 'strong')} onClick={() => toggleMarkCommand(view, 'strong')} disabled={disabled} />
      <ToolBtn label="I" title="Italic (Ctrl+I)" active={isActiveMark(state, 'em')} onClick={() => toggleMarkCommand(view, 'em')} disabled={disabled} />
      <ToolBtn label="U" title="Underline (Ctrl+U)" active={isActiveMark(state, 'underline')} onClick={() => toggleMarkCommand(view, 'underline')} disabled={disabled} />
      <ToolBtn label="S" title="Strikethrough" active={isActiveMark(state, 'strike')} onClick={() => toggleMarkCommand(view, 'strike')} disabled={disabled} />
      <ToolBtn label="x²" title="Superscript" active={isActiveMark(state, 'superscript')} onClick={() => toggleMarkCommand(view, 'superscript')} disabled={disabled} />
      <ToolBtn label="x₂" title="Subscript" active={isActiveMark(state, 'subscript')} onClick={() => toggleMarkCommand(view, 'subscript')} disabled={disabled} />

      <div className="relative">
        <button
          type="button"
          title="Text color"
          onClick={() => {
            const c = window.prompt('Warna teks (HEX, contoh #dc2626):')
            if (c && /^#[0-9a-fA-F]{3,8}$/.test(c)) setTextStyleCommand(view, { color: c })
          }}
          className="px-1.5 py-1 text-[13px] leading-none rounded hover:bg-gray-200 text-gray-700"
        >
          <span className="inline-block w-3 h-3 rounded-full border border-gray-400 align-middle" style={{ background: 'linear-gradient(#000 50%, transparent 50%)' }} />
          <span className="ml-0.5">A</span>
        </button>
        <div className="hidden">
          {TEXT_COLORS.map((c) => (
            <span key={c} onClick={() => setTextStyleCommand(view, { color: c })} />
          ))}
        </div>
      </div>
      <ToolBtn label="🖍" title="Highlight" onClick={() => {
        const c = window.prompt('Warna highlight (HEX):')
        if (c && /^#[0-9a-fA-F]{3,8}$/.test(c)) setTextStyleCommand(view, { backgroundColor: c })
      }} disabled={disabled} />
      <ToolBtn label="⌫Fmt" title="Clear formatting" onClick={() => { clearTextStyleCommand(view); toggleMarkCommand(view, 'strong'); toggleMarkCommand(view, 'em'); toggleMarkCommand(view, 'underline'); toggleMarkCommand(view, 'strike'); toggleMarkCommand(view, 'superscript'); toggleMarkCommand(view, 'subscript') }} disabled={disabled} />

      <div className="w-px h-5 bg-gray-300 mx-1" />

      <ToolBtn label="⇤" title="Align left" active={state.selection.$from.parent.attrs.textAlign === 'left' || !state.selection.$from.parent.attrs.textAlign} onClick={() => setTextAlign(view, null)} disabled={disabled} />
      <ToolBtn label="⇥" title="Align center" active={state.selection.$from.parent.attrs.textAlign === 'center'} onClick={() => setTextAlign(view, 'center')} disabled={disabled} />
      <ToolBtn label="⇥⇥" title="Align right" active={state.selection.$from.parent.attrs.textAlign === 'right'} onClick={() => setTextAlign(view, 'right')} disabled={disabled} />
      <ToolBtn label="⇤⇥" title="Justify" active={state.selection.$from.parent.attrs.textAlign === 'justify'} onClick={() => setTextAlign(view, 'justify')} disabled={disabled} />
      <Select
        title="Line spacing"
        disabled={disabled}
        value=""
        onChange={(v) => setLineHeight(view, v)}
        options={[{ value: '', label: 'Spacing' }, ...LINE_SPACINGS.map((s) => ({ value: s, label: s }))]}
      />

      <div className="w-px h-5 bg-gray-300 mx-1" />

      <ToolBtn label="•" title="Bullet list" active={currentListType(state) === 'bullet_list'} onClick={() => toggleListCmd(view, 'bullet_list')} disabled={disabled} />
      <ToolBtn label="1." title="Numbered list" active={currentListType(state) === 'ordered_list'} onClick={() => toggleListCmd(view, 'ordered_list')} disabled={disabled} />
      <ToolBtn label="☑" title="Checklist" active={isTaskItem(state)} onClick={() => toggleTaskList(view)} disabled={disabled} />
      <ToolBtn label="⇥+" title="Indent" onClick={() => indentList(view)} disabled={disabled || !isInList(state)} />
      <ToolBtn label="⇤-" title="Outdent" onClick={() => outdentList(view)} disabled={disabled || !isInList(state)} />

      <div className="w-px h-5 bg-gray-300 mx-1" />

      <div className="relative">
        <ToolBtn label="▦" title="Table" onClick={() => { setTableMenuOpen(!tableMenuOpen); setExportOpen(false); setImgMenuOpen(false) }} disabled={disabled} />
        {tableMenuOpen && !disabled && (
          <div className="absolute left-0 top-full mt-1 z-20 bg-white border rounded-md shadow-lg p-2 min-w-[180px]">
            {!inTable ? (
              <>
                <div className="text-[11px] text-gray-500 px-1 pb-1">Insert table (baris × kolom)</div>
                <div className="grid grid-cols-4 gap-1 p-1">
                  {[2, 3, 4, 5].map((n) => (
                    <button key={n} type="button" className="text-[12px] px-1 py-1 border rounded hover:bg-blue-50" onClick={() => { insertTable(view, n, n); setTableMenuOpen(false) }}>
                      {n}×{n}
                    </button>
                  ))}
                </div>
                <div className="flex gap-1 mt-1">
                  <button type="button" className="text-[12px] px-1 py-1 border rounded hover:bg-blue-50" onClick={() => { insertTable(view, 2, 4); setTableMenuOpen(false) }}>2×4</button>
                  <button type="button" className="text-[12px] px-1 py-1 border rounded hover:bg-blue-50" onClick={() => { insertTable(view, 4, 2); setTableMenuOpen(false) }}>4×2</button>
                </div>
              </>
            ) : (
              <>
                <div className="text-[11px] text-gray-500 px-1 pb-1">Table actions</div>
                <TableAction label="Insert row below" fn={() => addRowAfterCmd(view)} />
                <TableAction label="Insert row above" fn={() => addRowBeforeCmd(view)} />
                <TableAction label="Delete row" fn={() => deleteRowCmd(view)} />
                <TableAction label="Insert column right" fn={() => addColumnAfterCmd(view)} />
                <TableAction label="Insert column left" fn={() => addColumnBeforeCmd(view)} />
                <TableAction label="Delete column" fn={() => deleteColumnCmd(view)} />
                <TableAction label="Merge cells" fn={() => mergeCellsCmd(view)} />
                <TableAction label="Split cell" fn={() => splitCellCmd(view)} />
                <button type="button" className="w-full text-left text-[12px] px-2 py-1 hover:bg-red-50 text-red-600 rounded" onClick={() => { deleteTableCmd(view); setTableMenuOpen(false) }}>
                  Delete table
                </button>
              </>
            )}
          </div>
        )}
      </div>

      <div className="relative">
        <ToolBtn label="🖼" title="Insert image" onClick={() => { setImgMenuOpen(!imgMenuOpen); setTableMenuOpen(false); setExportOpen(false) }} disabled={disabled} />
        {imgMenuOpen && !disabled && (
          <div className="absolute left-0 top-full mt-1 z-20 bg-white border rounded-md shadow-lg p-2 min-w-[200px]">
            <div className="text-[11px] text-gray-500 pb-1">Insert image (E.89)</div>
            <input
              type="text"
              placeholder="Image URL…"
              className="w-full text-[12px] px-2 py-1 border rounded mb-1"
              onKeyDown={(e) => {
                if (e.key === 'Enter' && (e.target as HTMLInputElement).value) {
                  insertImage(view, (e.target as HTMLInputElement).value)
                  setImgMenuOpen(false)
                }
              }}
            />
            <button type="button" className="text-[12px] px-2 py-1 border rounded hover:bg-blue-50" onClick={handleImage}>Prompt URL…</button>
          </div>
        )}
      </div>

      <ToolBtn label="🔗" title="Link (Ctrl+K)" active={state.selection.$from.marks().some((m) => m.type.name === 'link')} onClick={() => run(handleLink)} disabled={disabled} />

      <div className="w-px h-5 bg-gray-300 mx-1" />

      <ToolBtn label="📄" title="Export / Print" onClick={() => { setExportOpen(!exportOpen); setTableMenuOpen(false); setImgMenuOpen(false) }} />
      {exportOpen && (
        <div className="relative">
          <div className="absolute left-0 top-full mt-1 z-20 bg-white border rounded-md shadow-lg p-2 min-w-[160px]">
            <button type="button" className="w-full text-left text-[12px] px-2 py-1 hover:bg-gray-100 rounded" onClick={() => { exportMarkdown(view); setExportOpen(false) }}>Download .md</button>
            <button type="button" className="w-full text-left text-[12px] px-2 py-1 hover:bg-gray-100 rounded" onClick={() => { exportTxt(view); setExportOpen(false) }}>Download .txt</button>
            <button type="button" className="w-full text-left text-[12px] px-2 py-1 hover:bg-gray-100 rounded" onClick={() => { exportHtml(view); setExportOpen(false) }}>Download .html</button>
            <button type="button" className="w-full text-left text-[12px] px-2 py-1 hover:bg-gray-100 rounded" onClick={() => { printDoc(view); setExportOpen(false) }}>Print (Ctrl+P)</button>
          </div>
        </div>
      )}

      <div className="flex-1" />

      <ToolBtn label="🔍" title="Find & replace (Ctrl+F)" active={findActive} onClick={onToggleFind} />
      <ToolBtn label="☰" title="Document outline" active={outlineActive} onClick={onToggleOutline} />
      <ToolBtn label="💬" title="Comments" active={commentsActive} onClick={onToggleComments} />
    </div>
  )
}

function TableAction({ label, fn }: { label: string; fn: () => void }) {
  return (
    <button type="button" className="w-full text-left text-[12px] px-2 py-1 hover:bg-gray-100 rounded" onClick={fn}>
      {label}
    </button>
  )
}

function isInList(state: EditorState): boolean {
  return ancestorsOf(state.selection.$from).some((n) => n.type.name === 'list_item')
}

function currentListType(state: EditorState): 'bullet_list' | 'ordered_list' | null {
  const list = ancestorsOf(state.selection.$from).find(
    (n) => n.type.name === 'bullet_list' || n.type.name === 'ordered_list',
  )
  return list ? (list.type.name as 'bullet_list' | 'ordered_list') : null
}

function isTaskItem(state: EditorState): boolean {
  const item = ancestorsOf(state.selection.$from).find((n) => n.type.name === 'list_item')
  return item ? item.attrs.checked !== null && item.attrs.checked !== undefined : false
}