// Pure, framework-free logic for the Obsidian-style wikilink autocomplete in the
// note editor. Kept free of CodeMirror/Vue imports so it can be unit-tested and
// reused. The CodeMirror wiring (CompletionSource + decoration) lives in the
// editor and imports these helpers.

import { typeForPrefix } from './nodeTypes'

export interface WikilinkQuery {
  // Whether the cursor is inside an open, unclosed wikilink.
  active: boolean
  // The text query to search for (with any `!<letter> ` type prefix stripped).
  query: string
  // The node type to filter by, when a known `!<letter> ` prefix was given.
  type?: string
}

// Text typed inside an open wikilink, e.g. "[[lin" -> "lin". Not active if a
// `]]` has already closed the link or a newline intervenes (wikilinks are
// single-line). Group captures everything after the last `[[` up to the cursor.
const OPEN_WIKILINK = /\[\[([^\]\n]*)$/

// A leading type filter: "!b " -> prefix "b", rest "...". The space is required
// so a bare "!" or "!b" (mid-type) is treated as literal query text.
const TYPE_PREFIX = /^!([a-z])\s+(.*)$/

export function parseWikilinkQuery(textBeforeCursor: string): WikilinkQuery {
  const m = textBeforeCursor.match(OPEN_WIKILINK)
  if (!m) return { active: false, query: '' }

  const inner = m[1]
  const pm = inner.match(TYPE_PREFIX)
  if (pm) {
    const type = typeForPrefix(pm[1])
    // Known prefix -> apply the type filter and use the remainder as the query.
    // Unknown prefix -> fall through and treat the whole thing as a literal query.
    if (type) return { active: true, query: pm[2], type }
  }

  return { active: true, query: inner }
}

// The text inserted on selecting a completion. The uuid resolves to any node
// type server-side; the alias keeps the rendered link readable.
export function buildWikilinkInsert(node: { id: string; title: string }): string {
  return `[[${node.id}|${node.title}]]`
}
