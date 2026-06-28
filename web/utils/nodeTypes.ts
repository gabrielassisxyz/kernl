// Single source of truth for graph node types: color, human label, Material
// Symbols icon, and a short prefix the editor autocomplete keys off. Plain TS
// (no Vue/Nuxt imports) so the graph view, the editor, and unit tests can all
// share it. Colors point at the live design-token source in tailwind.css.

export interface NodeTypeMeta {
  type: string
  label: string
  color: string
  icon: string
  prefix: string
}

const tokenColor = (token: string): string => `var(--color-${token})`

const FALLBACK_COLOR = tokenColor('text-muted')
const FALLBACK_ICON = 'circle'

export const NODE_TYPES: NodeTypeMeta[] = [
  { type: 'note', label: 'note', color: tokenColor('node-note'), icon: 'description', prefix: 'n' },
  { type: 'bookmark', label: 'bookmark', color: tokenColor('tertiary'), icon: 'bookmark', prefix: 'b' },
  { type: 'bookmark_list', label: 'bookmark list', color: tokenColor('node-bookmark-list'), icon: 'bookmarks', prefix: 'l' },
  { type: 'task', label: 'task', color: tokenColor('status-passed'), icon: 'checklist', prefix: 't' },
  { type: 'project', label: 'project', color: tokenColor('status-failed'), icon: 'folder_open', prefix: 'p' },
  { type: 'memory_claim', label: 'memory claim', color: tokenColor('node-memory-claim'), icon: 'neurology', prefix: 'm' },
  { type: 'chat_session', label: 'chat session', color: tokenColor('node-chat-session'), icon: 'forum', prefix: 'c' },
  { type: 'ingest_item', label: 'ingest item', color: tokenColor('status-running'), icon: 'input', prefix: 'i' },
  { type: 'capture', label: 'capture', color: tokenColor('node-capture'), icon: 'bolt', prefix: 'x' },
  { type: 'memory_refutation', label: 'memory refutation', color: tokenColor('node-memory-refutation'), icon: 'cancel', prefix: 'r' },
  { type: 'decision', label: 'decision', color: tokenColor('node-decision'), icon: 'fork_right', prefix: 'd' },
]

const BY_TYPE = new Map(NODE_TYPES.map((m) => [m.type, m]))
const BY_PREFIX = new Map(NODE_TYPES.map((m) => [m.prefix, m.type]))

// Meta for a type, with a sensible fallback for unknown types so the UI never
// renders a node without color/label/icon.
export function metaForType(type: string): NodeTypeMeta {
  const hit = BY_TYPE.get(type)
  if (hit) return hit
  return {
    type,
    label: (type || 'node').replace(/_/g, ' '),
    color: FALLBACK_COLOR,
    icon: FALLBACK_ICON,
    prefix: '',
  }
}

export const colorForType = (type: string): string => metaForType(type).color

export const labelForType = (type: string): string => metaForType(type).label

// Resolve an editor prefix (e.g. "b") back to a type (e.g. "bookmark").
export function typeForPrefix(prefix: string): string | undefined {
  return BY_PREFIX.get(prefix)
}
