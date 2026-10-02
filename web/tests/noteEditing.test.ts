import { afterEach, describe, it, expect } from 'vitest'
import { EditorState, type TransactionSpec } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { undo } from '@codemirror/commands'
import { noteMarkdown } from '../utils/noteLanguage'
import {
  insertLink,
  insertLinkSpec,
  noteEditingExtensions,
  shouldCloseQuote,
  toggleBold,
  toggleBoldSpec,
  toggleItalic,
  toggleItalicSpec,
  wrapSelectionSpec,
} from '../utils/noteEditing'

// Selecting `bold` in `a bold word`.
const SELECTED_FROM = 2
const SELECTED_TO = 6

const stateWith = (doc: string, anchor: number, head: number, readOnly = false) =>
  EditorState.create({
    doc,
    selection: { anchor, head },
    extensions: readOnly ? [EditorState.readOnly.of(true)] : [],
  })

const applied = (state: EditorState, text: string) => {
  const spec = wrapSelectionSpec(state, text)
  if (!spec) return null
  return state.update(spec).state
}

describe('wrapSelectionSpec', () => {
  it('wraps the selection and keeps it selected, so the pair can be nested', () => {
    let state = stateWith('a bold word', SELECTED_FROM, SELECTED_TO)

    state = applied(state, '`')!
    expect(state.doc.toString()).toBe('a `bold` word')
    // The selection still covers `bold`, not the backticks - that is what makes a
    // second keystroke produce a nested pair instead of replacing the text.
    expect(state.sliceDoc(state.selection.main.from, state.selection.main.to)).toBe('bold')

    state = applied(state, '`')!
    expect(state.doc.toString()).toBe('a ``bold`` word')
  })

  it('leaves an empty selection alone, so an apostrophe stays punctuation', () => {
    const state = stateWith('dont', 4, 4)
    expect(wrapSelectionSpec(state, "'")).toBeNull()
  })

  it('preserves a backwards selection', () => {
    const state = stateWith('a bold word', SELECTED_TO, SELECTED_FROM)
    const next = applied(state, '"')!

    expect(next.doc.toString()).toBe('a "bold" word')
    expect(next.sliceDoc(next.selection.main.from, next.selection.main.to)).toBe('bold')
  })

  it('refuses to write into a read-only document', () => {
    const state = stateWith('a bold word', SELECTED_FROM, SELECTED_TO, true)
    expect(wrapSelectionSpec(state, '`')).toBeNull()
  })
})

describe('shouldCloseQuote', () => {
  const at = (doc: string, pos: number) => shouldCloseQuote(EditorState.create({ doc }), pos)

  it('closes when opening a quotation - start of line or after a space', () => {
    expect(at('', 0)).toBe(true)
    expect(at('he said ', 8)).toBe(true)
    expect(at('line\n', 5)).toBe(true)
  })

  it('does not close after a word character, so an apostrophe stays one character', () => {
    expect(at('don', 3)).toBe(false)
    expect(at('anos 90', 7)).toBe(false)
  })
})

// The markdown toggles need the state they run against in the real editor: the
// parse-tree lookup is the whole mechanism, and without the language the tree
// would say nothing.
const edited = (
  command: (state: EditorState) => TransactionSpec | null,
  doc: string,
  anchor: number,
  head: number,
  readOnly = false,
) => {
  const extensions = [noteMarkdown(), noteEditingExtensions()]
  const state = EditorState.create({
    doc,
    selection: { anchor, head },
    extensions: readOnly ? [...extensions, EditorState.readOnly.of(true)] : extensions,
  })
  const spec = command(state)
  if (!spec) return null
  return state.update(spec).state
}

const selectedText = (state: EditorState) => state.sliceDoc(state.selection.main.from, state.selection.main.to)

const views: EditorView[] = []
afterEach(() => {
  for (const view of views) view.destroy()
  views.length = 0
})

const createView = (doc: string, anchor: number, head: number, readOnly = false) => {
  const view = new EditorView({
    state: EditorState.create({
      doc,
      selection: { anchor, head },
      extensions: [
        noteMarkdown(),
        noteEditingExtensions(),
        ...(readOnly ? [EditorState.readOnly.of(true)] : []),
      ],
    }),
    parent: document.body,
  })
  views.push(view)
  return view
}

