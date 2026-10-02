// The editing behaviour of the notes editor: undo, indentation, bracket pairs,
// selection wrapping and the markdown format toggles (bold / italic / link).
//
// Until this existed the editor bound exactly one key, Mod-s. Everything else a
// text editor is assumed to do came from the browser's contenteditable, which is
// why undo appeared to work once and then only walked the cursor backwards: the
// native history holds the last DOM edit, and CodeMirror rewrites the DOM out
// from under it. `history()` is what makes undo an editor feature instead of a
// browser accident.

import { EditorSelection, EditorState, type Extension, type Transaction, type TransactionSpec } from '@codemirror/state'
import { EditorView, keymap, type SelectionRange } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { indentUnit, syntaxTree } from '@codemirror/language'
import type { SyntaxNode } from '@lezer/common'

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

// Innermost node of one of the `wanted` types that covers [from, to), or null.
// Children are walked left-to-right and taken first, so a cursor resting on the
// seam of two touching children counts as inside the left one. Looking this up
// in the PARSE TREE rather than in the surrounding text is the whole point:
// `*` is both an italic marker and a list marker, and `**a** sel **b**` has
// `**` on both sides of `sel` while `sel` sits in no span at all - only the
// tree knows which span a pair of markers actually belongs to.
function enclosingSpan(node: SyntaxNode, from: number, to: number, wanted: readonly string[]): SyntaxNode | null {
  for (let child = node.firstChild; child; child = child.nextSibling) {
    if (child.from <= from && to <= child.to) {
      return enclosingSpan(child, from, to, wanted) ?? (wanted.includes(child.name) ? child : null)
    }
  }
  return wanted.includes(node.name) && node.from <= from && to <= node.to ? node : null
}

// The word the empty cursor sits IN. `state.wordAt` also returns a word that
// merely touches the cursor - the one just before it when the cursor rests on
// the space after the word - which would turn every cursor in whitespace into
// a word-wrap. Requiring the position to fall inside the range is what keeps
// cursor-in-whitespace in the empty-pair path below.
function wordUnderCursor(state: EditorState, pos: number): SelectionRange | null {
  const word = state.wordAt(pos)
  return word && word.from <= pos && pos < word.to ? word : null
}


// One `markup` pair around `range`, with the text left selected (direction
// preserved) so the same chord pressed again nests the pair.
function wrapPairSpec(range: SelectionRange, markup: string): TransactionSpec {
  return {
    changes: [
      { from: range.from, insert: markup },
      { from: range.to, insert: markup },
    ],
    selection: EditorSelection.range(range.anchor + markup.length, range.head + markup.length),
    userEvent: 'input.format',
    scrollIntoView: true,
  }
}

/**
 * The transaction behind Mod-b: toggles `**bold**` on the main selection. See
 * `emphasisToggleSpec` for the wrap / unwrap / word / whitespace semantics.
 * Exported for tests; null under `readOnly`.
 */
export function toggleBoldSpec(state: EditorState): TransactionSpec | null {
  return emphasisToggleSpec(state, 'StrongEmphasis', '**')
}

/**
 * The transaction behind Mod-i: toggles `*italic*`, same semantics with the
 * `Emphasis` node - which is what makes toggling italic inside bold-italic
 * (`***x***`) leave the **bold** in place. Exported for tests.
 */
export function toggleItalicSpec(state: EditorState): TransactionSpec | null {
  return emphasisToggleSpec(state, 'Emphasis', '*')
}

