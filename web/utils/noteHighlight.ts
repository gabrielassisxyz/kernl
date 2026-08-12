// Token colors for the notes editor.
//
// Until this existed the editor had no HighlightStyle at all, so every construct
// the live-preview walker doesn't personally decorate rendered as undifferentiated
// body text: a fenced code block read exactly like a paragraph, and a blockquote
// exactly like the line above it.
//
// Two rules from DESIGN.md shape what is here. The Accent Scarcity Rule keeps this
// palette in the neutral text stack: syntax highlighting is orientation, not signal,
// so a note is not the place to spend `primary`. The Token Exception Rule forbids
// literal colors in editor styling, so every value is a CSS variable.
//
// The division of labour with ./markdownPreview is deliberate: that module owns
// headings, bold, italic, inline code and inline links through its own `cm-md-*`
// marks, so those tags are absent here. Two layers styling one range is how a
// colour ends up depending on which decoration happened to win.

import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'

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

  // A task's `[x]` and a table's header row: both are structure the reader scans
  // past, so they get weight and mutedness rather than colour.
  { tag: tags.atom, color: 'var(--color-text-muted)' },
  { tag: tags.heading, fontWeight: '600' },
])

export function noteHighlighting() {
  return syntaxHighlighting(noteHighlightStyle)
}
