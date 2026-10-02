import { describe, it, expect } from 'vitest'
import { EditorState } from '@codemirror/state'
import { Decoration, EditorView, type DecorationSet } from '@codemirror/view'
import { mount } from '@vue/test-utils'
import { vi, afterEach } from 'vitest'
import { noteMarkdown } from '../utils/noteLanguage'
import {
  codeFenceField,
  collectCodeFenceSpecs,
  type CodeFenceSpec,
} from '../utils/codeFencePreview'
import MarkdownEditor from '../components/notes/MarkdownEditor.vue'
import { useEditorSettings } from '../composables/useEditorSettings'

// Build a parsed markdown state with the cursor at a given offset. The parser
// is the same dialect the editor uses (noteMarkdown), so what these tests see
// is what the editor sees.
function stateFor(doc: string, cursor = doc.length): EditorState {
  return EditorState.create({
    doc,
    selection: { anchor: cursor },
    extensions: [noteMarkdown()],
  })
}

function specs(doc: string, cursor?: number): CodeFenceSpec[] {
  return collectCodeFenceSpecs(stateFor(doc, cursor))
}

const fences = (s: CodeFenceSpec[]) => s.filter((x) => x.kind === 'fence')
const labels = (s: CodeFenceSpec[]) => s.filter((x) => x.kind === 'label')
const covered = (doc: string, s: CodeFenceSpec) => doc.slice(s.from, s.to)

describe('collectCodeFenceSpecs - concealment off the cursor', () => {
  const doc = '```python\nx = 1\ny = 2\n```\nafter'

  it('hides both fence lines, each including its trailing line break, with the cursor outside', () => {
    const s = specs(doc) // cursor at end, on "after"
    expect(fences(s).map((x) => covered(doc, x))).toEqual(['```python\n', '```\n'])
    expect(labels(s).map((x) => x.lang)).toEqual(['python'])
  })

  it('labels the FIRST code line with the CodeInfo text', () => {
    const s = specs(doc)
    expect(labels(s)).toHaveLength(1)
    expect(labels(s)[0].from).toBe(doc.indexOf('x = 1'))
    expect(labels(s)[0].from).toBe(labels(s)[0].to)
  })

  it('reveals both fences and drops the label with the cursor on the first code line', () => {
    const s = specs(doc, doc.indexOf('x = 1'))
    expect(fences(s)).toHaveLength(0)
    expect(labels(s)).toHaveLength(0)
  })

  it('reveals with the cursor on a middle code line too', () => {
    const s = specs(doc, doc.indexOf('y = 2'))
    expect(fences(s)).toHaveLength(0)
    expect(labels(s)).toHaveLength(0)
  })

  it('reveals with the cursor on the opening fence line', () => {
    expect(fences(specs(doc, doc.indexOf('python')))).toHaveLength(0)
  })

  it('reveals with the cursor on the closing fence line', () => {
    expect(fences(specs(doc, doc.indexOf('```') + 4))).toHaveLength(0)
  })

  it('hides a fence with no language and draws no label', () => {
    const doc2 = '```\nx = 1\n```\nafter'
    const s = specs(doc2)
    expect(fences(s).map((x) => covered(doc2, x))).toEqual(['```\n', '```\n'])
    expect(labels(s)).toHaveLength(0)
  })
})

describe('collectCodeFenceSpecs - unclosed fences', () => {
  const doc = 'intro\n\n```python\nx = 1\nno close'

  it('hides only the opening line when the fence never closes', () => {
    const s = specs(doc, 0) // cursor on "intro", outside the block
    expect(fences(s).map((x) => covered(doc, x))).toEqual(['```python\n'])
    expect(labels(s).map((x) => x.lang)).toEqual(['python'])
  })

  it('shows everything when the cursor is inside an unclosed fence', () => {
    const s = specs(doc, doc.indexOf('x = 1'))
    expect(fences(s)).toHaveLength(0)
    expect(labels(s)).toHaveLength(0)
  })

  it('handles a one-line fence at the end of the document', () => {
    const doc2 = 'before\n\n```py'
    const s = specs(doc2, 0)
    expect(fences(s).map((x) => covered(doc2, x))).toEqual(['```py'])
    expect(labels(s)).toHaveLength(0)
  })
})

