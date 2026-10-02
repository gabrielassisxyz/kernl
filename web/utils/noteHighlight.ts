// Token colors for the notes editor.
//
// Until this existed the editor had no HighlightStyle at all, so every construct
// the live-preview walker doesn't personally decorate rendered as undifferentiated
// body text: a fenced code block read exactly like a paragraph, and a blockquote
// exactly like the line above it.
//
// Two rules from DESIGN.md shape what is here. The Accent Scarcity Rule keeps this
// palette in the neutral text stack: syntax highlighting is orientation, not signal,
// so a note is not the place to spend `primary` - the one exception is a fenced
// code block's strings, which read in the same accent-text tone as the links. The
// Token Exception Rule forbids literal colors in editor styling, so every value is
// a CSS variable.
//
// The division of labour with ./markdownPreview is deliberate: that module owns
// headings, bold, italic, inline code and inline links through its own `cm-md-*`
// marks, so those tags are absent here. Two layers styling one range is how a
// colour ends up depending on which decoration happened to win.

import { Decoration, type DecorationSet, EditorView, ViewPlugin, type ViewUpdate } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting, syntaxTree } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import type { EditorState } from '@codemirror/state'

export const noteHighlightStyle = HighlightStyle.define([
  // Every syntax marker the grammar knows - `#`, `>`, `-`, backticks, `**`, `[`,
  // and a table's pipes - carries this one tag. Dimming it is what lets source
  // mode (and the revealed active line in live mode) read as prose with quiet
  // scaffolding, instead of as text competing with punctuation.
  { tag: tags.processingInstruction, color: 'var(--color-text-dim)' },

  // Code, inline and fenced alike. The font shift is the load-bearing half: the
  // editor body is proportional, so mono is what makes a fence legible as code
  // before any background exists to frame it.
  {
    tag: tags.monospace,
    fontFamily: 'var(--font-mono-data, monospace)',
    color: 'var(--color-on-surface-variant)',
  },

  // Quoted content steps back one notch from body text. The left border and the
  // concealed `>` that complete the blockquote are decorations over ranges this
  // tag also covers, so they belong to one owner - the preview walker - and land
  // with it rather than here.
  { tag: tags.quote, color: 'var(--color-text-secondary)' },

  // A fence's language and a link's reference label are metadata about the
  // construct, never its content.
  { tag: tags.labelName, color: 'var(--color-text-muted)' },

  // `tags.url` is deliberately absent. The preview walker styles addresses as
  // links now, and both layers colouring the same characters would leave the
  // result to whichever span nested deeper. The cost is that source mode shows a
  // URL in body colour - which is what source mode is for.

  // The line through the text is the signal; dimming it as far as `text-faint`
  // on top of that reads as unreadable rather than as struck.
  { tag: tags.strikethrough, color: 'var(--color-text-muted)', textDecoration: 'line-through' },
  { tag: tags.contentSeparator, color: 'var(--color-text-dim)' },
  { tag: tags.comment, color: 'var(--color-text-faint)', fontStyle: 'italic' },

  // Code tokens, once a fenced block's info string resolves to a grammar
  // (./noteLanguage). Code owns the token dimension - the walker never
  // decorates inside a fence - and only these three get style: keywords add
  // weight but no colour (the code base colour stays), strings take the
  // accent-text tone a note's links carry, comments stay on the muted rule
  // above. Every other tag a grammar emits (numbers, operators, names) falls
  // back to the base code text: restraint here is the point (Accent Scarcity
  // Rule, and no rainbow - this one is the place token colour is expected at
  // all).
  //
  // Specs further down win on a shared range, so these sit after everything
  // they must override.
  { tag: tags.keyword, fontWeight: '600' },
  { tag: tags.string, color: 'var(--color-primary-text)' },

  // A task's `[x]` and a table's header row: both are structure the reader
  // scans past, so they stay muted rather than coloured. The explicit 400 is
  // the guard against the keyword rule above: `atom` descends from `keyword`
  // in the tag hierarchy, so without it the marker would inherit that weight
  // too.
  { tag: tags.atom, color: 'var(--color-text-muted)', fontWeight: '400' },
  { tag: tags.heading, fontWeight: '600' },
])

// A resolved code fence's tokens are styled by the nested grammar's highlight
// walk alone, and that walk never inherits the doc-level `monospace` class the
// way an ordinary leaf does - the mounted overlay walk hard-resets the inherited
// class. So the moment a fence's grammar loads, its body would flip from mono to
// the editor's proportional body font. This re-homes the body's mono and base
// colour onto the fence-body LINES (a line decoration from a ViewPlugin, which
// the plan allows): the token spans sit inside those lines and pick both up,
// and unstyled tokens - numbers, operators - stay on the code base instead of
// jumping to body text. Same two values the `monospace` rule carries, so plain
// (unknown-language) bodies render identically with or without it.
const codeBodyLine = Decoration.line({ class: 'cm-note-code-body' })

const codeBodyTheme = EditorView.theme({
  '.cm-note-code-body': {
    fontFamily: 'var(--font-mono-data, monospace)',
    color: 'var(--color-on-surface-variant)',
  },
})

// Line decorations for the bodies of fenced and indented code in the visible
// viewport. Pure over (state, ranges) so tests run it headless.
export function codeBodyLineDecorations(
  state: EditorState,
  visibleRanges: readonly { from: number; to: number }[],
): DecorationSet {
  const builder: Decoration[] = []
  for (const { from, to } of visibleRanges) {
    syntaxTree(state).iterate({
      from,
      to,
      enter: (node) => {
        if (node.name !== 'FencedCode' && node.name !== 'CodeBlock') return
        const body = node.node.getChild('CodeText')
        if (!body) return
        for (let pos = body.from; pos <= body.to; ) {
          const line = state.doc.lineAt(pos)
          builder.push(codeBodyLine.range(line.from))
          pos = line.to + 1
        }
      },
    })
  }
  // sort=true: across ranges and lines this walks in document order, but the
  // sort must not be trusted to the traversal of two separate visibleRanges.
  return Decoration.set(builder, true)
}

const codeBodyPlugin = ViewPlugin.fromClass(
  class CodeBodyDecorations {
    decorations: DecorationSet
    constructor(view: EditorView) {
      this.decorations = codeBodyLineDecorations(view.state, view.visibleRanges)
    }
    update(update: ViewUpdate) {
      if (update.docChanged || update.viewportChanged) {
        this.decorations = codeBodyLineDecorations(update.view.state, update.view.visibleRanges)
      }
    }
  },
  { decorations: (value) => value.decorations },
)

export function noteHighlighting() {
  return [syntaxHighlighting(noteHighlightStyle), codeBodyPlugin, codeBodyTheme]
}