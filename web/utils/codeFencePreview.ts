// Hides fenced code blocks' fence lines from the CodeMirror surface in live /
// reading modes: the opening ``` line and the closing ``` line disappear, and
// the block's language (the CodeInfo text the markdown parser attaches to the
// fence) shows up as a small label in the first code line's top-right corner.
// Moving the cursor anywhere inside the block brings both fence lines back.
//
// The reveal unit is the block, not the line. A fence line is only reachable by
// the cursor while the user is editing the block, so revealing per line would
// hide the opening fence again the moment the caret moves onto the second code
// line - exactly the defect Obsidian's own live preview avoids.
//
// StateField, not ViewPlugin, for the same reason ./frontmatterConceal is one:
// a hidden fence spans its line break, and @codemirror/view forbids
// replacements crossing a line break (and block decorations outright) from
// plugins. This makes the field the editor's second decoration source, side by
// side with ./markdownPreview's viewport-scoped plugin: that one keeps marks,
// line classes and single-line replacements (it can still see the viewport,
// which a StateField cannot); whatever must replace whole lines lives here and
// is what later block constructs (images, tables) will reuse.
//
// A StateField only recomputes on transactions, so a decoration set built on a
// partially-parsed tree would never be corrected when background parsing later
// finishes: fences beyond the parsed prefix would stay visible with no signal.
// The tree is therefore forced complete on every rebuild - once per build, then
// cached, and rebuilds that follow are incremental for the parser.

import { StateField, type EditorState, type Text } from '@codemirror/state'
import { Decoration, EditorView, type DecorationSet } from '@codemirror/view'
import { ensureSyntaxTree } from '@codemirror/language'
import type { SyntaxNode } from '@lezer/common'

// A decoration intent emitted by the pure pass, parallel to PreviewSpec in
// ./markdownPreview: `fence` removes a whole fence line (including its line
// break), `label` anchors on the first code line so CSS can draw the language
// in its corner without putting any text in the document.
export interface CodeFenceSpec {
  kind: 'fence' | 'label'
  from: number
  to: number
  /** The CodeInfo text; present on `label` specs only. */
  lang?: string
}

// Whether any selection head sits inside [from, to]. The head decides, not the
// anchor: dragging a selection across a block leaves its head outside, and the
// fences stay concealed while its text is selected. Head inside the block is
// what "editing it" looks like, so that is what reveals it.
function selectionHeadInside(state: EditorState, from: number, to: number): boolean {
  return state.selection.ranges.some((range) => range.head >= from && range.head <= to)
}

// Walk the markdown syntax tree and emit one spec per concealed fence line plus
// the label anchors. Only `FencedCode`: an indented `CodeBlock` has no fence
// lines. The block tint itself is the walker's (`codeBlock` line class), so the
// two decoration sources never own the same thing.
export function collectCodeFenceSpecs(state: EditorState): CodeFenceSpec[] {
  const doc = state.doc
  const tree = ensureSyntaxTree(state, doc.length)
  const specs: CodeFenceSpec[] = []
  tree.iterate({
    enter: (nodeRef) => {
      if (nodeRef.name !== 'FencedCode') return
      if (selectionHeadInside(state, nodeRef.from, nodeRef.to)) return false
      collectFence(doc, nodeRef.node, specs)
      // Marks and code text only - no fence nests inside another.
      return false
    },
  })
  return specs
}