describe('collectCodeFenceSpecs - block-scoped reveal across fences', () => {
  const doc = 'two:\n\n```py\na\n```\n\n```bash\nb\n```\n\nend'

  it('with the cursor in the second block, the first stays hidden', () => {
    const s = specs(doc, doc.indexOf('```bash') + 4)
    // F2 (containing the head) is revealed; exactly F1 keeps its decorations.
    expect(fences(s).map((x) => covered(doc, x))).toEqual(['```py\n', '```\n'])
    expect(labels(s).map((x) => x.lang)).toEqual(['py'])
  })

  it('with the cursor in the first block, the second stays hidden', () => {
    const s = specs(doc, doc.indexOf('a'))
    const f = fences(s)
    expect(f.map((x) => covered(doc, x))).toEqual(['```bash\n', '```\n'])
    expect(labels(s).map((x) => x.lang)).toEqual(['bash'])
  })

  it('reveals on a selection whose head is inside the block, anchor outside', () => {
    const state = EditorState.create({
      doc,
      selection: { anchor: 0, head: doc.indexOf('```bash') + 4 },
      extensions: [noteMarkdown()],
    })
    // The block under the head (F2) reveals; the untouched one stays hidden.
    expect(fences(collectCodeFenceSpecs(state)).map((x) => covered(doc, x))).toEqual(['```py\n', '```\n'])
  })

  it('conceals when only the anchor is inside and the head is out (the head decides)', () => {
    const state = EditorState.create({
      doc,
      selection: { anchor: doc.indexOf('```bash') + 4, head: 0 },
      extensions: [noteMarkdown()],
    })
    // All four fence lines, both blocks: no head in a block means no reveal.
    expect(fences(collectCodeFenceSpecs(state))).toHaveLength(4)
  })
})

describe('collectCodeFenceSpecs - scope', () => {
  it('ignores indented code blocks, which have no fence lines to hide', () => {
    expect(specs('intro\n\n    indented = True\n\nafter', 0)).toEqual([])
  })

  it('returns nothing for a document without fences', () => {
    expect(specs('plain text\nsecond paragraph')).toEqual([])
  })

  it('never labels a fence whose only line after the opening is the closing fence', () => {
    const doc = 'intro\n\n```py\n```\nafter'
    const s = specs(doc, 0)
    expect(fences(s)).toHaveLength(2)
    expect(labels(s)).toHaveLength(0)
  })
})

// The glue between the pure collector and the editor: a StateField producing a
// DecorationSet. Block replaces may never come from a ViewPlugin, which is why
// this layer exists at all.
function fieldDecorations(doc: string, cursor?: number, state?: EditorState): DecorationSet {
  const s = state ?? EditorState.create({
    doc,
    selection: { anchor: cursor ?? doc.length },
    extensions: [noteMarkdown(), codeFenceField],
  })
  return s.field(codeFenceField)
}

function rangesOf(set: DecorationSet): { from: number; to: number; deco: Decoration }[] {
  const out: { from: number; to: number; deco: Decoration }[] = []
  const iter = set.iter()
  while (iter.value) {
    out.push({ from: iter.from, to: iter.to, deco: iter.value as Decoration })
    iter.next()
  }
  return out
}

const isBlockReplace = (deco: Decoration) => (deco.spec as { block?: boolean }).block === true

describe('codeFenceField (StateField glue)', () => {
  const doc = '```python\nx = 1\ny = 2\n```\nafter'

  it('emits block replacements for the fence lines and a line decoration for the label', () => {
    const ranges = rangesOf(fieldDecorations(doc))
    expect(ranges).toHaveLength(3)
    // Sorted by from: the opening hide, then the label anchored on the first
    // code line, then the closing hide.
    const [open, label, close] = ranges
    expect(isBlockReplace(open.deco)).toBe(true)
    expect(isBlockReplace(close.deco)).toBe(true)
    expect(doc.slice(open.from, open.to)).toBe('```python\n')
    expect(doc.slice(close.from, close.to)).toBe('```\n')
    expect(isBlockReplace(label.deco)).toBe(false)
    const spec = label.deco.spec as { class?: string; attributes?: Record<string, string> }
    expect(spec.class).toContain('cm-code-fence-lang')
    expect(spec.attributes?.['data-code-lang']).toBe('python')
    expect(label.from).toBe(doc.indexOf('x = 1'))
    expect(label.to).toBe(label.from)
  })

  it('recomputes when the selection moves into the block, and back out again', () => {
    const base = EditorState.create({
      doc,
      selection: { anchor: doc.length },
      extensions: [noteMarkdown(), codeFenceField],
    })
    const intoBlock = base.update({ selection: { anchor: doc.indexOf('x = 1') } })
    expect(rangesOf(intoBlock.state.field(codeFenceField))).toHaveLength(0)
    const backOut = intoBlock.state.update({ selection: { anchor: doc.length } })
    expect(rangesOf(backOut.state.field(codeFenceField))).toHaveLength(3)
  })

  it('recomputes on document changes', () => {
    const base = EditorState.create({
      doc,
      selection: { anchor: doc.length },
      extensions: [noteMarkdown(), codeFenceField],
    })
    const edited = base.update({ changes: { from: doc.length, insert: 'more' } })
    const ranges = rangesOf(edited.state.field(codeFenceField))
    expect(ranges).toHaveLength(3)
    expect(edited.state.doc.sliceString(ranges[0].from, ranges[0].to)).toBe('```python\n')
  })

  it('yields an empty set for a document without fences', () => {
    expect(fieldDecorations('plain text\nsecond paragraph').size).toBe(0)
  })
})

