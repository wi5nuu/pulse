import { EditorView } from 'prosemirror-view'
import { EditorState } from 'prosemirror-state'
import { toggleMark, setBlockType } from 'prosemirror-commands'
import { wrapInList, liftListItem, sinkListItem } from 'prosemirror-schema-list'
import { addColumnAfter, addColumnBefore, deleteColumn, addRowAfter, addRowBefore, deleteRow, deleteTable, mergeCells, splitCell } from 'prosemirror-tables'
import { insertPoint } from 'prosemirror-transform'
import { NodeType, type Node, type ResolvedPos } from 'prosemirror-model'

// Semua node ancestor (dari depth 1 ke root). Ganti pola $pos.ancestors
// yang tidak ada di prosemirror-model.
export function ancestorsOf($pos: ResolvedPos): Node[] {
  const out: Node[] = []
  for (let d = $pos.depth; d > 0; d--) {
    out.push($pos.node(d))
  }
  return out
}

// B.30 Font families (Google Fonts popular + system fonts)
export const FONT_FAMILIES = [
  { value: 'Arial, sans-serif', label: 'Arial' },
  { value: 'Georgia, serif', label: 'Georgia' },
  { value: '"Times New Roman", serif', label: 'Times New Roman' },
  { value: '"Courier New", monospace', label: 'Courier New' },
  { value: 'Verdana, sans-serif', label: 'Verdana' },
  { value: 'Helvetica, sans-serif', label: 'Helvetica' },
  { value: '"Comic Sans MS", cursive', label: 'Comic Sans' },
  { value: 'Impact, sans-serif', label: 'Impact' },
  { value: '"Trebuchet MS", sans-serif', label: 'Trebuchet' },
  { value: '"Roboto", sans-serif', label: 'Roboto' },
  { value: '"Open Sans", sans-serif', label: 'Open Sans' },
  { value: '"Lato", sans-serif', label: 'Lato' },
  { value: '"Montserrat", sans-serif', label: 'Montserrat' },
  { value: '"Inter", sans-serif', label: 'Inter' },
]

export const FONT_SIZES = ['8pt', '9pt', '10pt', '11pt', '12pt', '14pt', '16pt', '18pt', '20pt', '22pt', '24pt', '28pt', '32pt', '36pt', '48pt', '72pt']
export const LINE_SPACINGS = ['1', '1.15', '1.5', '2', '2.5', '3']

export const TEXT_COLORS = ['#000000', '#dc2626', '#ea580c', '#d97706', '#65a30d', '#16a34a', '#0d9488', '#2563eb', '#7c3aed', '#db2777', '#475569', '#ffffff']
export const HIGHLIGHT_COLORS = ['#fef08a', '#a7f3d0', '#bfdbfe', '#fbcfe8', '#fde68a', '#fed7aa', '#e2e8f0', '#ffffff']

export function isActiveMark(state: EditorState, markType: string, attrs?: Record<string, unknown>): boolean {
  const mark = state.schema.marks[markType]
  if (!mark) return false
  const { from, to, empty } = state.selection
  if (empty) {
    const stored = state.storedMarks
    if (stored) return stored.some((m) => m.type === mark && (!attrs || Object.entries(attrs).every(([k, v]) => m.attrs[k] === v)))
    const node = state.selection.$from.parent
    return mark.isInSet(node.marks) !== undefined && (!attrs || true)
  }
  return state.doc.rangeHasMark(from, to, mark)
}

export function toggleMarkCommand(view: EditorView, markType: string, attrs?: Record<string, unknown>) {
  const mark = view.state.schema.marks[markType]
  if (!mark) return
  toggleMark(mark, attrs)(view.state, view.dispatch)
  view.focus()
}

export function setTextStyleCommand(view: EditorView, attrs: Record<string, unknown>) {
  const mark = view.state.schema.marks.textStyle
  if (!mark) return
  toggleMark(mark, attrs)(view.state, view.dispatch)
  view.focus()
}

export function clearTextStyleCommand(view: EditorView) {
  const mark = view.state.schema.marks.textStyle
  if (!mark) return
  toggleMark(mark, null)(view.state, view.dispatch)
  view.focus()
}

