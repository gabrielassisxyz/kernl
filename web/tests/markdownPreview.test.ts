import { describe, it, expect } from 'vitest'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { undo, history } from '@codemirror/commands'
import { syntaxTree } from '@codemirror/language'
import { noteMarkdown } from '../utils/noteLanguage'
import {
  collectPreviewSpecs,
  computeActiveLines,
  previewDecorations,
  livePreviewExtensions,
  TaskCheckboxWidget,
  linkHref,
  type PreviewSpec,
  type PreviewKind,
} from '../utils/markdownPreview'

// Build a parsed markdown state with the cursor at a given offset (default: end
// of doc, i.e. away from line 1) so collectPreviewSpecs runs headlessly.
function stateFor(doc: string, cursor = doc.length): EditorState {
  return EditorState.create({
    doc,
    selection: { anchor: cursor },
    extensions: [noteMarkdown()],
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

describe('collectPreviewSpecs - concealment off the active line', () => {
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

describe('collectPreviewSpecs - links vs wikilinks', () => {
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

describe('collectPreviewSpecs - non-destructive', () => {
  it('never alters the document (decorations are presentation-only)', () => {
    const doc = '# Title\n**bold** *em* `code` [x](https://x.com)'
    const state = stateFor(doc)
    collectPreviewSpecs(state, computeActiveLines(state))
    expect(state.doc.toString()).toBe(doc)
  })
})

// The dialect is a load-bearing choice, not a default: under lang-markdown's
// CommonMark base these constructs produce no nodes at all, so every decoration
// built for them would silently do nothing.
describe('noteMarkdown dialect', () => {
  const nodeNames = (doc: string): Set<string> => {
    const state = EditorState.create({ doc, extensions: [noteMarkdown()] })
    const names = new Set<string>()
    syntaxTree(state).iterate({ enter: (node) => { names.add(node.name) } })
    return names
  }

  it('parses the GFM constructs the live preview has to reach', () => {
    const names = nodeNames([
      '~~struck~~',
      '',
      '| a | b |',
      '| --- | --- |',
      '| 1 | 2 |',
      '',
      '- [x] task',
      '',
    ].join('\n'))

    expect(names.has('Strikethrough')).toBe(true)
    expect(names.has('Table')).toBe(true)
    expect(names.has('TableCell')).toBe(true)
    expect(names.has('TaskMarker')).toBe(true)
  })

  it('keeps parsing the CommonMark constructs the preview already styles', () => {
    const names = nodeNames('# H\n\n**b** *i* `c` [x](https://e.com)\n\n> q\n\n---\n')

    for (const expected of ['ATXHeading1', 'StrongEmphasis', 'Emphasis', 'InlineCode', 'Link', 'Blockquote', 'HorizontalRule']) {
      expect(names.has(expected)).toBe(true)
    }
  })
})

describe('collectPreviewSpecs - bounded to the ranges it is given', () => {
  const doc = '**near** the top\n' + 'filler\n'.repeat(200) + '**far** at the bottom'
  const firstScreen = [{ from: 0, to: 40 }]

  it('ignores constructs outside the range, so cost tracks the viewport', () => {
    const state = stateFor(doc, 0)
    const inRange = collectPreviewSpecs(state, new Set(), firstScreen)

    expect(styled(inRange, 'strong').map((x) => slice(doc, x))).toEqual(['near'])
  })

  it('still sees everything when no range is given', () => {
    const state = stateFor(doc, 0)
    const whole = collectPreviewSpecs(state, new Set())

    expect(styled(whole, 'strong').map((x) => slice(doc, x))).toEqual(['near', 'far'])
  })

  it('emits a construct once when it straddles two ranges', () => {
    const straddling = 'a **bold** b'
    const state = stateFor(straddling, 0)
    const split = collectPreviewSpecs(state, new Set(), [{ from: 0, to: 6 }, { from: 6, to: straddling.length }])

    expect(styled(split, 'strong')).toHaveLength(1)
  })
})

// The decorations were built for two years by a call that carried `sort: true`.
// Dropping it does not fail a spec test - collectPreviewSpecs is still perfectly
// correct - it throws inside RangeSet, and CodeMirror answers by disabling the
// plugin, so every note renders raw. Nothing below the spec layer noticed.
describe('previewDecorations', () => {
  it('builds a usable decoration set for constructs that share a start offset', () => {
    // The heading emits a mark over the whole line AND a replace over "# ", both
    // starting at the same position.
    const doc = '# Title\n**bold** and [x](https://e.com)\n\n---\n\n```py\nx = 1\n```\n\nSetext\n======\nplain'
    const state = stateFor(doc, doc.length)

    const deco = previewDecorations(state, true)
    expect(deco.size).toBeGreaterThan(0)
  })
})

describe('collectPreviewSpecs - block structure', () => {
  const lineKinds = (s: PreviewSpec[], kind: PreviewKind) => s.filter((x) => x.kind === kind)

  it('sizes a setext title without blowing up its underline', () => {
    const doc = 'The title\n=========\n\nbody'
    const s = specs(doc)
    const h1 = styled(s, 'h1')

    expect(h1.map((x) => slice(doc, x))).toEqual(['The title'])
  })

  it('turns a horizontal rule into a line, and conceals the dashes off-cursor', () => {
    const doc = 'before\n\n---\n\nafter'
    const s = specs(doc)

    expect(lineKinds(s, 'rule')).toHaveLength(1)
    expect(hides(s).map((h) => slice(doc, h))).toEqual(['---'])
  })

  it('collapses back to raw dashes with the cursor on it, showing no rule as well', () => {
    const doc = 'before\n\n---\n\nafter'
    const s = specs(doc, doc.indexOf('---') + 1)

    // Drawing the rule AND revealing its markup renders the same thing twice.
    expect(lineKinds(s, 'rule')).toHaveLength(0)
    expect(hides(s)).toHaveLength(0)
  })

  it('marks every line of a fenced block, fences included', () => {
    const doc = '```python\nx = 1\ny = 2\n```\nafter'
    expect(lineKinds(specs(doc), 'codeBlock')).toHaveLength(4)
  })

  it('marks an indented code block too', () => {
    const doc = 'intro\n\n    indented = True\n\nafter'
    expect(lineKinds(specs(doc), 'codeBlock')).toHaveLength(1)
  })

  it('only marks the on-screen part of a code block longer than the viewport', () => {
    const doc = '```\n' + 'line\n'.repeat(300) + '```\n'
    const s = collectPreviewSpecs(stateFor(doc, 0), new Set(), [{ from: 0, to: 60 }])

    const marked = lineKinds(s, 'codeBlock').length
    expect(marked).toBeGreaterThan(0)
    expect(marked).toBeLessThan(30)
  })
})

describe('collectPreviewSpecs - inline markup', () => {
  it('drops the backslash of an escape and keeps the character it protects', () => {
    const doc = 'a \\*not italic\\* b\nnext'
    const s = specs(doc)

    expect(hides(s).map((h) => slice(doc, h))).toEqual(['\\', '\\'])
    // The escaped asterisks must not have been read as emphasis.
    expect(styled(s, 'emphasis')).toHaveLength(0)
  })

  it('hides the ~~ of a strikethrough without claiming its styling', () => {
    const doc = 'it is ~~gone~~ now\nnext'
    const s = specs(doc)

    expect(hides(s).map((h) => slice(doc, h))).toEqual(['~~', '~~'])
  })

  it('strips the angle brackets of an autolink and styles the address', () => {
    const doc = 'see <https://example.com> there\nnext'
    const s = specs(doc)

    expect(styled(s, 'link').map((x) => slice(doc, x))).toEqual(['https://example.com'])
    expect(hides(s).map((h) => slice(doc, h))).toEqual(['<', '>'])
  })

  it('styles a bare URL, which GFM turns into a link', () => {
    const doc = 'go to https://example.com/bare now\nnext'
    expect(styled(specs(doc), 'link').map((x) => slice(doc, x))).toEqual(['https://example.com/bare'])
  })

  it('does not style the url inside an inline link twice', () => {
    const doc = 'see [docs](https://example.com) now\nnext'
    // Only the link text: the address is part of the markup already concealed.
    expect(styled(specs(doc), 'link').map((x) => slice(doc, x))).toEqual(['docs'])
  })
})

describe('collectPreviewSpecs - Obsidian comments', () => {
  const comments = (doc: string) => styled(specs(doc), 'comment').map((x) => slice(doc, x))

  it('dims an inline comment', () => {
    expect(comments('before %%hidden note%% after\nnext')).toEqual(['%%hidden note%%'])
  })

  it('dims a comment spanning several lines', () => {
    const doc = 'before\n\n%%\nnote to self\n%%\n\nafter'
    expect(comments(doc)).toEqual(['%%\nnote to self\n%%'])
  })

  it('leaves shell parameter expansion inside a code block alone', () => {
    const doc = 'before\n\n```bash\necho "${var%%pattern}" "${x%%y}"\n```\n\nafter'
    expect(comments(doc)).toEqual([])
  })
})

describe('linkHref', () => {
  it('accepts what a note may open', () => {
    expect(linkHref('https://example.com')).toBe('https://example.com')
    expect(linkHref('http://example.com')).toBe('http://example.com')
    expect(linkHref('mailto:a@b.com')).toBe('mailto:a@b.com')
    expect(linkHref('someone@example.com')).toBe('mailto:someone@example.com')
  })

  it('refuses a scheme that would run code, whoever wrote the note', () => {
    expect(linkHref('javascript:alert(1)')).toBeUndefined()
    expect(linkHref('JavaScript:alert(1)')).toBeUndefined()
    expect(linkHref('data:text/html,<script>x</script>')).toBeUndefined()
    expect(linkHref('vbscript:msgbox')).toBeUndefined()
  })

  it('leaves a link with nowhere to go unopenable but still styled', () => {
    expect(linkHref('AGENTS.md')).toBeUndefined()
    expect(linkHref('#jump-to-tables')).toBeUndefined()
  })
})

describe('collectPreviewSpecs - link targets', () => {
  const hrefOf = (doc: string) => styled(specs(doc), 'link').map((x) => x.href)

  it('carries the address of an inline link, not its text', () => {
    expect(hrefOf('see [docs](https://example.com) now\nnext')).toEqual(['https://example.com'])
  })

  it('carries no address for a javascript: link', () => {
    expect(hrefOf('see [x](javascript:alert(1)) now\nnext')).toEqual([undefined])
  })

  it('carries the address of an autolink and of a bare url', () => {
    expect(hrefOf('a <https://one.example> b\nnext')).toEqual(['https://one.example'])
    expect(hrefOf('a https://two.example/x b\nnext')).toEqual(['https://two.example/x'])
  })
})

describe('collectPreviewSpecs - task checkboxes', () => {
  const taskSpecs = (doc: string, cursor?: number) => styled(specs(doc, cursor), 'task')

  it('replaces the whole "- [ ]" run with a checkbox spec off the cursor line', () => {
    const doc = '- [ ] pending\nother line'
    const s = taskSpecs(doc)
    expect(s).toHaveLength(1)
    // ListMark start through TaskMarker end: the dash and the brackets all go.
    expect(slice(doc, s[0])).toBe('- [ ]')
    expect(s[0].checked).toBe(false)
    expect(s[0].toggleAt).toBe(doc.indexOf('[') + 1)
    expect(styled(specs(doc), 'bullet')).toHaveLength(0)
  })

  it('keeps the raw markup when the cursor is on the item line, no widget drawn', () => {
    const doc = '- [ ] pending\nother line'
    // Nothing for this line: no checkbox spec, and no bullet either.
    expect(taskSpecs(doc, 2)).toHaveLength(0)
  })

  it('reads the box state from [x], [X] and [ ]', () => {
    const doc = '- [x] done\n- [X] ALSO DONE\n- [ ] todo\nzzz'
    expect(taskSpecs(doc).map((x) => x.checked)).toEqual([true, true, false])
  })

  it('covers ListMark through TaskMarker for a task nested at depth 2', () => {
    const doc = '- [ ] top\n    - [x] nested\nzzz'
    // The TaskMarker is a grandchild of the ListItem (Task > TaskMarker); a
    // lookup among the direct children would find nothing at any depth.
    expect(taskSpecs(doc).map((x) => slice(doc, x))).toEqual(['- [ ]', '- [x]'])
  })

  it('reads a checked item text as done-styling through the end of its line', () => {
    const doc = '- [x] done\nzzz'
    const done = styled(specs(doc), 'taskDone')
    expect(done).toHaveLength(1)
    // From the TaskMarker's close to the line's end: the whole item text.
    expect(slice(doc, done[0])).toBe(' done')
    expect(done[0].to).toBe(doc.indexOf('\n'))
  })

  it('leaves a pending item without done-styling', () => {
    expect(styled(specs('- [ ] pending\nzzz'), 'taskDone')).toHaveLength(0)
  })
})

describe('previewDecorations - task checkbox widget', () => {
  const taskWidgets = (doc: string, cursor?: number): TaskCheckboxWidget[] => {
    const state = stateFor(doc, cursor)
    const found: TaskCheckboxWidget[] = []
    // deco.spec is the options object Decoration.replace was called with.
    previewDecorations(state, true).between(0, doc.length, (_from, _to, deco) => {
      if (deco.spec?.widget instanceof TaskCheckboxWidget) found.push(deco.spec.widget)
    })
    return found
  }

  it('renders an <input type=checkbox> whose checked follows the box character', () => {
    const doc = '- [x] done\n- [ ] pending\nzzz'
    const widgets = taskWidgets(doc)
    expect(widgets).toHaveLength(2)
    const doms = widgets.map((w) => w.toDOM())
    for (const dom of doms) {
      const box = dom.querySelector('input')
      expect(box?.tagName).toBe('INPUT')
      expect(box?.getAttribute('type')).toBe('checkbox')
    }
    expect(doms[0].querySelector('input')?.checked).toBe(true)
    expect(doms[1].querySelector('input')?.checked).toBe(false)
  })

  it('draws no checkbox widget on the line the cursor sits on', () => {
    const doc = '- [x] done\n- [ ] pending\nzzz'
    // Only the cursor line's box collapses; the other item keeps its widget.
    const widgets = taskWidgets(doc, 2)
    expect(widgets).toHaveLength(1)
    // The survivor is the pending item's box on line 2.
    expect(widgets[0].checked).toBe(false)
    expect(widgets[0].toggleAt).toBeGreaterThanOrEqual(doc.indexOf('\n'))
  })
})

describe('task checkbox interactions', () => {
  // A real view: the widget edits the document only through CodeMirror, and the
  // undo guarantee comes from history() - the extension the app editor mounts.
  const viewWithTasks = (doc: string) => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    return new EditorView({
      state: EditorState.create({
        doc,
        // Cursor at the end, like stateFor: position 0 would land on line 1 and
        // collapse its checkbox before any click.
        selection: { anchor: doc.length },
        // The parser is load-bearing: without noteMarkdown the syntax tree has
        // no ListMark/Task, the walker decorates nothing, and no widget is built.
        extensions: [noteMarkdown(), livePreviewExtensions(true), history()],
      }),
      parent: container,
    })
  }

  const click = (box: HTMLInputElement | null) =>
    box?.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))

  it('flips one character per click, and one undo reverts exactly the last one', () => {
    // The trailing filler keeps the cursor (at doc end) off both task lines -
    // an item under the cursor shows its raw `- [x]` and has no widget.
    const start = '- [ ] buy\n- [x] done\nzzz'
    const view = viewWithTasks(start)
    try {
      const boxes = () => Array.from(view.dom.querySelectorAll<HTMLInputElement>('.cm-md-task-check input'))
      expect(boxes()).toHaveLength(2)
      const caret = view.state.selection.main.head

      click(boxes()[0])
      expect(view.state.doc.toString()).toBe('- [x] buy\n- [x] done\nzzz')
      // The click must not move the caret onto the item line: that would
      // collapse the concealment and destroy the checkbox mid-click.
      expect(view.state.selection.main.head).toBe(caret)

      click(boxes()[1])
      expect(view.state.doc.toString()).toBe('- [x] buy\n- [ ] done\nzzz')

      // One undo reverts exactly the last click: the clicks are isolated history
      // events, not one grouped change.
      undo(view)
      expect(view.state.doc.toString()).toBe('- [x] buy\n- [x] done\nzzz')

      undo(view)
      expect(view.state.doc.toString()).toBe(start)
    } finally {
      view.destroy()
      view.dom.remove()
    }
  })
})

describe('collectPreviewSpecs - list bullets', () => {
  const bullets = (doc: string, cursor?: number) =>
    specs(doc, cursor).filter((x) => x.kind === 'bullet').map((x) => slice(doc, x))

  it('replaces every unordered marker spelling', () => {
    expect(bullets('- one\n* two\n+ three\nend')).toEqual(['-', '*', '+'])
  })

  it('keeps the bullet with the cursor on the line, unlike every other marker', () => {
    // Flipping back to a dash would make the line jump for no editable gain.
    expect(bullets('- one\n- two', 2)).toEqual(['-', '-'])
  })

  it('leaves ordered markers alone, their number being the content', () => {
    expect(bullets('1. one\n2. two\nend')).toEqual([])
  })

  it('gives a task item no bullet: its checkbox replaces the dash with it', () => {
    // The checkbox spec spans ListMark through TaskMarker, dash included - the
    // walker's task exception ended when the checkbox shipped.
    expect(bullets('- [x] done\n- [ ] pending\nend')).toEqual([])
  })

  it('replaces markers at every nesting depth', () => {
    expect(bullets('- top\n    - nested\n        - deeper\nend')).toEqual(['-', '-', '-'])
  })
})

describe('collectPreviewSpecs - blockquotes', () => {
  // A quote spec is a line decoration: from === to === the covered line's start.
  const quoteLines = (doc: string, s: PreviewSpec[]) => ({
    // 1-based line numbers carrying that depth's decoration.
    byDepth: (depth: number) => styled(s, `quote${depth}` as PreviewKind)
      .map((x) => doc.slice(0, x.from).split('\n').length),
    all: s.filter((x) => x.kind.startsWith('quote')),
  })

  it('draws a one-level quote line at the line start', () => {
    const doc = '> quoted\n\nafter'
    const q = quoteLines(doc, specs(doc))
    expect(q.byDepth(1)).toEqual([1])
    expect(q.byDepth(2)).toEqual([])
    expect(q.all).toHaveLength(1)
  })

  it('marks the inner line of a nested quote at depth 2, the outer at 1', () => {
    const doc = '> outer\n> > inner\n\nafter'
    const q = quoteLines(doc, specs(doc))
    expect(q.byDepth(1)).toEqual([1])
    expect(q.byDepth(2)).toEqual([2])
    expect(q.byDepth(3)).toEqual([])
    expect(q.all).toHaveLength(2)
  })

  it('tracks three levels, each line at the deepest quote reaching it', () => {
    const doc = '> one\n> > two\n> > > three\n\nafter'
    const q = quoteLines(doc, specs(doc))
    expect(q.byDepth(1)).toEqual([1])
    expect(q.byDepth(2)).toEqual([2])
    expect(q.byDepth(3)).toEqual([3])
    expect(q.all).toHaveLength(3)
  })

  it('hides QuoteMark markers when the cursor is elsewhere', () => {
    const doc = '> hidden\n\nafter'
    expect(hides(specs(doc)).map((h) => slice(doc, h))).toEqual(['>'])
  })

  it('shows the cursor line markers but keeps every quote line bordered', () => {
    const doc = '> one\n> two\n\nafter'
    const s = specs(doc, 2) // cursor on line 1
    expect(hides(s).map((h) => slice(doc, h))).toEqual(['>'])
    expect(quoteLines(doc, s).byDepth(1)).toEqual([1, 2])
  })

  it('keeps a list bullet inside a quote', () => {
    const doc = '> - item\n\nafter'
    const s = specs(doc)
    expect(styled(s, 'bullet').map((x) => slice(doc, x))).toEqual(['-'])
    expect(quoteLines(doc, s).byDepth(1)).toEqual([1])
  })

  it('keeps an inline code span inside a quote', () => {
    const doc = '> run `echo` here\n\nafter'
    const s = specs(doc)
    expect(styled(s, 'code').map((x) => slice(doc, x))).toEqual(['echo'])
  })

  it('keeps a fenced block quoting itself: quote lines plus code lines', () => {
    const doc = '> ```py\n> x = 1\n> ```\n\nafter'
    const s = specs(doc)
    expect(styled(s, 'codeBlock')).toHaveLength(3)
    expect(quoteLines(doc, s).byDepth(1)).toEqual([1, 2, 3])
  })

  it('caps the depth at six for a seven-deep quote', () => {
    const doc = '> 1\n> > 2\n> > > 3\n> > > > 4\n> > > > > 5\n> > > > > > 6\n> > > > > > > 7\n'
    const q = quoteLines(doc, specs(doc))
    for (let depth = 1; depth <= 5; depth++) {
      expect(q.byDepth(depth)).toEqual([depth])
    }
    // Line 7 is inside seven nested quotes but keeps level 6's look.
    expect(q.byDepth(6)).toEqual([6, 7])
    expect(q.all).toHaveLength(7)
  })

  it('renders reading mode the same and never alters the document', () => {
    const doc = '> outer\n> > inner\n> > > deep\n\nafter'
    const state = stateFor(doc)
    collectPreviewSpecs(state, new Set())
    expect(state.doc.toString()).toBe(doc)
    // Reading mode conceals every marker and keeps the quote lines: the cursor
    // gates nothing there.
    const reading = collectPreviewSpecs(state, new Set())
    expect(hides(reading).map((h) => slice(doc, h))).toEqual(['>', '>', '>', '>', '>', '>'])
    expect(quoteLines(doc, reading).byDepth(1)).toEqual([1])
    expect(quoteLines(doc, reading).byDepth(2)).toEqual([2])
    expect(quoteLines(doc, reading).byDepth(3)).toEqual([3])
  })
})

describe('collectPreviewSpecs - vault tags', () => {
  const tags = (doc: string) => styled(specs(doc), 'tag').map((x) => slice(doc, x))

  it('finds tags, nested ones included', () => {
    expect(tags('see #reference and #markdown/showcase here\nnext')).toEqual(['#reference', '#markdown/showcase'])
  })

  it('never mistakes a heading for a tag', () => {
    expect(tags('## 2. Paragraphs\n\n# Title\nbody')).toEqual([])
  })

  it('never mistakes a url fragment for a tag', () => {
    expect(tags('go to https://example.com#section now\nnext')).toEqual([])
  })

  it('rejects an all-digit tag, which is an issue reference', () => {
    expect(tags('fixes #123 and #45\nnext')).toEqual([])
  })

  it('ignores a colour literal inside a code block', () => {
    expect(tags('before\n\n```css\na { color: #fff; }\n```\n\nafter')).toEqual([])
  })
})
