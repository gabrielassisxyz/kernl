import { ref } from 'vue'

// Mirrors api.tagTreeDTO (internal/api/tags.go) — JSON is camelCase.
// Count and byType are subtree-inclusive: they are the size of the list you get
// by clicking the tag, so the badge and the page behind it cannot disagree.
export interface TagNode {
  name: string
  segment: string
  count: number
  byType: Record<string, number>
  children: TagNode[]
}

// Mirrors api.taggedNodeDTO. Path is populated for notes only — notes are
// file-backed and the Notes UI opens them by vault path, not by node id.
export interface TaggedNode {
  id: string
  title: string
  type: string
  updatedAt: string
  path?: string
}

/**
 * Tags are the one axis that connects a note, a task and a bookmark that are
 * about the same subject. Backed by the type-agnostic /api/tags endpoints;
 * system tags (`sys/*`) are excluded, as they are bookkeeping, not subjects.
 */
export function useTags() {
  const tree = ref<TagNode[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function loadTree(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const res = await fetch('/api/tags')
      if (!res.ok) throw new Error(`GET /api/tags → ${res.status}`)
      tree.value = await res.json()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  return { tree, loading, error, loadTree }
}

/**
 * Nodes carrying `tag` or any tag nested under it, optionally narrowed to one
 * node type. The tag travels as a query parameter because tag names contain `/`.
 */
export async function fetchTagNodes(tag: string, type = ''): Promise<TaggedNode[]> {
  const params = new URLSearchParams({ tag })
  if (type) params.set('type', type)
  const res = await fetch(`/api/tags/nodes?${params}`)
  if (!res.ok) throw new Error(`GET /api/tags/nodes?${params} → ${res.status}`)
  return res.json()
}
