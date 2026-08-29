import { EditorView } from 'prosemirror-view'
import { DOMSerializer } from 'prosemirror-model'
import { MarkdownSerializer, defaultMarkdownSerializer } from 'prosemirror-markdown'

// Export dokumen (fiturwajibada P.275-282): .md, .txt, .html + print (A.19).

function download(filename: string, content: string, mime: string) {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

// Markdown: extend serializer default dengan strike/underline/table/task.
const markdownSerializer = new MarkdownSerializer(
  {
    ...defaultMarkdownSerializer.nodes,
    strike: undefined as never,
    table: (state, node) => {
      state.write('\n')
      const header = node.firstChild
      if (header) {
        const cells: string[] = []
        header.forEach((cell: { textContent: string }) => cells.push(cell.textContent.replace(/\|/g, '\\|').replace(/\n/g, ' ')))
        state.write(`| ${cells.join(' | ')} |\n`)
        state.write(`| ${cells.map(() => '---').join(' | ')} |\n`)
      }
      node.forEach((row: { forEach: (arg0: (cell: { textContent: string }) => void) => void }) => {
        if (row === header) return
        const cells: string[] = []
        row.forEach((cell: { textContent: string }) => cells.push(cell.textContent.replace(/\|/g, '\\|').replace(/\n/g, ' ')))
        state.write(`| ${cells.join(' | ')} |\n`)
      })
      state.write('\n')
    },
  },
  {
    ...defaultMarkdownSerializer.marks,
    strike: { open: '~~', close: '~~', mixable: true, expelEnclosingWhitespace: true },
    underline: { open: '__', close: '__', mixable: true, expelEnclosingWhitespace: true },
    textStyle: {
      open: (_state, mark) => {
        const style: string[] = []
        if (mark.attrs.fontSize) style.push(`font-size:${mark.attrs.fontSize}`)
        if (mark.attrs.color) style.push(`color:${mark.attrs.color}`)
        if (mark.attrs.backgroundColor) style.push(`background-color:${mark.attrs.backgroundColor}`)
        return style.length ? `<span style="${style.join(';')}">` : ''
      },
      close: (_state, mark) => {
        const style: string[] = []
        if (mark.attrs.fontSize) style.push('font-size', 'color', 'background-color')
        return style.length ? '</span>' : ''
      },
      mixable: true,
      expelEnclosingWhitespace: true,
    },
  },
)

export function exportMarkdown(view: EditorView, filename = 'document.md') {
  const md = markdownSerializer.serialize(view.state.doc)
  download(filename, md, 'text/markdown;charset=utf-8')
}

export function exportTxt(view: EditorView, filename = 'document.txt') {
  download(filename, view.state.doc.textContent, 'text/plain;charset=utf-8')
}

export function exportHtml(view: EditorView, filename = 'document.html') {
  const serializer = DOMSerializer.fromSchema(view.state.schema)
  const dom = serializer.serializeFragment(view.state.doc.content)
  const container = document.createElement('div')
  container.appendChild(dom)
  const html = `<!doctype html><html><head><meta charset="utf-8"><title>Pulse Document</title>
<style>body{font-family:Georgia,serif;max-width:800px;margin:40px auto;padding:0 20px;line-height:1.6}
table{border-collapse:collapse}td,th{border:1px solid #999;padding:6px 10px}img{max-width:100%}</style></head>
<body>${container.innerHTML}</body></html>`
  download(filename, html, 'text/html;charset=utf-8')
}

export function printDoc(view: EditorView) {
  const serializer = DOMSerializer.fromSchema(view.state.schema)
  const dom = serializer.serializeFragment(view.state.doc.content)
  const container = document.createElement('div')
  container.appendChild(dom)
  const win = window.open('', '_blank', 'width=900,height=700')
  if (!win) return
  win.document.write(`<!doctype html><html><head><title>Print</title>
<style>body{font-family:Georgia,serif;max-width:800px;margin:40px auto;padding:0 20px;line-height:1.6}
table{border-collapse:collapse}td,th{border:1px solid #999;padding:6px 10px}img{max-width:100%}</style></head>
<body>${container.innerHTML}<script>window.onload=function(){window.print()}</script></body></html>`)
  win.document.close()
}