// The shared body of both emphasis toggles. Matching on node NAME is what keeps
// `***x***` working: Mod-i looks for an `Emphasis` node, never the nested
// `StrongEmphasis`, so the italics unwrap while the bold stays.
function emphasisToggleSpec(state: EditorState, nodeType: 'StrongEmphasis' | 'Emphasis', markup: string): TransactionSpec | null {
  if (state.readOnly) return null
  const range = state.selection.main
  const node = enclosingSpan(syntaxTree(state).topNode, range.from, range.to, [nodeType])
  if (node) {
    const open = node.getChild('EmphasisMark')
    const close = node.lastChild
    // The markdown grammar always gives an emphasis node its open/close mark
    // pair; the guard only keeps a hypothetical malformed branch from mangling
    // the document.
    if (open && close?.name === 'EmphasisMark' && close.from >= open.to) {
      const innerFrom = open.from
      const innerTo = close.from - markup.length
      const backward = range.head < range.anchor
      return {
        changes: [
          { from: open.from, to: open.to },
          { from: close.from, to: close.to },
        ],
        selection: EditorSelection.range(backward ? innerTo : innerFrom, backward ? innerFrom : innerTo),
        userEvent: 'input.format',
        scrollIntoView: true,
      }
    }
    return null
  }
  if (!range.empty) return wrapPairSpec(range, markup)

  const word = wordUnderCursor(state, range.head)
  if (word) return wrapPairSpec(EditorSelection.range(word.from, word.to), markup)

  return {
    changes: { from: range.head, insert: markup + markup },
    selection: { anchor: range.head + markup.length },
    userEvent: 'input.format',
    scrollIntoView: true,
  }
}

/**
 * The transaction behind Mod-k: wraps the main selection as `[sel]()` with the
 * cursor between the parentheses, leaves an empty `[]()` when the cursor is
 * bare, and selects the URL of the link the selection sits inside so it can be
 * inspected or retyped. Reference-style links (`[text][ref]`) parse without a
 * URL and are left alone. Exported for tests; null under `readOnly`.
 */
export function insertLinkSpec(state: EditorState): TransactionSpec | null {
  if (state.readOnly) return null
  const range = state.selection.main
  const link = enclosingSpan(syntaxTree(state).topNode, range.from, range.to, ['Link'])
  if (link) {
    const url = link.getChild('URL')
    // Reference-style links have no URL child; claiming the chord without an
    // action beats mangling a `[text][ref]` construct.
    if (!url) return null
    return {
      selection: EditorSelection.range(url.from, url.to),
      userEvent: 'select',
      scrollIntoView: true,
    }
  }
  if (!range.empty) {
    return {
      changes: [
        { from: range.from, insert: '[' },
        { from: range.to, insert: ']()' },
      ],
      // `]()` lands shifted by the lead `[`: the cursor goes just before the
      // closing paren.
      selection: { anchor: range.to + 3 },
      userEvent: 'input.format',
      scrollIntoView: true,
    }
  }
  return {
    changes: { from: range.head, insert: '[]()' },
    selection: { anchor: range.head + 1 },
    userEvent: 'input.format',
    scrollIntoView: true,
  }
}

/**
 * Ctrl+B (Cmd on macOS) toggles `**bold**` on the main selection. Always one
 * explicit transaction, so undo removes it in a single step. Returns true even
 * when nothing changes (read-only), so the browser's own Ctrl+B never fires.
 */
export function toggleBold(view: EditorView): boolean {
  const spec = toggleBoldSpec(view.state)
  if (spec) view.dispatch(spec)
  return true
}

/**
 * Ctrl+I toggles `*italic*`, with the same semantics as toggleBold. Inside
 * bold-italic (`***x***`) it strips the wrapping `Emphasis` and leaves the bold.
 */
export function toggleItalic(view: EditorView): boolean {
  const spec = toggleItalicSpec(view.state)
  if (spec) view.dispatch(spec)
  return true
}

/** Ctrl+K inserts a markdown link on the main selection. */
export function insertLink(view: EditorView): boolean {
  const spec = insertLinkSpec(view.state)
  if (spec) view.dispatch(spec)
  return true
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
    keymap.of([
      // The toggles lead and each claims its chord even when it changes nothing;
      // a false return would let the browser's own Ctrl+B / Ctrl+K fire. They use
      // the `input.format` user event, which history never joins across: a toggle
      // stays one undo step even right after typing.
      { key: 'Mod-b', run: toggleBold },
      { key: 'Mod-i', run: toggleItalic },
      { key: 'Mod-k', run: insertLink },
      ...closeBracketsKeymap,
      ...historyKeymap,
      ...defaultKeymap,
      indentWithTab,
    ]),
  ]
}
