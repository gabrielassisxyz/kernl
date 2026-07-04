import { marked } from 'marked'
import DOMPurify from 'dompurify'

// DA replies arrive as markdown; render + sanitize before v-html. Sanitizing is
// non-negotiable: the LLM can echo note content, which is user-writable, so the
// rendered HTML must never carry scripts or event handlers into the page.
export function renderMarkdown(source: string): string {
  const html = marked.parse(source, { async: false, gfm: true, breaks: true }) as string
  return DOMPurify.sanitize(html)
}
