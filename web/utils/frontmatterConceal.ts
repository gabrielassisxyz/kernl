// Hides the raw YAML frontmatter block from the CodeMirror surface in live /
// reading modes, where it's presented instead by the NoteProperties block above
// the editor (Obsidian-style). Source mode omits this extension, so the raw
// `---` block stays visible and editable there.
//
// The range computation is a pure function (unit-tested headless); the ViewPlugin
// is the thin CM6 glue, rebuilding the decoration as the document changes.

import { StateField, type EditorState } from '@codemirror/state'
import {
  Decoration,
  EditorView,
  type DecorationSet,
} from '@codemirror/view'

// A leading `---` block and its closing `---` delimiter, including the trailing
// newline so the concealed range ends on a clean line boundary.
const FRONTMATTER_RE = /^---\r?\n[\s\S]*?\r?\n---(?:\r?\n|$)/

// Returns [from, to) of the frontmatter block to conceal, or null when the
// document has no well-formed frontmatter (nothing to hide).
export function frontmatterConcealRange(doc: string): { from: number; to: number } | null {
  if (!doc.startsWith('---\n') && !doc.startsWith('---\r\n')) return null
  const match = doc.match(FRONTMATTER_RE)
  if (!match) return null
  const to = match[0].length
  if (to <= 0) return null
  return { from: 0, to }
}

function buildDecorations(state: EditorState): DecorationSet {
  const range = frontmatterConcealRange(state.doc.toString())
  if (!range) return Decoration.none
  // block:true removes the whole-line range so no blank gap is left behind.
  return Decoration.set([Decoration.replace({ block: true }).range(range.from, range.to)])
}

// A StateField, not a ViewPlugin: CodeMirror forbids block decorations from
// plugins ("Block decorations may not be specified via plugins"), so the
// concealment must live in editor state.
const concealField = StateField.define<DecorationSet>({
  create: (state) => buildDecorations(state),
  update: (value, tr) => (tr.docChanged ? buildDecorations(tr.state) : value),
  provide: (field) => [
    EditorView.decorations.from(field),
    // Atomic so the caret skips the concealed block instead of landing inside it.
    EditorView.atomicRanges.of((view) => view.state.field(field, false) || Decoration.none),
  ],
})

export function frontmatterConcealExtension() {
  return concealField
}