// End-to-end through the mounted component: proves the field is registered
// beside frontmatterConceal and that the DOM really loses the fence lines.
const EDITOR_DOC = '```python\nx = 1\ny = 2\n```\n\nafter\n'

const stubVault = () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    if (url.startsWith('/api/vault/file')) {
      return new Response(EDITOR_DOC, {
        status: 200,
        headers: { 'Last-Modified': 'Wed, 12 Aug 2026 10:00:00 GMT' },
      })
    }
    return new Response('[]', { status: 200 })
  }))
}

const viewOf = (wrapper: ReturnType<typeof mount>) => {
  const dom = wrapper.element.querySelector('.cm-editor')
  const view = dom ? EditorView.findFromDOM(dom as HTMLElement) : null
  if (!view) throw new Error('no CodeMirror view mounted')
  return view
}

// CodeMirror paints decoration rebuilds on requestAnimationFrame, so a dispatch
// is not visible to DOM assertions until one frame has passed.
const flushView = async (wrapper: ReturnType<typeof mount>) => {
  await new Promise((resolve) => setTimeout(resolve, 30))
  await wrapper.vm.$nextTick()
}

describe('code fence field in the mounted editor', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    useEditorSettings().settings.viewMode = 'live'
  })

  it('reads without fence lines off the cursor, with the label attached to the first code line', async () => {
    stubVault()
    const wrapper = mount(MarkdownEditor, { props: { path: 'notes/x.md' }, attachTo: document.body })
    await new Promise((resolve) => setTimeout(resolve, 0))
    await wrapper.vm.$nextTick()
    const view = viewOf(wrapper)

    view.dispatch({ selection: { anchor: view.state.doc.length } })
    await flushView(wrapper)

    const content = view.contentDOM
    expect(content.textContent).not.toContain('```')
    const labelled = content.querySelector('[data-code-lang]') as HTMLElement | null
    expect(labelled).not.toBeNull()
    expect(labelled!.getAttribute('data-code-lang')).toBe('python')
    expect(labelled!.textContent).toContain('x = 1')
    // Both decoration sources coexist on that line: the walker's block tint and
    // the field's label.
    expect(labelled!.className).toContain('cm-md-code-block')

    wrapper.unmount()
  })

  it('shows the raw fences again when the cursor moves into the block', async () => {
    stubVault()
    const wrapper = mount(MarkdownEditor, { props: { path: 'notes/x.md' }, attachTo: document.body })
    await new Promise((resolve) => setTimeout(resolve, 0))
    await wrapper.vm.$nextTick()
    const view = viewOf(wrapper)

    view.dispatch({ selection: { anchor: view.state.doc.toString().indexOf('x = 1') } })
    await flushView(wrapper)

    const content = view.contentDOM
    expect(content.textContent).toContain('```python')
    expect(content.querySelector('[data-code-lang]')).toBeNull()

    wrapper.unmount()
  })

  it('stays concealed in reading mode, which has no cursor to reveal anything', async () => {
    stubVault()
    const wrapper = mount(MarkdownEditor, { props: { path: 'notes/x.md' }, attachTo: document.body })
    await new Promise((resolve) => setTimeout(resolve, 0))
    await wrapper.vm.$nextTick()
    const view = viewOf(wrapper)

    // Off the block first: position 0 sits ON the opening fence, which would
    // legitimately reveal it and make the mode flip prove nothing.
    view.dispatch({ selection: { anchor: view.state.doc.length } })
    useEditorSettings().settings.viewMode = 'reading'
    await flushView(wrapper)

    // The concealment compartment is reconfigured on mode flips; the field
    // rebuilds against the (unchanged) selection and hides the fences again.
    expect(view.contentDOM.textContent).not.toContain('```')
    expect(view.contentDOM.querySelector('[data-code-lang]')).not.toBeNull()

    wrapper.unmount()
  })
})