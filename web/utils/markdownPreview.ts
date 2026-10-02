// CodeMirror 6 live-preview for the notes editor: conceal markdown syntax
// markers on lines the cursor isn't on, and style the content inline so a note
// reads like rendered markdown (Obsidian-like) until you move into a line to
// edit it. The marker-collection logic is a pure function over the parsed
// syntax tree (unit-tested headless via collectPreviewSpecs); this module is the
// thin glue that turns specs into a CM6 ViewPlugin + theme.
//
// Wikilinks ([[...]]) are handled by ./wikilinkEditor, not here. The markdown
// parser sees a [[target]] as a reference-style Link with no URL child, so we
// only ever conceal links that carry an explicit URL - which leaves wikilinks
// untouched and avoids fighting the pill decorations for the same range.

import type { EditorState } from '@codemirror/state'
import {
  Decoration,
  EditorView,
  ViewPlugin,
  WidgetType,
  type DecorationSet,
  type ViewUpdate,
} from '@codemirror/view'
import { syntaxTree } from '@codemirror/language'
import { isolateHistory } from '@codemirror/commands'
import type { SyntaxNodeRef } from '@lezer/common'

// A decoration intent emitted by the pure pass. `hide` removes a syntax marker
// from view; every other kind styles a content range. Keeping these as plain
// data (not Decoration objects) is what lets the collector be tested headlessly.
export type PreviewKind =
  | 'hide'
  | 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6'
  | 'strong'
  | 'emphasis'
  | 'code'
  | 'link'
  // Line kinds: they decorate a whole line rather than a span of text, and are
  // emitted with from === to at the line's start offset.
  | 'rule'
  | 'codeBlock'
  | 'comment'
  | 'tag'
  // Replaces the list marker with a bullet glyph.
  | 'bullet'
  // Replaces a task item's whole "- [x]" run with a clickable checkbox.
  | 'task'
  // Styles a checked task item's text as done (muted, no strike).
  | 'taskDone'

export interface PreviewSpec {
  from: number
  to: number
  kind: PreviewKind
  /** Where a link goes, when it goes anywhere this editor is willing to open. */
  href?: string
  /** Task checkbox only: the box state, and where the toggled character sits. */
  checked?: boolean
  toggleAt?: number
}

// A link is only clickable when its target is one of the schemes a note is
// allowed to launch. Note bodies are written by the DA as well as by hand, so an
// unchecked `javascript:` here would be a scripting hole with an author that is
// not always human. Anything else - a relative path, a heading anchor - still
// gets styled, it just does not open.
export function linkHref(raw: string): string | undefined {
  const target = raw.trim()
  if (/^(https?:\/\/|mailto:)/i.test(target)) return target
  // GFM autolinks a bare address without a scheme.
  if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(target)) return `mailto:${target}`
  return undefined
}

// Line numbers (1-based) touched by any selection range. A formatting marker is
// only concealed when its line is absent from this set - the raw-on-cursor-line
// safety valve the brief asks for.
export function computeActiveLines(state: EditorState): Set<number> {
  const lines = new Set<number>()
  for (const range of state.selection.ranges) {
    const first = state.doc.lineAt(range.from).number
    const last = state.doc.lineAt(range.to).number
    for (let n = first; n <= last; n++) lines.add(n)
  }
  return lines
}

// Ordered markers (`1.`, `1)`) are left alone: their text is the number, which
// carries meaning a glyph cannot.
const BULLET_MARKERS = new Set(['-', '*', '+'])

const HEADING_LEVEL: Record<string, PreviewKind> = {
  ATXHeading1: 'h1', ATXHeading2: 'h2', ATXHeading3: 'h3',
  ATXHeading4: 'h4', ATXHeading5: 'h5', ATXHeading6: 'h6',
}

// A setext heading is two lines: the title, then `===` or `---` underneath. Only
// the title is styled - the node spans both lines, so styling it whole would set
// the underline in 1.6em as well. The underline is left in place and merely
// dimmed (it carries the same tag as every other syntax marker), which is what
// Obsidian shows too.
const SETEXT_LEVEL: Record<string, PreviewKind> = {
  SetextHeading1: 'h1', SetextHeading2: 'h2',
}

// Whether a node's line is one the cursor is on. Headings/inline marks are
// single-line in practice, so checking the start line is sufficient and cheap.
function lineActive(state: EditorState, pos: number, activeLines: Set<number>): boolean {
  return activeLines.has(state.doc.lineAt(pos).number)
}

