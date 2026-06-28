// CodeMirror 6 live-preview for the notes editor: conceal markdown syntax
// markers on lines the cursor isn't on, and style the content inline so a note
// reads like rendered markdown (Obsidian-like) until you move into a line to
// edit it. The marker-collection logic is a pure function over the parsed
// syntax tree (unit-tested headless via collectPreviewSpecs); this module is the
// thin glue that turns specs into a CM6 ViewPlugin + theme.
//
// Wikilinks ([[...]]) are handled by ./wikilinkEditor, not here. The markdown
// parser sees a [[target]] as a reference-style Link with no URL child, so we
// only ever conceal links that carry an explicit URL — which leaves wikilinks
// untouched and avoids fighting the pill decorations for the same range.

import type { EditorState } from '@codemirror/state'
import {
  Decoration,
  EditorView,
  ViewPlugin,
  type DecorationSet,
  type ViewUpdate,
} from '@codemirror/view'
import { syntaxTree } from '@codemirror/language'

// A decoration intent emitted by the pure pass. `hide` removes a syntax marker
// from view; every other kind styles a content range. Keeping these as plain
// data (not Decoration objects) is what lets the collector be tested headlessly.
export type PreviewKind =
  | 'hide'
  | 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6'
  | 'strong'
  | 'emphasis'
  | 'code'
  | 'link'

export interface PreviewSpec {
  from: number
  to: number
  kind: PreviewKind
}

// Line numbers (1-based) touched by any selection range. A formatting marker is
// only concealed when its line is absent from this set — the raw-on-cursor-line
// safety valve the brief asks for.
export function computeActiveLines(state: EditorState): Set<number> {
  const lines = new Set<number>()
  for (const range of state.selection.ranges) {
    const first = state.doc.lineAt(range.from).number
    const last = state.doc.lineAt(range.to).number
    for (let n = first; n <= last; n++) lines.add(n)
  }
  return lines
}

const HEADING_LEVEL: Record<string, PreviewKind> = {
  ATXHeading1: 'h1', ATXHeading2: 'h2', ATXHeading3: 'h3',
  ATXHeading4: 'h4', ATXHeading5: 'h5', ATXHeading6: 'h6',
}

// Whether a node's line is one the cursor is on. Headings/inline marks are
// single-line in practice, so checking the start line is sufficient and cheap.
function lineActive(state: EditorState, pos: number, activeLines: Set<number>): boolean {
  return activeLines.has(state.doc.lineAt(pos).number)
}

// Walk the markdown syntax tree and emit style + (conditionally) hide specs.
// Styling is always emitted so bold reads as bold even on the active line;
// only marker concealment is gated on the line being inactive.
export function collectPreviewSpecs(state: EditorState, activeLines: Set<number>): PreviewSpec[] {
  const specs: PreviewSpec[] = []
  const doc = state.doc

  const style = (from: number, to: number, kind: PreviewKind) => {
    if (to > from) specs.push({ from, to, kind })
  }
  // Conceal a marker only when its line isn't being edited.
  const hide = (from: number, to: number) => {
    if (to > from && !lineActive(state, from, activeLines)) {
      specs.push({ from, to, kind: 'hide' })
    }
  }

  syntaxTree(state).iterate({
    enter: (node) => {
      const headingKind = HEADING_LEVEL[node.name]
      if (headingKind) {
        style(node.from, node.to, headingKind)
        const mark = node.node.getChild('HeaderMark')
        if (mark) {
          // Swallow the single space after the "#" run so the title sits flush.
          const after = doc.sliceString(mark.to, mark.to + 1) === ' ' ? mark.to + 1 : mark.to
          hide(node.from, after)
        }
        return
      }

      if (node.name === 'StrongEmphasis' || node.name === 'Emphasis') {
        const marks = node.node.getChildren('EmphasisMark')
        if (marks.length >= 2) {
          const open = marks[0]
          const close = marks[marks.length - 1]
          style(open.to, close.from, node.name === 'StrongEmphasis' ? 'strong' : 'emphasis')
          hide(open.from, open.to)
          hide(close.from, close.to)
        }
        return
      }

      if (node.name === 'InlineCode') {
        const marks = node.node.getChildren('CodeMark')
        if (marks.length >= 2) {
          const open = marks[0]
          const close = marks[marks.length - 1]
          style(open.to, close.from, 'code')
          hide(open.from, open.to)
          hide(close.from, close.to)
        }
        return
      }

      if (node.name === 'Link') {
        // Only inline links carry a URL child; reference-style links and
        // wikilinks ([[...]]) don't, so they fall through untouched.
        if (!node.node.getChild('URL')) return
        const marks = node.node.getChildren('LinkMark')
        if (marks.length >= 2) {
          const open = marks[0]
          const close = marks[1]
          style(open.to, close.from, 'link')
          hide(node.from, open.to)      // "["
          hide(close.from, node.to)     // "](url)"
        }
      }
    },
  })

  return specs
}

