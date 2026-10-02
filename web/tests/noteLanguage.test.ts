// Tests for the notes editor's fenced-code language configuration: the info
// string resolver, that grammars stay unloaded until a fence asks for them, and
// the three code-token styles the highlight style owns - no more.
//
// The support-unset check below must run BEFORE the tests that call
// `.load()`: those deliberately fill the lazy slot, and nothing should be able
// to pass with it prefilled.

import { describe, it, expect } from 'vitest'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { syntaxTree } from '@codemirror/language'
import { highlightTree, tags, type Tag } from '@lezer/highlight'
import { noteMarkdown, noteCodeLanguage } from '../utils/noteLanguage'
import { noteHighlightStyle, noteHighlighting } from '../utils/noteHighlight'

const resolveName = (info: string) => noteCodeLanguage(info)?.name ?? null

// style-mod's getRules() is one CSS text; parse a class's own declaration body
// out of it so style values can be asserted directly.
function cssBodyFor(cls: string): string | null {
  const allRules = noteHighlightStyle.module?.getRules() ?? ''
  const match = allRules.match(new RegExp(`\\.${cls}\\s*\\{([^}]*)\\}`))
  return match ? match[1] : null
}

const cssRuleFor = (tag: Tag): string | null => {
  const cls = noteHighlightStyle.style([tag])
  return cls ? cssBodyFor(cls) : null
}

describe('noteCodeLanguage', () => {
  it('resolves each accepted info string to its grammar', () => {
    expect(resolveName('go')).toBe('Go')
    expect(resolveName('ts')).toBe('TypeScript')
    expect(resolveName('typescript')).toBe('TypeScript')
    expect(resolveName('js')).toBe('JavaScript')
    expect(resolveName('javascript')).toBe('JavaScript')
    expect(resolveName('sh')).toBe('Shell')
    expect(resolveName('bash')).toBe('Shell')
    expect(resolveName('python')).toBe('Python')
    expect(resolveName('json')).toBe('JSON')
    expect(resolveName('yaml')).toBe('YAML')
    expect(resolveName('sql')).toBe('SQL')
  })

  it('returns null for unknown and empty info strings', () => {
    expect(resolveName('')).toBe(null)
    expect(resolveName('madefup')).toBe(null)
  })

  // Runs before the `.load()` tests on purpose: it asserts the lazy slot is
  // still empty, which is what keeps notes that never code from paying for a
  // grammar.
  it('returns grammars with support unset before first load', () => {
    for (const info of ['go', 'ts', 'sh']) {
      const language = noteCodeLanguage(info)
      expect(language).not.toBe(null)
      expect(language!.support).toBeFalsy()
    }
  })
})

describe('code token styles', () => {
  it('maps comment, keyword and string, and nothing else among code tags', () => {
    expect(cssRuleFor(tags.comment)).toBeTruthy()
    expect(cssRuleFor(tags.keyword)).toBeTruthy()
    expect(cssRuleFor(tags.string)).toBeTruthy()
    // Common code tags that must stay on the base code text.
    for (const tag of [tags.number, tags.operator, tags.variableName, tags.typeName, tags.punctuation, tags.bool]) {
      expect(cssRuleFor(tag), 'tag must be unstyled').toBe(null)
    }
  })

  it('keeps keywords at the code colour and strings on the accent text tone', () => {
    const keyword = cssRuleFor(tags.keyword)
    expect(keyword).toContain('font-weight: 600')
    expect(keyword).not.toContain('color:')
    expect(cssRuleFor(tags.string)).toContain('color: var(--color-primary-text)')
    expect(cssRuleFor(tags.comment)).toContain('var(--color-text-faint)')
  })

  it('pins the task marker against the keyword rule', () => {
    // `atom` descends from `keyword`, so a task's `[x]` picks up the keyword
    // class on top of its own. The explicit 400 on the atom rule is what keeps
    // the marker at its shipped weight once the cascade settles.
    const atomCls = noteHighlightStyle.style([tags.atom])
    expect(atomCls).toBeTruthy()
    const rules = noteHighlightStyle.module?.getRules() ?? ''
    const bodies = (atomCls ?? '')
      .split(' ')
      .map((cls) => {
        const match = rules.match(new RegExp(`\\.${cls}\\s*\\{([^}]*)\\}`))
        return match ? match[1] : null
      })
    const pinned = bodies.find((body) => body?.includes('font-weight: 400'))
    expect(pinned).toBeTruthy()
    expect(pinned).toContain('var(--color-text-muted)')
  })
})

