// Client-side twin of internal/graph/tagname (Go). The server is the authority —
// it normalises and rejects on every write path — but the input needs the same
// rules locally to tell the user *before* the request that `sys/` is reserved or
// that `foo//bar` is not a name, and to send the same canonical string the server
// would have derived anyway.
//
// Plain TS (no Vue imports) so the input component and unit tests share it.

export const TAG_SEPARATOR = '/'

// Machine-authored tags live under this prefix so they never compete with the
// user's own subjects in the tag surface. Mirrors tags.SystemPrefix.
export const SYSTEM_PREFIX = 'sys/'

export const isSystemTag = (name: string): boolean => name.startsWith(SYSTEM_PREFIX)

export interface TagNameResult {
  name?: string
  error?: string
}

// Canonical form: trimmed, lowercased, every `/`-separated segment trimmed.
// Lowercasing is what makes a tag a matching axis rather than prose — the graph
// stores names UNIQUE, so `Homelab` and `homelab` must converge.
export function normalizeTag(raw: string): TagNameResult {
  const trimmed = raw.trim()
  if (!trimmed) return { error: 'Tag name is empty.' }

  const segments = trimmed.toLowerCase().split(TAG_SEPARATOR).map((s) => s.trim())
  if (segments.some((s) => s === '')) {
    return { error: `"${raw}" has an empty segment. Nest tags as parent/child.` }
  }

  const name = segments.join(TAG_SEPARATOR)
  if (isSystemTag(name)) {
    return { error: `"${SYSTEM_PREFIX}" is reserved for system tags.` }
  }
  return { name }
}