// A stretch of the document worth decorating. The plugin passes the viewport;
// tests pass nothing and get the whole document.
export interface PreviewRange {
  from: number
  to: number
}

// Walk the markdown syntax tree and emit style + (conditionally) hide specs.
// Styling is always emitted so bold reads as bold even on the active line;
// only marker concealment is gated on the line being inactive.
//
// `ranges` bounds the walk. It matters more than it looks: this runs again on
// every cursor movement, and walking a whole note to decorate the screenful the
// reader can see makes the cost of moving the caret grow with the length of the
// document.
export function collectPreviewSpecs(
  state: EditorState,
  activeLines: Set<number>,
  ranges: readonly PreviewRange[] = [{ from: 0, to: state.doc.length }],
): PreviewSpec[] {
  const specs: PreviewSpec[] = []
  const doc = state.doc
  // A node straddling two ranges is entered once per range; the same decoration
  // twice would stack a mark on itself.
  const seen = new Set<string>()
  // The stretch being walked right now, so a block longer than the screen only
  // decorates the part of itself that is on it.
  let bounds: PreviewRange = ranges[0] ?? { from: 0, to: doc.length }

  const style = (from: number, to: number, kind: PreviewKind, href?: string) => {
    if (to <= from) return
    const key = `${from}:${to}:${kind}`
    if (seen.has(key)) return
    seen.add(key)
    specs.push({ from, to, kind, href })
  }
  // Conceal a marker only when its line isn't being edited.
  const hide = (from: number, to: number) => {
    if (lineActive(state, from, activeLines)) return
    style(from, to, 'hide')
  }
  // The task checkbox spec. Unlike the style kinds it carries widget data: the
  // box state and the position of the character a click flips inside the
  // brackets, so the widget can edit the document without re-parsing.
  const taskSpec = (from: number, to: number, checked: boolean, toggleAt: number) => {
    const key = `${from}:${to}:task`
    if (seen.has(key)) return
    seen.add(key)
    specs.push({ from, to, kind: 'task', checked, toggleAt })
  }
  // A line decoration is anchored at the line's start with zero width, so it
  // cannot go through style(), which drops empty spans.
  const line = (pos: number, kind: PreviewKind) => {
    const key = `${pos}:${kind}`
    if (seen.has(key)) return
    seen.add(key)
    specs.push({ from: pos, to: pos, kind })
  }
  // Every line of a block, clipped to the stretch currently being walked: a code
  // block can be longer than the screen, and only what is on screen needs marking.
  const linesOf = (from: number, to: number, kind: PreviewKind) => {
    let pos = Math.max(from, bounds.from)
    const end = Math.min(to, bounds.to)
    while (pos <= end) {
      const docLine = doc.lineAt(pos)
      line(docLine.from, kind)
      if (docLine.to >= doc.length) break
      pos = docLine.to + 1
    }
  }

  const enter = (node: SyntaxNodeRef) => {
    const headingKind = HEADING_LEVEL[node.name]
    if (headingKind) {
      style(node.from, node.to, headingKind)
      const mark = node.node.getChild('HeaderMark')
      if (mark) {
        // Swallow the single space after the "#" run so the title sits flush.
        const after = doc.sliceString(mark.to, mark.to + 1) === ' ' ? mark.to + 1 : mark.to
        hide(node.from, after)
      }
      return
    }

    const setextKind = SETEXT_LEVEL[node.name]
    if (setextKind) {
      style(node.from, Math.min(node.to, doc.lineAt(node.from).to), setextKind)
      return
    }

    // `---` is a rule, not three characters of punctuation. Both halves are gated
    // on the cursor together: the rule is drawn only while the dashes are hidden,
    // so the active line shows its source and nothing else - the same bargain
    // every other construct here makes, rather than the rule and its markup at
    // once.
    if (node.name === 'HorizontalRule') {
      if (!lineActive(state, node.from, activeLines)) {
        line(doc.lineAt(node.from).from, 'rule')
        hide(node.from, node.to)
      }
      return
    }

    // Fenced and indented code alike. This tints the block; concealing the fence
    // lines and surfacing the language belong to the pass that can replace a
    // whole line.
    if (node.name === 'FencedCode' || node.name === 'CodeBlock') {
      linesOf(node.from, node.to, 'codeBlock')
      return
    }

    // `-`, `*` and `+` all mean the same thing and none of them is what a list
    // looks like. Unlike every other concealment here this one is NOT released
    // when the cursor arrives: the marker carries no information to edit, so
    // flipping it back to a dash would only make the line jump.
    if (node.name === 'ListMark') {
      const marker = doc.sliceString(node.from, node.to)
      if (!BULLET_MARKERS.has(marker)) return
      const task = node.node.parent?.getChild('Task')
      if (task) {
        // A task item gets a checkbox over its whole "- [x]" run, dash included:
        // this is the one that ended the walker's task exception, where a task
        // kept its raw dash because a bullet beside a checkbox would be noise.
        // TaskMarker sits under the Task GRANDchild of the ListItem - looking for
        // it among the direct children finds nothing.
        const check = task.getChild('TaskMarker')
        if (!check) return
        const box = doc.sliceString(check.from + 1, check.to - 1)
        const checked = box === 'x' || box === 'X'
        // The checkbox is a marker: it collapses on the cursor line like every
        // other one here, so the raw `- [x]` shows again for editing.
        if (!lineActive(state, node.from, activeLines)) {
          taskSpec(node.from, check.to, checked, check.from + 1)
        }
        // A checked item's text reads muted. Styling, so it survives the cursor
        // arriving - the same bargain every content style in this walker makes.
        if (checked) style(check.to, doc.lineAt(check.to).to, 'taskDone')
        return
      }
      style(node.from, node.to, 'bullet')
      return
    }

    // `\*` is one escape node covering both characters; only the backslash goes.
    if (node.name === 'Escape') {
      hide(node.from, node.from + 1)
      return
    }

    // Only the `~~` markers. The struck look itself comes from the highlight
    // style, which - unlike this walker - is also present in source mode, so
    // taking the styling over here would lose it there.
    if (node.name === 'Strikethrough') {
      const marks = node.node.getChildren('StrikethroughMark')
      if (marks.length >= 2) {
        hide(marks[0].from, marks[0].to)
        hide(marks[marks.length - 1].from, marks[marks.length - 1].to)
      }
      return
    }

    // `<https://x>`: the angle brackets are delimiters, not address.
    if (node.name === 'Autolink') {
      const url = node.node.getChild('URL')
      if (url) style(url.from, url.to, 'link', linkHref(doc.sliceString(url.from, url.to)))
      for (const mark of node.node.getChildren('LinkMark')) hide(mark.from, mark.to)
      return
    }

    // A URL standing on its own, which GFM turns into a link. Inside a `[text](url)`
    // the URL is part of the markup the Link branch already conceals, so styling it
    // would only ever show through on the line being edited.
    if (node.name === 'URL') {
      if (node.node.parent?.name === 'Link') return
      style(node.from, node.to, 'link', linkHref(doc.sliceString(node.from, node.to)))
      return
    }

    if (node.name === 'StrongEmphasis' || node.name === 'Emphasis') {
      const marks = node.node.getChildren('EmphasisMark')
      if (marks.length >= 2) {
        const open = marks[0]
        const close = marks[marks.length - 1]
        style(open.to, close.from, node.name === 'StrongEmphasis' ? 'strong' : 'emphasis')
        hide(open.from, open.to)
        hide(close.from, close.to)
      }
      return
    }

    if (node.name === 'InlineCode') {
      const marks = node.node.getChildren('CodeMark')
      if (marks.length >= 2) {
        const open = marks[0]
        const close = marks[marks.length - 1]
        style(open.to, close.from, 'code')
        hide(open.from, open.to)
        hide(close.from, close.to)
      }
      return
    }

    if (node.name === 'Link') {
      // Only inline links carry a URL child; reference-style links and
      // wikilinks ([[...]]) don't, so they fall through untouched.
      if (!node.node.getChild('URL')) return
      const url = node.node.getChild('URL')
      const marks = node.node.getChildren('LinkMark')
      if (marks.length >= 2) {
        const open = marks[0]
        const close = marks[1]
        style(open.to, close.from, 'link', url && linkHref(doc.sliceString(url.from, url.to)))
        hide(node.from, open.to)      // "["
        hide(close.from, node.to)     // "](url)"
      }
    }
  }

  const tree = syntaxTree(state)
  for (const range of ranges) {
    bounds = range
    tree.iterate({ from: range.from, to: range.to, enter })
    collectObsidianComments(state, tree, range, style)
    collectTags(state, tree, range, style)
  }

  return specs
}

