// The editing behaviour of the notes editor: undo, indentation, bracket pairs
// and selection wrapping.
//
// Until this existed the editor bound exactly one key, Mod-s. Everything else a
// text editor is assumed to do came from the browser's contenteditable, which is
// why undo appeared to work once and then only walked the cursor backwards: the
// native history holds the last DOM edit, and CodeMirror rewrites the DOM out
// from under it. `history()` is what makes undo an editor feature instead of a
// browser accident.

import { EditorSelection, EditorState, type Extension, type Transaction, type TransactionSpec } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { indentUnit } from '@codemirror/language'

// Characters whose open and close form are the same. They wrap a selection, but
// they deliberately do NOT auto-close on an empty selection: `closeBrackets`
// would happily turn `don't` into `don''t`, and a notes editor is mostly prose,
// where an apostrophe is punctuation rather than an unclosed pair. Real brackets
// keep the full behaviour - see the `closeBrackets` language data in
// ./noteLanguage.
//
// NOTE: on a layout with dead keys these three never arrive here at all. The key
// press opens an input-method composition instead of emitting a character, and
// intercepting the keydown to wrap anyway was measured NOT to work: the IME
// ignores preventDefault, runs the composition regardless, and the note ends up
// with both the wrapped text and a stray accent character.
const WRAPPING_CHARS = new Set(['"', "'", '`'])

/**
 * The transaction that wraps every non-empty selection range in `text`, or null
 * when there is nothing to wrap. The selection is preserved over the original
 * text rather than collapsed, so pressing the same key again nests the pair -
 * which is how `[[` reaches `[[target]]` in two keystrokes.
 *
 * Example: with `bold` selected, `wrapSelectionSpec(state, '`')` yields a
 * transaction producing `` `bold` `` with `bold` still selected.
 */
export function wrapSelectionSpec(state: EditorState, text: string): TransactionSpec | null {
  if (state.readOnly) return null
  if (state.selection.ranges.every((range) => range.empty)) return null

  return {
    ...state.changeByRange((range) =>
      range.empty
        ? { range }
        : {
            changes: [
              { from: range.from, insert: text },
              { from: range.to, insert: text },
            ],
            range: EditorSelection.range(range.anchor + text.length, range.head + text.length),
          },
    ),
    userEvent: 'input.type',
    scrollIntoView: true,
  }
}

/**
 * Whether a quote or backtick typed at `pos` should bring its closing half. The
 * rule is the mirror image of the one `closeBrackets` applies to real brackets:
 * a bracket closes based on what follows the cursor, a quote on what PRECEDES
 * it. Opening a quotation always happens after a space or at the start of a
 * line, while the apostrophe in `don't` never does - so this is what separates
 * the two without asking the writer to think about it.
 */
export function shouldCloseQuote(state: EditorState, pos: number): boolean {
  if (pos === 0) return true
  return /\s/.test(state.sliceDoc(pos - 1, pos))
}

function quotePairSpec(state: EditorState, text: string): TransactionSpec | null {
  if (state.readOnly) return null
  const range = state.selection.main
  if (!range.empty || !shouldCloseQuote(state, range.head)) return null

  return {
    changes: { from: range.head, insert: text + text },
    selection: { anchor: range.head + text.length },
    userEvent: 'input.type',
    scrollIntoView: true,
  }
}

function wrapSelectionInput(): Extension {
  return EditorView.inputHandler.of((view, _from, _to, text) => {
    if (!WRAPPING_CHARS.has(text)) return false
    const spec = wrapSelectionSpec(view.state, text) ?? quotePairSpec(view.state, text)
    if (!spec) return false
    view.dispatch(spec)
    return true
  })
}

// True when a transaction types whitespace. Used to end the undo group there, so
// undo walks back word by word. CodeMirror's own rule is purely temporal - it
// merges any two adjacent edits typed less than `newGroupDelay` apart - and
// nobody pauses half a second mid-sentence, so a whole paragraph typed at speed
// collapses into a single undo step.
function insertsWhitespace(tr: Transaction): boolean {
  let found = false
  tr.changes.iterChanges((_fromA, _toA, _fromB, _toB, inserted) => {
    if (!found && /\s/.test(inserted.toString())) found = true
  })
  return found
}

/**
 * Editing extensions for the notes editor, in the order their key bindings get
 * to claim a keystroke. `closeBracketsKeymap` leads so Backspace can delete a
 * whole pair; lang-markdown's own keymap outranks all of these anyway (it is
 * registered at high precedence), which is what keeps Enter continuing a list.
 *
 * Tab indents, which does mean Tab no longer moves focus out of the editor -
 * accepted deliberately: nesting a list item is the far more common intent in a
 * note, and the editor is a document surface rather than a form field.
 */
export function noteEditingExtensions(): Extension {
  return [
    history({ joinToEvent: (tr, adjacent) => adjacent && !insertsWhitespace(tr) }),
    // One Tab is four spaces. CodeMirror's default is two; four is what the vault's
    // markdown already uses, and it clears CommonMark's nesting threshold for every
    // list marker width rather than only for `- `.
    indentUnit.of('    '),
    closeBrackets(),
    wrapSelectionInput(),
    keymap.of([...closeBracketsKeymap, ...historyKeymap, ...defaultKeymap, indentWithTab]),
  ]
}
