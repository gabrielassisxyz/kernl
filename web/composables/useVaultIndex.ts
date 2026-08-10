import { ref } from 'vue'

// Mirrors api.noteIndexEntry (internal/api/notes_index.go) - JSON is camelCase.
export interface VaultNote {
  id: string
  path: string
  title: string
  /**
   * What the note IS, resolved from the entity it describes - a project's
   * companion reports "project". Every vault file is a `note` node, so this can
   * never be read off the node's own type.
   */
  category: string
  tags: string[]
  /** Resolved authorship: "human", "agent:da", or a human identifier. */
  author: string
  pinned: boolean
  createdAt: string
  updatedAt: string
}

/** A note nobody but the user wrote is unmarked; the badge means "not yours". */
export const isAgentAuthored = (note: VaultNote): boolean => note.author.startsWith('agent:')

/**
 * The vault index behind both tabs of the Notes panel: the Files tab lists
 * these rows, and the Tags tab derives its whole tree from their tags. One
 * request for both, so the tree and the list can never disagree about what the
 * vault holds.
 */
export function useVaultIndex() {
  const notes = ref<VaultNote[]>([])
  const pinnedTags = ref<string[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const res = await fetch('/api/notes')
      if (!res.ok) throw new Error(`GET /api/notes → ${res.status}`)
      const body = await res.json()
      notes.value = body.notes || []
      pinnedTags.value = body.pinnedTags || []
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  // Both pins update in place rather than reloading the index: a pin moves a
  // row between sections, and refetching 700 rows to learn one boolean makes
  // the list flicker under a control the user may click repeatedly.
  async function setNotePinned(id: string, pinned: boolean): Promise<void> {
    const res = await fetch(`/api/notes/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ pinned }),
    })
    if (!res.ok) throw new Error(`PATCH /api/notes/${id} → ${res.status}`)
    const hit = notes.value.find((n) => n.id === id)
    if (hit) hit.pinned = pinned
  }

  async function setTagPinned(name: string, pinned: boolean): Promise<void> {
    const res = await fetch('/api/notes/tags', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, pinned }),
    })
    if (!res.ok) throw new Error(`PATCH /api/notes/tags → ${res.status}`)
    const without = pinnedTags.value.filter((t) => t !== name)
    pinnedTags.value = pinned ? [...without, name].sort() : without
  }

  return { notes, pinnedTags, loading, error, load, setNotePinned, setTagPinned }
}