// `%%like this%%`, inline or spanning lines. It is an Obsidian convention rather
// than markdown, so no node exists for it and a scan of the text is the only way
// to find it - the same approach ./wikilinkEditor takes for `[[...]]`.
const OBSIDIAN_COMMENT = /%%[\s\S]*?%%/g
const CODE_NODES = new Set(['InlineCode', 'FencedCode', 'CodeBlock'])

function insideCode(tree: ReturnType<typeof syntaxTree>, pos: number): boolean {
  let node: { name: string; parent: unknown } | null = tree.resolveInner(pos, 1) as never
  while (node) {
    if (CODE_NODES.has(node.name)) return true
    node = node.parent as never
  }
  return false
}

function collectObsidianComments(
  state: EditorState,
  tree: ReturnType<typeof syntaxTree>,
  range: PreviewRange,
  style: (from: number, to: number, kind: PreviewKind) => void,
): void {
  const text = state.doc.sliceString(range.from, range.to)
  OBSIDIAN_COMMENT.lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = OBSIDIAN_COMMENT.exec(text))) {
    const from = range.from + match.index
    // `${var%%pattern}` in a shell block is not a comment.
    if (insideCode(tree, from)) continue
    style(from, from + match[0].length, 'comment')
  }
}

// `#tag`, `#nested/tag`. Anchored to a space or a line start so a URL fragment
// (`example.com#section`) and a heading (`## Title`, whose `#` is followed by
// another `#` or a space) are never mistaken for one.
const VAULT_TAG = /(^|\s)(#[\w/-]+)/g

function collectTags(
  state: EditorState,
  tree: ReturnType<typeof syntaxTree>,
  range: PreviewRange,
  style: (from: number, to: number, kind: PreviewKind) => void,
): void {
  const text = state.doc.sliceString(range.from, range.to)
  VAULT_TAG.lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = VAULT_TAG.exec(text))) {
    const tag = match[2]
    // `#1` is an issue reference, `#fff` in prose is a colour. A tag has to carry
    // at least one character that is not a digit.
    if (!/[^\d#]/.test(tag)) continue
    const from = range.from + match.index + match[1].length
    if (insideCode(tree, from)) continue
    style(from, from + tag.length, 'tag')
  }
}

class BulletWidget extends WidgetType {
  toDOM(): HTMLElement {
    const dot = document.createElement('span')
    dot.className = 'cm-md-bullet'
    dot.textContent = '\u2022'
    return dot
  }
  // Every bullet is the same bullet, so CodeMirror can reuse the DOM node.
  eq(): boolean { return true }
}

// The checkbox standing in for a task item's "- [x]" run. Clicking it dispatches
// ONE transaction flipping the character inside the brackets, so the change hits
// the document (autosave persists it) and one undo step reverts it - the
// isolateHistory annotation keeps two quick clicks from merging into a single
// history event under the 500ms newGroupDelay.
export class TaskCheckboxWidget extends WidgetType {
  constructor(
    readonly checked: boolean,
    readonly toggleAt: number,
    readonly editable: boolean,
  ) { super() }

  toDOM(view?: EditorView): HTMLElement {
    const wrap = document.createElement('span')
    wrap.className = 'cm-md-task-check'
    const box = wrap.appendChild(document.createElement('input'))
    box.setAttribute('type', 'checkbox')
    box.checked = this.checked
    // Reading mode has the document faceted uneditable: the box then only shows
    // its state, and must never act as a control there.
    box.disabled = !this.editable
    // preventDefault on mousedown keeps CodeMirror from moving the caret onto
    // the line - it skips its handlers once an event is default-prevented - which
    // would collapse the concealment and destroy this widget mid-click. It also
    // keeps the browser from pulling focus off the editor.
    box.addEventListener('mousedown', (event) => event.preventDefault())
    box.addEventListener('click', (event) => {
      event.preventDefault()
      if (!this.editable) return
      // The model decides the next state; the checkbox only renders it. Unchecking
      // normalizes to a space, checking writes lowercase x - the same flip GFM
      // spellings accept. The dispatch re-renders this widget through eq().
      view?.dispatch({
        changes: { from: this.toggleAt, to: this.toggleAt + 1, insert: this.checked ? ' ' : 'x' },
        annotations: isolateHistory.of('full'),
        userEvent: 'input',
      })
    })
    return wrap
  }

  // Identical boxes can reuse the DOM; a different state or edit target cannot.
  eq(other: TaskCheckboxWidget): boolean {
    return other.checked === this.checked
      && other.toggleAt === this.toggleAt
      && other.editable === this.editable
  }
}

const bulletDeco = Decoration.replace({ widget: new BulletWidget() })

const KIND_CLASS: Partial<Record<PreviewKind, string>> = {
  h1: 'cm-md-h1', h2: 'cm-md-h2', h3: 'cm-md-h3',
  h4: 'cm-md-h4', h5: 'cm-md-h5', h6: 'cm-md-h6',
  strong: 'cm-md-strong',
  emphasis: 'cm-md-emphasis',
  code: 'cm-md-code',
  link: 'cm-md-link',
  comment: 'cm-md-comment',
  tag: 'cm-md-tag',
  taskDone: 'cm-md-task-done',
}

const LINE_CLASS: Partial<Record<PreviewKind, string>> = {
  rule: 'cm-md-rule',
  codeBlock: 'cm-md-code-block',
}

const hideDeco = Decoration.replace({})

function specToDecoration(spec: PreviewSpec, editable: boolean): Decoration {
  if (spec.kind === 'hide') return hideDeco
  if (spec.kind === 'bullet') return bulletDeco
  if (spec.kind === 'task') {
    return Decoration.replace({
      widget: new TaskCheckboxWidget(
        spec.checked === true,
        // The collector always sets toggleAt on 'task' specs; the fields are
        // optional in the shared interface only so the other kinds omit them.
        spec.toggleAt ?? spec.from,
        editable,
      ),
    })
  }
  const lineClass = LINE_CLASS[spec.kind]
  if (lineClass) return Decoration.line({ class: lineClass })
  const cls = KIND_CLASS[spec.kind] ?? ''
  if (!spec.href) return Decoration.mark({ class: cls })
  return Decoration.mark({
    class: `${cls} cm-md-link--live`,
    attributes: { 'data-md-href': spec.href, title: spec.href },
  })
}

// `reveal` controls the raw-on-cursor-line behaviour: in live-preview the line
// under the cursor shows its raw markers (so you can edit them); in reading
// mode nothing is active, so every marker stays concealed for a clean read.
export function previewDecorations(
  state: EditorState,
  reveal: boolean,
  ranges?: readonly PreviewRange[],
): DecorationSet {
  const activeLines = reveal ? computeActiveLines(state) : new Set<number>()
  const specs = collectPreviewSpecs(state, activeLines, ranges)
  // The document's editability reaches the widget through the spec: in reading
  // mode it renders disabled and a click must not change the document.
  const editable = state.facet(EditorView.editable)
  // sort=true is not optional. Specs come out of a tree walk, and a heading emits
  // a mark and a replace starting at the SAME offset - which RangeSet orders by
  // startSide, not by `from`. Sorting on `from` alone throws "Ranges must be added
  // sorted", the plugin is dropped whole, and the note renders as raw markdown.
  return Decoration.set(specs.map((s) => specToDecoration(s, editable).range(s.from, s.to)), true)
}

function livePreviewPlugin(reveal: boolean) {
  return ViewPlugin.fromClass(
    class {
      decorations: DecorationSet
      constructor(view: EditorView) {
        this.decorations = previewDecorations(view.state, reveal, view.visibleRanges)
      }
      update(update: ViewUpdate) {
        // Selection changes move the active line, so reveal/conceal must refresh
        // on selectionSet too, not just docChanged.
        if (update.docChanged || update.selectionSet || update.viewportChanged) {
          this.decorations = previewDecorations(update.view.state, reveal, update.view.visibleRanges)
        }
      }
    },
    {
      decorations: (v) => v.decorations,
      eventHandlers: {
        click(event: MouseEvent) {
          // Plain click only: a modifier click belongs to the browser and to text
          // selection. Mirrors how a wikilink pill navigates.
          if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return false
          const el = (event.target as HTMLElement)?.closest('[data-md-href]') as HTMLElement | null
          const href = el?.getAttribute('data-md-href')
          if (!href) return false
          window.open(href, '_blank', 'noopener,noreferrer')
          event.preventDefault()
          return true
        },
      },
    },
  )
}

// Inline styling for the concealed-marker content. Sizes/weights track the
// editor's existing IBM-Plex look; colors come from the @theme tokens so the
// preview restyles centrally with the rest of the app (U2/U2.1).
// Heading sizes multiply by --notes-heading-scale (default 1) so the settings
// popover can tune heading prominence without touching this module.
export const livePreviewTheme = EditorView.theme({
  '.cm-md-h1': { fontSize: 'calc(1.6em * var(--notes-heading-scale, 1))', fontWeight: '600', lineHeight: '1.3' },
  '.cm-md-h2': { fontSize: 'calc(1.4em * var(--notes-heading-scale, 1))', fontWeight: '600', lineHeight: '1.3' },
  '.cm-md-h3': { fontSize: 'calc(1.2em * var(--notes-heading-scale, 1))', fontWeight: '600', lineHeight: '1.3' },
  '.cm-md-h4': { fontSize: 'calc(1.1em * var(--notes-heading-scale, 1))', fontWeight: '600' },
  '.cm-md-h5': { fontSize: 'calc(1em * var(--notes-heading-scale, 1))', fontWeight: '600' },
  '.cm-md-h6': { fontSize: '1em', fontWeight: '600', color: 'var(--color-text-muted)' },
  '.cm-md-strong': { fontWeight: '700' },
  '.cm-md-emphasis': { fontStyle: 'italic' },
  '.cm-md-code': {
    fontFamily: 'var(--font-mono-data, monospace)',
    fontSize: '0.9em',
    backgroundColor: 'color-mix(in srgb, var(--color-on-surface) 8%, transparent)',
    borderRadius: 'var(--radius-lg)',
    padding: '0 3px',
  },
  // The interface accent, not the assistant's: DESIGN.md reserves `da-accent` for
  // surfaces where the DA is involved, and a link in a note is the writer's.
  '.cm-md-link': {
    color: 'var(--color-primary-text)',
    textDecoration: 'underline',
    textUnderlineOffset: '2px',
    transition: 'color 120ms ease',
  },
  // Only a link that resolves to something openable offers to be clicked.
  '.cm-md-link--live': { cursor: 'pointer' },
  '.cm-md-link--live:hover': { color: 'var(--color-primary)' },

  // A rule is drawn by the line, not by its characters, so the `---` can be
  // concealed without the line collapsing to nothing. 1px: DESIGN.md forbids
  // heavier rules, and a separator that announces itself stops separating.
  //
  // Painted as a centred background band rather than a border-bottom, which would
  // sit on the line's lower edge - reading as a divider between the two lines
  // around it instead of as the content of its own.
  '.cm-md-rule': {
    backgroundImage: 'linear-gradient(var(--color-border-default), var(--color-border-default))',
    backgroundSize: '100% 1px',
    backgroundPosition: 'center',
    backgroundRepeat: 'no-repeat',
  },

  // The tint runs the full width of the line rather than hugging the text, which
  // is what makes a block of code read as one object instead of as several
  // monospaced sentences. Lighter than inline code's 8%: this covers far more
  // area, and at equal strength the page starts to look striped.
  // Matches how the highlight style renders an HTML comment, because both say the
  // same thing: this is here for the writer and not for the reader.
  '.cm-md-comment': {
    color: 'var(--color-text-faint)',
    fontStyle: 'italic',
  },

  // The glyph sits where the marker was, so nothing reflows when a line becomes
  // a list item. Muted, because a bullet is punctuation for the eye.
  '.cm-md-bullet': {
    color: 'var(--color-text-muted)',
  },

  // A native checkbox tinted with the muted token: the box and the done text are
  // one quiet signal, and the browser's default accent blue would break the
  // neutral palette (DESIGN.md: no rainbow). Sized to the text so the line's
  // rhythm holds.
  '.cm-md-task-check': {
    display: 'inline-block',
  },
  '.cm-md-task-check input': {
    width: '0.95em',
    height: '0.95em',
    verticalAlign: '-0.1em',
    accentColor: 'var(--color-text-muted)',
    cursor: 'pointer',
  },
  '.cm-md-task-check input:disabled': {
    cursor: 'default',
  },
  // A checked task reads as done at the same muted tone the strikethrough was
  // moved to (too faint there). No line-through: the checkbox already carries
  // the state, striking the text would say it twice.
  '.cm-md-task-done': {
    color: 'var(--color-text-muted)',
  },

  // A chip, not coloured text: a tag is a handle on the note rather than part of
  // the sentence, and the surface says so without spending the accent on it.
  '.cm-md-tag': {
    backgroundColor: 'var(--color-surface-chip)',
    color: 'var(--color-text-secondary)',
    borderRadius: 'var(--radius-sm)',
    padding: '1px 5px',
    fontSize: '0.92em',
  },

  '.cm-md-code-block': {
    backgroundColor: 'color-mix(in srgb, var(--color-on-surface) 5%, transparent)',
  },
})

// Single bundle for the editor's extensions array, mirroring wikilinkExtensions.
// reveal=true (default) is live-preview; reveal=false is reading mode.
export function livePreviewExtensions(reveal = true) {
  return [livePreviewPlugin(reveal), livePreviewTheme]
}
