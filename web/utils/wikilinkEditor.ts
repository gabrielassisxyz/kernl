// CodeMirror 6 wiring for the wikilink feature: an async autocomplete source
// that talks to the node search API, and a decoration that renders inline
// [[uuid|alias]] / [[target]] links as compact, readable pills. The pure
// parsing/formatting logic lives in ./wikilinkComplete (unit-tested); this
// module is the thin glue to CM6 + the node-type registry.

import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete'
import { autocompletion } from '@codemirror/autocomplete'
import { RangeSetBuilder, StateEffect } from '@codemirror/state'
import {
  Decoration,
  EditorView,
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

// Dispatched when the caller's resolver data (known node ids/titles) arrives
// after the editor mounted, so pills re-style without waiting for an edit.
export const wikilinkResolverUpdated = StateEffect.define<void>()

// isResolved: returns whether a wikilink target points at an existing node.
// Undefined predicate = unknown; style everything as resolved (no false alarms
// while the lookup is still loading).
export type WikilinkResolver = (target: string) => boolean

// Build decorations for the visible ranges. Mark decorations (not a replace
// widget) keep every character editable. Brackets are concealed unless the
// selection touches the link (keyboard parity with mouse hover, which reveals
// via CSS) — mirrors how markdownPreview reveals raw markers on the active line.
function buildWikilinkDeco(view: EditorView, isResolved?: WikilinkResolver): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()
  const sel = view.state.selection.main
  for (const range of view.visibleRanges) {
    const text = view.state.doc.sliceString(range.from, range.to)
    PILL_RE.lastIndex = 0
    let match: RegExpExecArray | null
    while ((match = PILL_RE.exec(text))) {
      const from = range.from + match.index
      const to = from + match[0].length
      const target = match[1]
      const hasAlias = match[2] !== undefined
      const aliasStart = hasAlias ? from + 2 + target.length + 1 : from + 2
      const aliasEnd = to - 2

      const active = sel.from <= to && sel.to >= from
      const unresolved = isResolved ? !isResolved(target) : false
      let pillClass = 'cm-wl-pill'
      if (active) pillClass += ' cm-wl-pill--active'
      if (unresolved) pillClass += ' cm-wl-unresolved'

      // Whole-link wrapper (hover target + click navigation via data-wl-target),
      // then the "[[uuid|" prefix, the alias, and the closing "]]" — in document
      // order, as RangeSetBuilder requires.
      builder.add(from, to, Decoration.mark({ class: pillClass, attributes: { 'data-wl-target': target } }))
      
      if (hasAlias) {
        // Render [[
        builder.add(from, from + 2, Decoration.mark({ class: 'cm-wl-bracket' }))
        // Completely hide the target and the pipe (UUID|)
        builder.add(from + 2, aliasStart, Decoration.replace({}))
      } else {
        // No alias, so aliasStart is from + 2. Just render [[
        builder.add(from, aliasStart, Decoration.mark({ class: 'cm-wl-bracket' }))
      }
      
      builder.add(aliasStart, aliasEnd, Decoration.mark({ class: 'cm-wl-alias' }))
      builder.add(aliasEnd, to, Decoration.mark({ class: 'cm-wl-bracket' }))
    }
  }
  return builder.finish()
}

// onPillClick: best-effort navigation hook. The plugin emits the clicked
// target via this callback (wired to a Vue event in the component).
export function wikilinkPills(onPillClick?: (target: string) => void, isResolved?: WikilinkResolver) {
  return ViewPlugin.fromClass(
    class {
      decorations: DecorationSet
      constructor(view: EditorView) {
        this.decorations = buildWikilinkDeco(view, isResolved)
      }
      update(update: ViewUpdate) {
        const resolverArrived = update.transactions.some((tr) =>
          tr.effects.some((e) => e.is(wikilinkResolverUpdated)),
        )
        // selectionSet: bracket reveal follows the keyboard cursor, not just hover.
        if (update.docChanged || update.viewportChanged || update.selectionSet || resolverArrived) {
          this.decorations = buildWikilinkDeco(update.view, isResolved)
        }
      }
    },
    {
      decorations: (v) => v.decorations,
      eventHandlers: {
        click(event: MouseEvent) {
          if (!onPillClick) return false
          // Plain click navigates (the UAT-expected behaviour); the text stays
          // editable via keyboard and by clicking just outside the pill.
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
    filter: 'brightness(1.2) saturate(1.2)',
  },
  // "[[", trailing "]]", and any "uuid|" — concealed until hover OR until the
  // keyboard selection touches the link (cm-wl-pill--active).
  '.cm-wl-bracket': {
    display: 'none',
    color: 'var(--color-text-dim)',
  },
  '.cm-wl-pill:hover .cm-wl-bracket, .cm-wl-pill--active .cm-wl-bracket': {
    display: 'inline',
  },
  // Unresolved target: same hue, visibly desaturated, so a dangling link is
  // distinguishable at a glance without shouting.
  '.cm-wl-unresolved .cm-wl-alias': {
    color: 'color-mix(in srgb, var(--color-node-note) 45%, var(--color-text-muted))',
  },
  '.cm-wl-unresolved:hover .cm-wl-alias': {
    color: 'var(--color-node-note)',
    filter: 'none',
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
export function wikilinkExtensions(onPillClick?: (target: string) => void, isResolved?: WikilinkResolver) {
  return [wikilinkAutocomplete(), wikilinkPills(onPillClick, isResolved), wikilinkTheme]
}
