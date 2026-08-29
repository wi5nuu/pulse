import { Schema, type DOMOutputSpec, type NodeSpec, type MarkSpec } from 'prosemirror-model'
import OrderedMap from 'orderedmap'
import { schema as basicSchema } from 'prosemirror-schema-basic'
import { addListNodes } from 'prosemirror-schema-list'
import { tableNodes } from 'prosemirror-tables'

// Schema lengkap editor (fiturwajibada B/C/D/F/G):
//   - marks: bold/italic/underline/strike/sup/sub (B), link, code,
//     textStyle (font size, warna teks, highlight — B.32-34)
//   - nodes: heading 1-6, bullet/numbered/task list (C.55-59),
//     blockquote, code block, horizontal rule, image (E.89/G),
//     tabel (F: insert/merge/resize via prosemirror-tables)

// Paragraph dengan textAlign (C.48) + lineHeight (C.49).
const paragraph: NodeSpec = {
  content: 'inline*',
  group: 'block',
  attrs: {
    textAlign: { default: null },
    lineHeight: { default: null },
  },
  parseDOM: [
    {
      tag: 'p',
      getAttrs: (dom: HTMLElement) => ({
        textAlign: dom.style.textAlign || null,
        lineHeight: dom.style.lineHeight || null,
      }),
    },
  ],
  toDOM: (node) => {
    const style: string[] = []
    if (node.attrs.textAlign) style.push(`text-align:${node.attrs.textAlign}`)
    if (node.attrs.lineHeight) style.push(`line-height:${node.attrs.lineHeight}`)
    return ['p', style.length ? { style: style.join(';') } : null, 0]
  },
}

// Heading dengan dukungan textAlign/lineHeight (konsisten dengan paragraph).
const heading: NodeSpec = {
  attrs: {
    level: { default: 1 },
    textAlign: { default: null },
    lineHeight: { default: null },
  },
  content: 'inline*',
  group: 'block',
  defining: true,
  parseDOM: [1, 2, 3, 4, 5, 6].map((lvl) => ({
    tag: `h${lvl}`,
    getAttrs: (dom: HTMLElement) => ({
      level: lvl,
      textAlign: dom.style.textAlign || null,
      lineHeight: dom.style.lineHeight || null,
    }),
  })),
  toDOM: (node) => {
    const style: string[] = []
    if (node.attrs.textAlign) style.push(`text-align:${node.attrs.textAlign}`)
    if (node.attrs.lineHeight) style.push(`line-height:${node.attrs.lineHeight}`)
    return [`h${node.attrs.level}`, style.length ? { style: style.join(';') } : null, 0]
  },
}

// List item dengan dukungan task/checklist (C.57): attrs.checked = null
// (item biasa) | true/false (checkbox interaktif).
const listItem: NodeSpec = {
  content: 'paragraph block*',
  group: 'block',
  defining: true,
  attrs: { checked: { default: null } },
  parseDOM: [
    {
      tag: 'li',
      getAttrs: (dom: HTMLElement) => {
        const c = dom.getAttribute('data-checked')
        return { checked: c === 'true' ? true : c === 'false' ? false : null }
      },
    },
  ],
  toDOM: (node) => {
    if (node.attrs.checked === null) return ['li', 0]
    return ['li', { 'data-checked': String(node.attrs.checked), class: 'pm-task-item' }, 0]
  },
}

const marks: Record<string, MarkSpec> = {
  link: basicSchema.spec.marks.get('link') as MarkSpec,
  em: basicSchema.spec.marks.get('em') as MarkSpec,
  strong: basicSchema.spec.marks.get('strong') as MarkSpec,
  code: basicSchema.spec.marks.get('code') as MarkSpec,
  // B.27 strikethrough
  strike: {
    parseDOM: [{ tag: 's' }, { tag: 'del' }, { tag: 'strike' }, { style: 'text-decoration:line-through' }],
    toDOM: (): DOMOutputSpec => ['s', 0],
  },
  // B.26 underline
  underline: {
    parseDOM: [{ tag: 'u' }, { style: 'text-decoration:underline' }],
    toDOM: (): DOMOutputSpec => ['u', 0],
  },
  // B.28 superscript
  superscript: {
    parseDOM: [{ tag: 'sup' }, { style: 'vertical-align:super' }],
    toDOM: (): DOMOutputSpec => ['sup', 0],
  },
  // B.29 subscript
  subscript: {
    parseDOM: [{ tag: 'sub' }, { style: 'vertical-align:sub' }],
    toDOM: (): DOMOutputSpec => ['sub', 0],
  },
  // B.30-34: font family, font size, text color, highlight
  textStyle: {
    attrs: {
      fontFamily: { default: null },
      fontSize: { default: null },
      color: { default: null },
      backgroundColor: { default: null },
    },
    parseDOM: [
      {
        style: 'font-family',
        getAttrs: (v: string) => ({ fontFamily: v || null }),
      },
      {
        style: 'font-size',
        getAttrs: (v: string) => ({ fontSize: v || null }),
      },
      {
        style: 'color',
        getAttrs: (v: string) => ({ color: v || null }),
      },
      {
        style: 'background-color',
        getAttrs: (v: string) => ({ backgroundColor: v || null }),
      },
    ],
    toDOM: (mark): DOMOutputSpec => {
      const style: string[] = []
      if (mark.attrs.fontFamily) style.push(`font-family:${mark.attrs.fontFamily}`)
      if (mark.attrs.fontSize) style.push(`font-size:${mark.attrs.fontSize}`)
      if (mark.attrs.color) style.push(`color:${mark.attrs.color}`)
      if (mark.attrs.backgroundColor) style.push(`background-color:${mark.attrs.backgroundColor}`)
      return ['span', { style: style.join(';') }, 0]
    },
  },
}

const baseNodes = basicSchema.spec.nodes
const nodes: Record<string, NodeSpec> = {
  doc: baseNodes.get('doc') as NodeSpec,
  paragraph,
  blockquote: baseNodes.get('blockquote') as NodeSpec,
  horizontal_rule: baseNodes.get('horizontal_rule') as NodeSpec,
  heading,
  code_block: baseNodes.get('code_block') as NodeSpec,
  text: baseNodes.get('text') as NodeSpec,
  image: baseNodes.get('image') as NodeSpec,
  hard_break: baseNodes.get('hard_break') as NodeSpec,
}

const listNodes = addListNodes(OrderedMap.from(nodes), 'paragraph block*', 'block')

const tableSpec = tableNodes({
  tableGroup: 'block',
  cellContent: 'block+',
  cellAttributes: {
    background: {
      default: null,
      getFromDOM: (dom: HTMLElement) => dom.style.backgroundColor || null,
      setDOMAttr: (value: unknown, attrs: Record<string, unknown>) => {
        if (value) attrs.style = `background-color:${value}`
      },
    },
    colwidth: {
      default: null,
      getFromDOM: (dom: HTMLElement) => dom.getAttribute('data-colwidth') || null,
      setDOMAttr: (value: unknown, attrs: Record<string, unknown>) => {
        if (value) attrs['data-colwidth'] = String(value)
      },
    },
  },
})

export const schema = new Schema({
  nodes: OrderedMap.from({
    ...listNodes.toObject(),
    list_item: listItem,
    table: tableSpec.table,
    table_row: tableSpec.table_row,
    table_cell: tableSpec.table_cell,
    table_header: tableSpec.table_header,
  }),
  marks: OrderedMap.from(marks),
})