describe('fences in the editor', () => {
  // happy-dom mounts a real EditorView; the generated class names come from the
  // style itself (stable within the process) rather than being guessed.
  async function viewFor(doc: string) {
    const view = new EditorView({
      state: EditorState.create({ doc, extensions: [noteMarkdown(), noteHighlighting()] }),
      parent: document.createElement('div'),
    })
    await new Promise((resolve) => setTimeout(resolve, 50))
    return view
  }

  function spansWithClass(view: EditorView) {
    const count = (cls: string | null) => (cls ? view.dom.querySelectorAll('.' + cls).length : 0)
    return {
      monospace: count(noteHighlightStyle.style([tags.monospace])),
      comment: count(noteHighlightStyle.style([tags.comment])),
      keyword: count(noteHighlightStyle.style([tags.keyword])),
      string: count(noteHighlightStyle.style([tags.string])),
    }
  }

  it('a fence with unknown or empty language renders as plain code, exactly as today', async () => {
    const view = await viewFor('```madefup\nplain code\n```\n\n```\nalso plain\n```\n')
    const { monospace, comment, keyword, string: stringTag } = spansWithClass(view)
    // Unresolved bodies stay on the CodeText token styling: mono + code colour.
    expect(monospace).toBeGreaterThanOrEqual(2)
    expect(comment).toBe(0)
    expect(keyword).toBe(0)
    expect(stringTag).toBe(0)
    view.destroy()
  })

  it('a loaded go fence keeps the mono body and styles comment, keyword and string', async () => {
    await noteCodeLanguage('go')!.load()
    const view = await viewFor('```go\n// quiet\nconst x = "hello"\n```\n')
    const { comment, keyword, string: stringTag } = spansWithClass(view)
    // The mono on parsed bodies arrives from the fence-body line class, because
    // the nested grammar's token walk does not inherit the doc-level class.
    expect(view.dom.querySelectorAll('.cm-note-code-body').length).toBeGreaterThanOrEqual(2)
    expect(comment).toBeGreaterThanOrEqual(1)
    expect(keyword).toBeGreaterThanOrEqual(1)
    expect(stringTag).toBeGreaterThanOrEqual(1)
    view.destroy()
  })

  it('prose never carries the code body class', async () => {
    const view = await viewFor('plain **bold** text\n\n```go\nx := 1\n```\n')
    const lines = Array.from(view.dom.querySelectorAll('.cm-line'))
    const bodies = lines.filter((line) => line.className.includes('cm-note-code-body'))
    expect(bodies.length).toBe(1) // only the go fence's body line
    expect(bodies[0].textContent).toBe('x := 1')
    view.destroy()
  })

  it('highlightTree emits the token classes inside a resolved fence', async () => {
    await noteCodeLanguage('go')!.load()
    const doc = '```go\n// quiet\nconst x = "hello"\n100\n```'
    const state = EditorState.create({ doc, extensions: [noteMarkdown()] })
    const classFor = (tag: Tag) => noteHighlightStyle.style([tag]) ?? '?'
    const spans: { text: string; cls: string }[] = []
    highlightTree(syntaxTree(state), [noteHighlightStyle], (from, to, classes) => {
      spans.push({ text: doc.slice(from, to), cls: classes })
    })
    const byIncludes = (needle: string) => spans.find((span) => span.text.includes(needle))
    expect(byIncludes('quiet')?.cls).toContain(classFor(tags.comment))
    expect(byIncludes('const')?.cls).toContain(classFor(tags.keyword))
    expect(byIncludes('hello')?.cls).toContain(classFor(tags.string))
  })
})