describe.each([
  ['bold', toggleBoldSpec, '**'],
  ['italic', toggleItalicSpec, '*'],
] as const)('%s toggle', (_label, command, markup) => {
  const span = (text: string) => markup + text + markup

  it('wraps a plain selection and keeps it selected', () => {
    const next = edited(command, 'a bold word', SELECTED_FROM, SELECTED_TO)!
    expect(next.doc.toString()).toBe(`a ${span('bold')} word`)
    expect(selectedText(next)).toBe('bold')
  })

  it('unwraps when the selection covers the whole construct, marks included', () => {
    const next = edited(command, `a ${span('bold')} word`, 2, 2 + markup.length * 2 + 4)!
    expect(next.doc.toString()).toBe('a bold word')
    expect(selectedText(next)).toBe('bold')
  })

  it('unwraps when only the inner text is selected, and survives toggling back', () => {
    const wrapped = edited(command, 'a bold word', SELECTED_FROM, SELECTED_TO)!
    const innerFrom = 2 + markup.length
    const next = edited(command, wrapped.doc.toString(), innerFrom, innerFrom + 4)!
    expect(next.doc.toString()).toBe('a bold word')
    expect(selectedText(next)).toBe('bold')
  })

  it('keeps a backwards selection backwards when wrapping', () => {
    const next = edited(command, 'a bold word', SELECTED_TO, SELECTED_FROM)!
    expect(next.doc.toString()).toBe(`a ${span('bold')} word`)
    expect(selectedText(next)).toBe('bold')
  })

  it('wraps the word under an empty cursor', () => {
    const next = edited(command, 'a bold word', 4, 4)!
    expect(next.doc.toString()).toBe(`a ${span('bold')} word`)
    expect(selectedText(next)).toBe('bold')
  })

  it('unwraps the span an empty cursor sits inside', () => {
    const next = edited(command, span('bold'), markup.length + 1, markup.length + 1)!
    expect(next.doc.toString()).toBe('bold')
    expect(selectedText(next)).toBe('bold')
  })

  it('treats a cursor resting on the construct boundary as inside it', () => {
    // Wrapping at a seam stacks a second pair against the first (`****bold**`),
    // so the boundary edges count as inside and unwrap instead.
    const next = edited(command, span('bold'), 0, 0)!
    expect(next.doc.toString()).toBe('bold')
  })

  it('wraps a selection that two separate spans merely surround', () => {
    // `**` (or `*`) sits on both sides of `sel` but belongs to two different
    // spans: the case that forces the lookup through the parse tree rather
    // than through the characters around the selection.
    const next = edited(
      command,
      `${span('a')} sel ${span('b')}`,
      markup.length * 2 + 2,
      markup.length * 2 + 5,
    )!
    expect(next.doc.toString()).toBe(`${span('a')} ${span('sel')} ${span('b')}`)
    expect(selectedText(next)).toBe('sel')
  })

  it('leaves an empty pair in whitespace for the writer to type into', () => {
    const next = edited(command, 'a word', 1, 1)!
    expect(next.doc.toString()).toBe(`a${markup + markup} word`)
    expect(next.selection.main.from).toBe(1 + markup.length)
  })

  it('leaves an empty pair on an empty document', () => {
    const next = edited(command, '', 0, 0)!
    expect(next.doc.toString()).toBe(markup + markup)
    expect(next.selection.main.from).toBe(markup.length)
  })

  it('refuses to write into a read-only document', () => {
    expect(edited(command, 'a bold word', SELECTED_FROM, SELECTED_TO, true)).toBeNull()
  })
})

describe('italic and bold interact through the tree, not the characters', () => {
  it('wrapping italic around bold text makes bold-italic', () => {
    const next = edited(toggleItalicSpec, '**bold**', 2, 6)!
    expect(next.doc.toString()).toBe('***bold***')
    expect(selectedText(next)).toBe('bold')
  })

  it('toggling italic inside bold-italic leaves the bold', () => {
    const next = edited(toggleItalicSpec, '***x***', 3, 4)!
    expect(next.doc.toString()).toBe('**x**')
    expect(selectedText(next)).toBe('**x**')
  })

  it('toggling bold inside bold-italic leaves the italic', () => {
    const next = edited(toggleBoldSpec, '***x***', 3, 4)!
    expect(next.doc.toString()).toBe('*x*')
    expect(selectedText(next)).toBe('x')
  })
})

