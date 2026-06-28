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
  WidgetType,
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

// A small leading dot that hints "this is a link". Kept as an atomic widget so
// it can't be split by the caret; the link text itself stays plain editable
// text (mark decoration), so editing the underlying [[uuid|alias]] still works.
class WikilinkDotWidget extends WidgetType {
  eq() {
    return true
  }
  toDOM() {
    const dot = document.createElement('span')
    dot.className = 'cm-wl-dot'
    return dot
  }
  ignoreEvent() {
    return false
  }
}

const dotWidget = new WikilinkDotWidget()

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

  // Whole-link pill background.
  add(
    from,
    to,
    Decoration.mark({ class: 'cm-wl-pill', attributes: { 'data-wl-target': target } }),
  )
  // Leading dot widget, just inside the pill.
  add(from, from, Decoration.widget({ widget: dotWidget, side: 1 }))

  // Dim the structural "[[" + (uuid + "|") part. For an alias-less link this is
  // just the "[[" brackets.
  add(from, aliasStart, Decoration.mark({ class: 'cm-wl-dim' }))
  // Dim the closing "]]".
  add(aliasEnd, to, Decoration.mark({ class: 'cm-wl-dim' }))
  // Emphasise the alias (or, for [[target]], the target itself).
  add(aliasStart, aliasEnd, Decoration.mark({ class: 'cm-wl-alias' }))
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

// Theme: pills + completion menu chip, matching the editor's dark IBM-Plex look.
export const wikilinkTheme = EditorView.theme({
  '.cm-wl-pill': {
    backgroundColor: 'color-mix(in srgb, var(--color-da-accent) 10%, transparent)',
    borderRadius: '4px',
    padding: '0 3px',
    boxShadow: 'inset 0 0 0 1px color-mix(in srgb, var(--color-da-accent) 22%, transparent)',
    transition: 'background-color 120ms ease, box-shadow 120ms ease',
  },
  // Brighten on hover so the link reads as interactive (ctrl/cmd-click navigates).
  '.cm-wl-pill:hover': {
    backgroundColor: 'color-mix(in srgb, var(--color-da-accent) 18%, transparent)',
    boxShadow: 'inset 0 0 0 1px color-mix(in srgb, var(--color-da-accent) 40%, transparent)',
  },
  '.cm-wl-dot': {
    display: 'inline-block',
    width: '5px',
    height: '5px',
    marginRight: '4px',
    borderRadius: '50%',
    backgroundColor: 'var(--color-da-accent)',
    boxShadow: '0 0 0 2px color-mix(in srgb, var(--color-da-accent) 15%, transparent)',
    transform: 'translateY(-1px)',
    verticalAlign: 'middle',
  },
  '.cm-wl-dim': {
    color: 'var(--color-text-dim)',
    opacity: '0.55',
    transition: 'color 120ms ease, opacity 120ms ease',
  },
  '.cm-wl-alias': {
    color: 'var(--color-primary-fixed-dim)',
    transition: 'color 120ms ease',
  },
  '.cm-wl-pill:hover .cm-wl-alias': {
    color: 'var(--color-primary-fixed)',
  },
  // De-emphasised uuid/brackets fade up slightly on hover so the full target is
  // legible when the user is about to navigate, without ever fully shouting.
  '.cm-wl-pill:hover .cm-wl-dim': {
    opacity: '0.75',
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