export function setTextAlign(view: EditorView, align: 'left' | 'center' | 'right' | 'justify' | null) {
  const { state, dispatch } = view
  const tr = state.tr
  const attrs: Record<string, unknown> = { textAlign: align }
  state.doc.nodesBetween(state.selection.from, state.selection.to, (node, pos) => {
    if (node.type.name === 'paragraph' || node.type.name === 'heading') {
      tr.setNodeMarkup(pos, undefined, { ...node.attrs, ...attrs })
    }
  })
  dispatch(tr)
  view.focus()
}

export function setLineHeight(view: EditorView, value: string | null) {
  const { state, dispatch } = view
  const tr = state.tr
  state.doc.nodesBetween(state.selection.from, state.selection.to, (node, pos) => {
    if (node.type.name === 'paragraph' || node.type.name === 'heading') {
      tr.setNodeMarkup(pos, undefined, { ...node.attrs, lineHeight: value })
    }
  })
  dispatch(tr)
  view.focus()
}

export function setHeading(view: EditorView, level: number | null) {
  const { state, dispatch } = view
  const type = state.schema.nodes[level ? `heading` : 'paragraph'] as NodeType
  if (!type) return
  const ok = setBlockType(type, level ? { level } : {})(state, dispatch)
  if (ok) view.focus()
}

// Toggle bullet/ordered list (C.55-56): masuk → keluar → ganti jenis.
export function toggleListCmd(view: EditorView, typeName: 'bullet_list' | 'ordered_list') {
  const { state, dispatch } = view
  const listType = state.schema.nodes[typeName] as NodeType
  const { $from } = state.selection
  const range = $from.blockRange()
  if (!range) return
  const list = range.depth >= 1 ? range.parent : null
  const inList = list !== null && (list.type.name === 'bullet_list' || list.type.name === 'ordered_list')
  if (!inList) {
    wrapInList(listType, null)(state, dispatch)
    view.focus()
    return
  }
  if (list!.type.name === typeName) {
    // Keluar dari list (lift list → blok biasa).
    const tr = state.tr.lift(range, range.depth - 1)
    dispatch(tr.scrollIntoView())
    view.focus()
    return
  }
  // Ganti jenis list (bullet ↔ ordered).
  const tr = state.tr
  state.doc.nodesBetween(range.$from.pos, range.$to.pos, (node, pos) => {
    if (node.type.name === 'bullet_list' || node.type.name === 'ordered_list') {
      tr.setNodeMarkup(pos, listType, node.attrs)
    }
  })
  dispatch(tr.scrollIntoView())
  view.focus()
}

// Task list (C.57 checklist): dalam task item → toggle checked attr;
// di luar → wrap jadi bullet list bertanda checkbox.
export function toggleTaskList(view: EditorView) {
  const { state, dispatch } = view
  const { $from } = state.selection
  const listType = state.schema.nodes.bullet_list as NodeType
  const ancestors = ancestorsOf($from)
  const itemIdx = ancestors.findIndex((n) => n.type.name === 'list_item')
  const item = itemIdx >= 0 ? ancestors[itemIdx] : null
  if (!item) {
    // Belum dalam list: wrap lalu tandai item sebagai task.
    if (wrapInList(listType, null)(state, dispatch)) {
      const { from, to } = view.state.selection
      const tr2 = view.state.tr
      view.state.doc.nodesBetween(from, to, (node, pos) => {
        if (node.type.name === 'list_item' && node.attrs.checked === null) {
          tr2.setNodeMarkup(pos, undefined, { ...node.attrs, checked: false })
        }
      })
      view.dispatch(tr2)
    }
    view.focus()
    return
  }
  // Dalam task item: toggle checked (true → false → null = keluar mode task).
  const current = item.attrs.checked as boolean | null
  const next = current === null ? false : current === false ? true : null
  // depth item = itemIdx+1 (ancestors[0] = node(depth=1)).
  const itemDepth = itemIdx + 1
  const tr = state.tr
  const pos = $from.before(itemDepth)
  tr.setNodeMarkup(pos, undefined, { ...item.attrs, checked: next })
  dispatch(tr)
  view.focus()
}

export function indentList(view: EditorView) {
  sinkListItem(view.state.schema.nodes.list_item as NodeType)(view.state, view.dispatch)
  view.focus()
}

