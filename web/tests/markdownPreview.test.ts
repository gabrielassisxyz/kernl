import { describe, it, expect } from 'vitest'
import { EditorState } from '@codemirror/state'
import { markdown } from '@codemirror/lang-markdown'
import {
  collectPreviewSpecs,
  computeActiveLines,
  type PreviewSpec,
} from '../utils/markdownPreview'

// Build a parsed markdown state with the cursor at a given offset (default: end
// of doc, i.e. away from line 1) so collectPreviewSpecs runs headlessly.
function stateFor(doc: string, cursor = doc.length): EditorState {
  return EditorState.create({
    doc,
    selection: { anchor: cursor },
    extensions: [markdown()],
  })
}

function specs(doc: string, cursor?: number): PreviewSpec[] {
  const state = stateFor(doc, cursor)
  return collectPreviewSpecs(state, computeActiveLines(state))
}

const hides = (s: PreviewSpec[]) => s.filter((x) => x.kind === 'hide')
const styled = (s: PreviewSpec[], kind: PreviewSpec['kind']) => s.filter((x) => x.kind === kind)
const slice = (doc: string, spec: PreviewSpec) => doc.slice(spec.from, spec.to)

describe('computeActiveLines', () => {
  it('reports the line the cursor sits on', () => {
    const lines = computeActiveLines(stateFor('line one\nline two\nline three', 12))
    expect(lines.has(2)).toBe(true)
    expect(lines.has(1)).toBe(false)
  })
})

describe('collectPreviewSpecs — concealment off the active line', () => {
  it('hides bold markers and styles the content when the cursor is elsewhere', () => {
    // Two lines so the cursor (default: end, line 2) is off the bold on line 1.
    const doc = 'some **bold** text\nother line'
    const s = specs(doc)
    expect(hides(s).map((h) => slice(doc, h))).toEqual(['**', '**'])
    expect(styled(s, 'strong').map((x) => slice(doc, x))).toEqual(['bold'])
  })

  it('reveals the raw markers but keeps styling once the cursor is on the line', () => {
    const doc = 'some **bold** text\nother line'
    const s = specs(doc, 0) // cursor on line 1
    expect(hides(s)).toHaveLength(0)
    expect(styled(s, 'strong').map((x) => slice(doc, x))).toEqual(['bold'])
  })

  it('hides the "# " marker and styles a heading', () => {
    const doc = '# Title\nbody'
    const s = specs(doc)
    expect(hides(s).map((h) => slice(doc, h))).toEqual(['# '])
    expect(styled(s, 'h1').map((x) => slice(doc, x))).toEqual(['# Title'])
  })

  it('handles italic and inline code', () => {
    const doc = 'an *em* and `code` here\nnext'
    const s = specs(doc)
    expect(styled(s, 'emphasis').map((x) => slice(doc, x))).toEqual(['em'])
    expect(styled(s, 'code').map((x) => slice(doc, x))).toEqual(['code'])
    expect(hides(s).map((h) => slice(doc, h))).toEqual(['*', '*', '`', '`'])
  })
})

describe('collectPreviewSpecs — links vs wikilinks', () => {
  it('conceals an inline link\'s brackets and url, styling the text', () => {
    const doc = 'see [docs](https://x.com) now\nnext'
    const s = specs(doc)
    expect(styled(s, 'link').map((x) => slice(doc, x))).toEqual(['docs'])
    expect(hides(s).map((h) => slice(doc, h))).toEqual(['[', '](https://x.com)'])
  })

  it('leaves [[wikilinks]] untouched (handled by the pill decorations)', () => {
    // A wikilink parses as a reference-style Link with no URL child.
    const doc = 'a [[uuid-1|Alias]] link\nnext'
    const s = specs(doc)
    expect(styled(s, 'link')).toHaveLength(0)
    expect(hides(s)).toHaveLength(0)
  })
})

describe('collectPreviewSpecs — non-destructive', () => {
  it('never alters the document (decorations are presentation-only)', () => {
    const doc = '# Title\n**bold** *em* `code` [x](https://x.com)'
    const state = stateFor(doc)
    collectPreviewSpecs(state, computeActiveLines(state))
    expect(state.doc.toString()).toBe(doc)
  })
})