function collectFence(doc: Text, node: SyntaxNode, specs: CodeFenceSpec[]): void {
  const marks = node.getChildren('CodeMark')
  const openingLine = doc.lineAt(marks[0].from)
  // The whole line, indentation included, plus the break: stopping short would
  // leave the fence's leading whitespace behind as a blank-looking line.
  specs.push({
    kind: 'fence',
    from: openingLine.from,
    to: Math.min(openingLine.to + 1, doc.length),
  })
  // No closing mark before the document ends is the unclosed fence: only the
  // opening line can go, the code that follows is all there is.
  const closingMark = marks.length >= 2 ? marks[marks.length - 1] : null
  const closingLine = closingMark ? doc.lineAt(closingMark.from) : null
  if (closingLine) {
    specs.push({
      kind: 'fence',
      from: closingLine.from,
      to: Math.min(closingLine.to + 1, doc.length),
    })
  }
  // The label lands on the first code line, which must exist and must not be
  // the closing fence itself: a bare ``` pair with nothing between them draws
  // none.
  const firstCodePos = openingLine.to + 1
  const hasCodeLine = closingLine
    ? firstCodePos < closingLine.from
    : firstCodePos <= node.to
  if (!hasCodeLine) return
  const info = node.getChild('CodeInfo')
  if (!info) return
  const lang = doc.sliceString(info.from, info.to).trim()
  if (lang) specs.push({ kind: 'label', from: firstCodePos, to: firstCodePos, lang })
}

// One instance for every fence line; identical specs would churn a fresh
// Decoration object per rebuild otherwise. `inclusiveEnd: false` is load-bearing:
// a block replace defaults to closing with a huge side (+200000001), and the
// spans algorithm then swallows any decoration that opens exactly where the
// block ends - which is precisely where the label's line decoration sits (the
// first code line starts at the concealed opening fence's line break). Closing
// non-inclusive orders the block's end event before the label's (Side.Line,
// -200000000), so both reach the view builder; the start stays inclusive, which
// is what keeps the hidden fence from leaving an empty line behind.
const fenceHide = Decoration.replace({ block: true, inclusiveEnd: false })

const labelDecorations = new Map<string, Decoration>()
function labelDecoration(lang: string): Decoration {
  let deco = labelDecorations.get(lang)
  if (!deco) {
    deco = Decoration.line({
      class: 'cm-code-fence-lang',
      attributes: { 'data-code-lang': lang },
    })
    labelDecorations.set(lang, deco)
  }
  return deco
}

function buildDecorations(state: EditorState): DecorationSet {
  const specs = collectCodeFenceSpecs(state)
  if (specs.length === 0) return Decoration.none
  // sort=true is not optional: a fence hide ends exactly where the label line
  // decoration begins, and RangeSet orders by (from, startSide). The walker's
  // spec-to-decoration pass carries the same trap in its own comment.
  return Decoration.set(
    specs.map((s) =>
      s.kind === 'fence'
        ? fenceHide.range(s.from, s.to)
        : labelDecoration(s.lang ?? '').range(s.from, s.to),
    ),
    true,
  )
}

// Selection changes matter as much as document changes - the reveal flips on
// cursor movement alone. `tr.selection` is undefined on transactions that left
// the selection alone, so the two cases collapse into one guard.
export const codeFenceField = StateField.define<DecorationSet>({
  create: buildDecorations,
  update: (value, tr) => (tr.docChanged || tr.selection ? buildDecorations(tr.state) : value),
  provide: (field) => EditorView.decorations.from(field),
})

// The label is a pseudo-element over the line decoration's data attribute, so
// the language never becomes part of the document text: it cannot be selected
// with the caret, and `user-select: none` plus `pointer-events: none` keep
// browsers from offering it on copy or reacting to clicks on it. It rides in
// the corner of the block tint the walker draws, in the same neutral text
// palette as every other preview style - accent scarcity applies here too.
export const codeFencePreviewTheme = EditorView.theme({
  '.cm-code-fence-lang': {
    position: 'relative',
  },
  '.cm-code-fence-lang::after': {
    content: 'attr(data-code-lang)',
    position: 'absolute',
    top: '0',
    right: '0',
    fontFamily: 'var(--font-mono-data, monospace)',
    fontSize: '0.72em',
    lineHeight: '1.2',
    padding: '0 2px',
    color: 'var(--color-text-dim)',
    userSelect: 'none',
    pointerEvents: 'none',
  },
})

export function codeFencePreviewExtension() {
  return [codeFenceField, codeFencePreviewTheme]
}