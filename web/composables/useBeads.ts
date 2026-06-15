import { ref, computed } from 'vue'

// Mirrors backend.Bead (internal/backend/port.go) — JSON is already camelCase.
export interface Bead {
  id: string
  type: string
  state: string
  title: string
  description?: string
  notes?: string
  acceptance?: string
  priority: number
  labels: string[]
  parentId?: string
  workflowId?: string
  workflowMode?: string
  nextActionState?: string
  nextActionOwnerKind?: string
  requiresHumanAction?: boolean
  isAgentClaimable?: boolean
  createdAt: string
  updatedAt: string
  closedAt?: string
}

/**
 * Single source of truth for bead data across the Projects, Tasks and
 * Orchestrator surfaces. Backed by GET /api/beads (server-side memoized 2s).
 */
export function useBeads() {
  const beads = ref<Bead[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const res = await fetch('/api/beads')
      if (!res.ok) throw new Error(`GET /api/beads → ${res.status}`)
      beads.value = await res.json()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  const epics = computed(() => beads.value.filter((b) => b.type === 'epic'))
  const tasks = computed(() => beads.value.filter((b) => b.type !== 'epic'))

  function childrenOf(parentId: string): Bead[] {
    return beads.value.filter((b) => b.parentId === parentId)
  }

  return { beads, loading, error, load, epics, tasks, childrenOf }
}
