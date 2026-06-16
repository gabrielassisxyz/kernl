// Single source of truth for graph node types: color, human label, Material
// Symbols icon, and a short prefix the editor autocomplete keys off. Plain TS
// (no Vue/Nuxt imports) so the graph view, the editor, and unit tests can all
// share it. Colors are a cohesive muted jewel-tone palette over the dark base.

export interface NodeTypeMeta {
  type: string
  label: string
  color: string
  icon: string
  prefix: string
}

const FALLBACK_COLOR = '#9098A7'
const FALLBACK_ICON = 'circle'

export const NODE_TYPES: NodeTypeMeta[] = [
  { type: 'note', label: 'note', color: '#7B8FE0', icon: 'description', prefix: 'n' },
  { type: 'bookmark', label: 'bookmark', color: '#E5C270', icon: 'bookmark', prefix: 'b' },
  { type: 'bookmark_list', label: 'bookmark list', color: '#D49A6A', icon: 'bookmarks', prefix: 'l' },
  { type: 'task', label: 'task', color: '#6D9A78', icon: 'checklist', prefix: 't' },
  { type: 'project', label: 'project', color: '#C2675C', icon: 'folder_open', prefix: 'p' },
  { type: 'memory_claim', label: 'memory claim', color: '#B58BD4', icon: 'neurology', prefix: 'm' },
  { type: 'chat_session', label: 'chat session', color: '#5FA8C4', icon: 'forum', prefix: 'c' },
  { type: 'ingest_item', label: 'ingest item', color: '#8089A0', icon: 'input', prefix: 'i' },
  { type: 'capture', label: 'capture', color: '#D98E73', icon: 'bolt', prefix: 'x' },
  { type: 'memory_refutation', label: 'memory refutation', color: '#C76B7A', icon: 'cancel', prefix: 'r' },
  { type: 'decision', label: 'decision', color: '#5FB39A', icon: 'fork_right', prefix: 'd' },
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
