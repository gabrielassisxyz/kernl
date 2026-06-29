// CodeMirror 6 wiring for the wikilink feature: an async autocomplete source
// that talks to the node search API, and a decoration that renders inline
// [[uuid|alias]] / [[target]] links as compact, readable pills. The pure
// parsing/formatting logic lives in ./wikilinkComplete (unit-tested); this
// module is the thin glue to CM6 + the node-type registry.

import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete'
import { autocompletion } from '@codemirror/autocomplete'
import {
  Decoration,
  EditorView,
  MatchDecorator,
  ViewPlugin,
  type DecorationSet,
  type ViewUpdate,
} from '@codemirror/view'
import { parseWikilinkQuery, buildWikilinkInsert } from './wikilinkComplete'
import { metaForType } from './nodeTypes'

interface SearchNode {
  id: string
  title: string
  type: string
}

// ----- Autocomplete source -----------------------------------------------

// Matches the open-wikilink trigger so we know the range to replace. Mirrors the
// regex parseWikilinkQuery uses, but here we only need the matched range.
const TRIGGER = /\[\[[^\]\n]*$/

async function fetchNodes(query: string, type: string | undefined): Promise<SearchNode[]> {
  const params = new URLSearchParams({ q: query, limit: '8' })
  if (type) params.set('type', type)
  const res = await fetch(`/api/nodes/search?${params.toString()}`)
  if (!res.ok) return []
  return (await res.json()) as SearchNode[]
}

export function wikilinkCompletionSource() {
  return async (ctx: CompletionContext): Promise<CompletionResult | null> => {
    const match = ctx.matchBefore(TRIGGER)
    if (!match) return null

    const parsed = parseWikilinkQuery(match.text)
    if (!parsed.active) return null

    // Don't fire an empty search on a bare "[[" unless explicitly invoked.
    if (!parsed.query && !ctx.explicit) return null

    const nodes = await fetchNodes(parsed.query, parsed.type)
    if (!nodes.length) return null

    return {
      // Replace from the opening "[[" through the cursor.
      from: match.from,
      to: ctx.pos,
      // We rely on the server's prefix-match ordering, not CM's fuzzy filter.
      filter: false,
      options: nodes.map((node) => {
        const meta = metaForType(node.type)
        return {
          label: node.title,
          detail: meta.label,
          // Insert "[[<uuid>|<title>]]"; caret lands after the "]]".
          apply: buildWikilinkInsert(node),
          // Per-row type chip (icon + color from the registry).
          info: () => {
            const el = document.createElement('span')
            el.className = 'cm-wl-info'
            const icon = document.createElement('span')
            icon.className = 'material-symbols-outlined cm-wl-info-icon'
            icon.style.color = meta.color
            icon.textContent = meta.icon
            const lbl = document.createElement('span')
            lbl.textContent = meta.label
            el.appendChild(icon)
            el.appendChild(lbl)
            return el
          },
        }
      }),
    }
  }
}

export function wikilinkAutocomplete() {
  return autocompletion({
    override: [wikilinkCompletionSource()],
    icons: false,
  })
}

// ----- Pill decoration ----------------------------------------------------

// Matches [[target]] or [[target|alias]] on a single line. Group 1 = target,
// group 2 = optional alias.
const PILL_RE = /\[\[([^\]\n|]+)(?:\|([^\]\n]*))?\]\]/g

// Build decorations for one match: a pill background over the whole link, a
// de-emphasis mark over the "[[uuid|" prefix and the trailing "]]", and an
// emphasised mark over the alias so it reads cleanly. Mark decorations (not a
// replace widget) keep every character editable — clicking in and arrow keys
// behave normally, which is the robust choice the brief asks for.
function decorateMatch(
  add: (from: number, to: number, deco: Decoration) => void,
  from: number,
  to: number,
  match: RegExpExecArray,
) {
  const target = match[1]
  const hasAlias = match[2] !== undefined
  const aliasStart = hasAlias ? from + 2 + target.length + 1 : from + 2
  const aliasEnd = to - 2

  // Ranges must reach the RangeSetBuilder sorted by `from`. All four are marks
  // (startSide 5e8), so document order is enough: whole-link wrapper, then the
  // opening "[[uuid|", the alias, and the closing "]]".

  // Whole-link wrapper (hover target + click navigation via data-wl-target).
  add(
    from,
    to,
    Decoration.mark({ class: 'cm-wl-pill', attributes: { 'data-wl-target': target } }),
  )
  // The structural "[[" + (uuid + "|") part — concealed until hover.
  add(from, aliasStart, Decoration.mark({ class: 'cm-wl-bracket' }))
  // The alias (or, for [[target]], the target itself) — shown in the link accent.
  add(aliasStart, aliasEnd, Decoration.mark({ class: 'cm-wl-alias' }))
  // The closing "]]" — concealed until hover.
  add(aliasEnd, to, Decoration.mark({ class: 'cm-wl-bracket' }))
}

