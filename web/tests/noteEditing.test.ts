import { describe, it, expect } from 'vitest'
import { EditorState } from '@codemirror/state'
import { shouldCloseQuote, wrapSelectionSpec } from '../utils/noteEditing'

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
    const state = stateWith("dont", 4, 4)
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