describe('insertLink', () => {
  it('wraps a selection with the cursor between the parentheses', () => {
    const next = edited(insertLinkSpec, 'see this', 4, 8)!
    expect(next.doc.toString()).toBe('see [this]()')
    expect(next.selection.main.from).toBe(next.selection.main.to)
    expect(next.sliceDoc(next.selection.main.from - 1, next.selection.main.from + 1)).toBe('()')
  })

  it('leaves an empty pair with the cursor between the brackets', () => {
    const next = edited(insertLinkSpec, 'a', 1, 1)!
    expect(next.doc.toString()).toBe('a[]()')
    expect(next.selection.main.from).toBe(2)
  })

  it('selects the URL of the link the cursor sits inside', () => {
    const next = edited(insertLinkSpec, 'see [docs](https://x)', 7, 7)!
    expect(next.doc.toString()).toBe('see [docs](https://x)')
    expect(selectedText(next)).toBe('https://x')
  })

  it('selects the URL when a text range inside the link is selected', () => {
    const next = edited(insertLinkSpec, 'see [docs](https://x)', 5, 9)!
    expect(next.doc.toString()).toBe('see [docs](https://x)')
    expect(selectedText(next)).toBe('https://x')
  })

  it('leaves a reference-style link alone - it parses without a URL', () => {
    expect(edited(insertLinkSpec, 'see [docs][ref]', 5, 5)).toBeNull()
  })

  it('refuses to write into a read-only document', () => {
    expect(edited(insertLinkSpec, 'see this', 4, 8, true)).toBeNull()
  })
})

describe('one undo per command', () => {
  it('restores the text before a bold toggle', () => {
    const view = createView('a bold word', SELECTED_FROM, SELECTED_TO)
    expect(toggleBold(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('a **bold** word')
    expect(undo(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('a bold word')
  })

  it('restores the text before an italic toggle', () => {
    const view = createView('a bold word', SELECTED_FROM, SELECTED_TO)
    expect(toggleItalic(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('a *bold* word')
    expect(undo(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('a bold word')
  })

  it('restores the text before a link insert', () => {
    const view = createView('see this', 4, 8)
    expect(insertLink(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('see [this]()')
    expect(undo(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('see this')
  })

  it('restores the text before a word wrap', () => {
    const view = createView('a bold word', 4, 4)
    expect(toggleBold(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('a **bold** word')
    expect(undo(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('a bold word')
  })

  it('does not join the word typed just before the toggle', () => {
    // First undo after a toggle removes the format only, leaving the word the
    // writer had just typed - which requires history NOT to merge the toggle
    // transaction with the adjacent typing.
    const view = createView('hello', 5, 5)
    view.dispatch({ changes: { from: 5, insert: 'x' }, selection: { anchor: 5, head: 6 }, userEvent: 'input.type' })
    expect(toggleBold(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('hello**x**')
    expect(undo(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('hellox')
  })
})

describe('toggle commands under read-only', () => {
  it('change nothing and still claim the chord so the browser never answers', () => {
    const bold = createView('a **bold** word', 2, 10, true)
    expect(toggleBold(bold)).toBe(true)
    expect(bold.state.doc.toString()).toBe('a **bold** word')
    expect(bold.state.selection.main.from).toBe(2)

    const italic = createView('*bold*', 0, 6, true)
    expect(toggleItalic(italic)).toBe(true)
    expect(italic.state.doc.toString()).toBe('*bold*')

    const link = createView('see this', 4, 8, true)
    expect(insertLink(link)).toBe(true)
    expect(link.state.doc.toString()).toBe('see this')
  })
})

describe('the keymap answers the real chords', () => {
  it.each([
    ['b', 'Ctrl+B'],
    ['i', 'Ctrl+I'],
    ['k', 'Ctrl+K'],
  ] as const)('%s reaches the editor through the keymap', (key) => {
    const view = createView('a bold word', 2, 6)
    view.contentDOM.dispatchEvent(
      new KeyboardEvent('keydown', { key, ctrlKey: true, bubbles: true, cancelable: true }),
    )
    expect(view.state.doc.toString()).not.toBe('a bold word')
  })
})