const KIND_CLASS: Record<Exclude<PreviewKind, 'hide'>, string> = {
  h1: 'cm-md-h1', h2: 'cm-md-h2', h3: 'cm-md-h3',
  h4: 'cm-md-h4', h5: 'cm-md-h5', h6: 'cm-md-h6',
  strong: 'cm-md-strong',
  emphasis: 'cm-md-emphasis',
  code: 'cm-md-code',
  link: 'cm-md-link',
}

const hideDeco = Decoration.replace({})

function specToDecoration(spec: PreviewSpec): Decoration {
  if (spec.kind === 'hide') return hideDeco
  return Decoration.mark({ class: KIND_CLASS[spec.kind] })
}

function buildDecorations(state: EditorState): DecorationSet {
  const specs = collectPreviewSpecs(state, computeActiveLines(state))
  const ranges = specs.map((s) => specToDecoration(s).range(s.from, s.to))
  // sort=true: specs come out of a tree walk, not in document order.
  return Decoration.set(ranges, true)
}

const livePreviewPlugin = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet
    constructor(view: EditorView) {
      this.decorations = buildDecorations(view.state)
    }
    update(update: ViewUpdate) {
      // Selection changes move the active line, so reveal/conceal must refresh
      // on selectionSet too, not just docChanged.
      if (update.docChanged || update.selectionSet || update.viewportChanged) {
        this.decorations = buildDecorations(update.view.state)
      }
    }
  },
  { decorations: (v) => v.decorations },
)

// Inline styling for the concealed-marker content. Sizes/weights track the
// editor's existing IBM-Plex look; colors come from the @theme tokens so the
// preview restyles centrally with the rest of the app (U2/U2.1).
export const livePreviewTheme = EditorView.theme({
  '.cm-md-h1': { fontSize: '1.6em', fontWeight: '600', lineHeight: '1.3' },
  '.cm-md-h2': { fontSize: '1.4em', fontWeight: '600', lineHeight: '1.3' },
  '.cm-md-h3': { fontSize: '1.2em', fontWeight: '600', lineHeight: '1.3' },
  '.cm-md-h4': { fontSize: '1.1em', fontWeight: '600' },
  '.cm-md-h5': { fontSize: '1em', fontWeight: '600' },
  '.cm-md-h6': { fontSize: '1em', fontWeight: '600', color: 'var(--color-text-muted)' },
  '.cm-md-strong': { fontWeight: '700' },
  '.cm-md-emphasis': { fontStyle: 'italic' },
  '.cm-md-code': {
    fontFamily: 'var(--font-mono-data, monospace)',
    fontSize: '0.9em',
    backgroundColor: 'color-mix(in srgb, var(--color-on-surface) 8%, transparent)',
    borderRadius: 'var(--radius-lg)',
    padding: '0 3px',
  },
  '.cm-md-link': {
    color: 'var(--color-da-accent)',
    textDecoration: 'underline',
    textUnderlineOffset: '2px',
  },
})

// Single bundle for the editor's extensions array, mirroring wikilinkExtensions.
export function livePreviewExtensions() {
  return [livePreviewPlugin, livePreviewTheme]
}