const wikilinkMatcher = new MatchDecorator({
  regexp: PILL_RE,
  decorate: (add, from, to, match) => decorateMatch(add, from, to, match),
})

// onPillClick: best-effort navigation hook. The plugin emits the clicked
// target via this callback (wired to a Vue event in the component).
export function wikilinkPills(onPillClick?: (target: string) => void) {
  return ViewPlugin.fromClass(
    class {
      decorations: DecorationSet
      constructor(view: EditorView) {
        this.decorations = wikilinkMatcher.createDeco(view)
      }
      update(update: ViewUpdate) {
        this.decorations = wikilinkMatcher.updateDeco(update, this.decorations)
      }
    },
    {
      decorations: (v) => v.decorations,
      eventHandlers: {
        click(event: MouseEvent) {
          if (!onPillClick) return false
          // Best-effort: ctrl/cmd-click a pill to navigate. Plain clicks still
          // place the caret for editing.
          if (!event.ctrlKey && !event.metaKey) return false
          const el = (event.target as HTMLElement)?.closest('.cm-wl-pill') as HTMLElement | null
          const target = el?.getAttribute('data-wl-target')
          if (target) {
            onPillClick(target)
            return true
          }
          return false
        },
      },
    },
  )
}

// Theme: Obsidian-style wikilinks. The link reads as plain accent-coloured text
// with its [[ ]] brackets (and any uuid) concealed; hovering reveals the raw
// brackets and shows a pointer, signalling it's navigable (ctrl/cmd-click).
export const wikilinkTheme = EditorView.theme({
  '.cm-wl-pill:hover': {
    cursor: 'pointer',
  },
  // The visible link text, coloured with the note-node accent.
  '.cm-wl-alias': {
    color: 'var(--color-node-note)',
    transition: 'color 120ms ease',
  },
  '.cm-wl-pill:hover .cm-wl-alias': {
    textDecoration: 'underline',
    textUnderlineOffset: '2px',
  },
  // "[[", trailing "]]", and any "uuid|" — concealed until hover.
  '.cm-wl-bracket': {
    display: 'none',
    color: 'var(--color-text-dim)',
  },
  '.cm-wl-pill:hover .cm-wl-bracket': {
    display: 'inline',
  },

  // --- Completion popup, themed to the dark IBM-Plex editor ---
  '.cm-tooltip.cm-tooltip-autocomplete': {
    border: '1px solid var(--color-border-default)',
    borderRadius: '6px',
    backgroundColor: 'var(--color-surface-overlay)',
    overflow: 'hidden',
  },
  '.cm-tooltip-autocomplete > ul': {
    fontFamily: 'inherit',
    maxHeight: '15rem',
  },
  '.cm-tooltip-autocomplete > ul > li': {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: '12px',
    padding: '5px 10px',
    color: 'var(--color-text-primary)',
    lineHeight: '1.4',
  },
  '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
    backgroundColor: 'var(--color-surface-hover)',
    color: 'var(--color-on-surface)',
  },
  // The node title.
  '.cm-tooltip-autocomplete .cm-completionLabel': {
    flex: '1 1 auto',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  // The type label (from `detail`) reads as a quiet chip on the right.
  '.cm-tooltip-autocomplete .cm-completionDetail': {
    flex: '0 0 auto',
    fontStyle: 'normal',
    fontSize: '10px',
    letterSpacing: '0.04em',
    textTransform: 'uppercase',
    color: 'var(--color-text-muted)',
  },

  // Completion type chip (rendered in the info panel).
  '.cm-wl-info': {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '12px',
    color: 'var(--color-text-muted)',
  },
  '.cm-wl-info-icon': {
    fontSize: '16px',
    lineHeight: '1',
  },
})

// Single extension bundle for the editor to drop into its extensions array.
export function wikilinkExtensions(onPillClick?: (target: string) => void) {
  return [wikilinkAutocomplete(), wikilinkPills(onPillClick), wikilinkTheme]
}