export function outdentList(view: EditorView) {
  liftListItem(view.state.schema.nodes.list_item as NodeType)(view.state, view.dispatch)
  view.focus()
}

// Insert tabel (F.128) ukuran rows×cols di posisi selection.
export function insertTable(view: EditorView, rows: number, cols: number) {
  const { state, dispatch } = view
  const tableType = state.schema.nodes.table
  const rowType = state.schema.nodes.table_row
  const cellType = state.schema.nodes.table_cell
  if (!tableType || !rowType || !cellType) return

  const cells: unknown[] = []
  for (let r = 0; r < rows; r++) {
    const rowCells: unknown[] = []
    for (let c = 0; c < cols; c++) {
      rowCells.push(cellType.createAndFill() as never)
    }
    cells.push(rowType.create(null, rowCells as never))
  }
  const table = tableType.create(null, cells as never)

  const { $from } = state.selection
  const pos = insertPoint(state.doc, $from.pos, tableType) ?? $from.pos
  const tr = state.tr.replaceWith(pos, pos, table)
  dispatch(tr.scrollIntoView())
  view.focus()
}

// Insert gambar via URL (E.89). Resize via attr width (G.150).
export function insertImage(view: EditorView, src: string, alt = '') {
  const { state, dispatch } = view
  const type = state.schema.nodes.image
  const { from, to } = state.selection
  const tr = state.tr.replaceWith(from, to, type.create({ src, alt }))
  dispatch(tr)
  view.focus()
}

// Set link (E.98): prompt URL, terapkan mark pada seleksi.
export function setLink(view: EditorView, href: string | null) {
  const mark = view.state.schema.marks.link
  if (!mark) return
  toggleMark(mark, href ? { href, title: null } : null)(view.state, view.dispatch)
  view.focus()
}

export function addRowAfterCmd(view: EditorView) { addRowAfter(view.state, view.dispatch); view.focus() }
export function addRowBeforeCmd(view: EditorView) { addRowBefore(view.state, view.dispatch); view.focus() }
export function deleteRowCmd(view: EditorView) { deleteRow(view.state, view.dispatch); view.focus() }
export function addColumnAfterCmd(view: EditorView) { addColumnAfter(view.state, view.dispatch); view.focus() }
export function addColumnBeforeCmd(view: EditorView) { addColumnBefore(view.state, view.dispatch); view.focus() }
export function deleteColumnCmd(view: EditorView) { deleteColumn(view.state, view.dispatch); view.focus() }
export function deleteTableCmd(view: EditorView) { deleteTable(view.state, view.dispatch); view.focus() }
export function mergeCellsCmd(view: EditorView) { mergeCells(view.state, view.dispatch); view.focus() }
export function splitCellCmd(view: EditorView) { splitCell(view.state, view.dispatch); view.focus() }

export function isInTable(state: EditorState): boolean {
  return ancestorsOf(state.selection.$from).some((n) => n.type.name === 'table')
}

// Hitung statistik dokumen (K.214 word count live).
export function docStats(state: EditorState): { words: number; chars: number; charsNoSpaces: number; paragraphs: number } {
  let words = 0
  let chars = 0
  let charsNoSpaces = 0
  let paragraphs = 0
  state.doc.descendants((node) => {
    if (node.isText && node.text) {
      chars += node.text.length
      charsNoSpaces += node.text.replace(/\s/g, '').length
      words += node.text.split(/\s+/).filter((w) => w.length > 0).length
    }
    if (node.type.name === 'paragraph' || node.type.name === 'heading') paragraphs++
  })
  return { words, chars, charsNoSpaces, paragraphs }
}

// Ambil teks pada rentang (dipakai komentar — snippet anchor).
export function textAtRange(state: EditorState, from: number, to: number): string {
  return state.doc.textBetween(from, to, '\n', ' ').slice(0, 200)
}

// Daftar heading untuk outline (K.235).
export function outlineNodes(state: EditorState): { level: number; text: string; pos: number }[] {
  const out: { level: number; text: string; pos: number }[] = []
  state.doc.descendants((node, pos) => {
    if (node.type.name === 'heading') {
      out.push({ level: node.attrs.level, text: node.textContent, pos })
    }
  })
  return out
}