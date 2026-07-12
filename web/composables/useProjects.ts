import { ref } from 'vue'

// Mirrors api.projectDTO (internal/api/projects.go) — JSON is camelCase.
export interface Project {
  id: string
  title: string
  description: string
  status: ProjectStatus
  tags: string[]
  createdAt: string
  updatedAt: string
  taskCount: number
  doneCount: number
}

export type ProjectStatus = 'active' | 'paused' | 'done' | 'archived'

export const PROJECT_STATUSES: { id: ProjectStatus; label: string }[] = [
  { id: 'active', label: 'Active' },
  { id: 'paused', label: 'Paused' },
  { id: 'done', label: 'Done' },
  { id: 'archived', label: 'Archived' },
]

export interface NewProject {
  title: string
  description?: string
  status?: ProjectStatus
  tags?: string[]
}

/**
 * Projects are human-created organizational nodes in the graph (type "project"),
 * distinct from orchestrator beads. Backed by /api/projects.
 */
export function useProjects() {
  const projects = ref<Project[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const res = await fetch('/api/projects')
      if (!res.ok) throw new Error(`GET /api/projects → ${res.status}`)
      projects.value = await res.json()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function create(p: NewProject): Promise<string> {
    const res = await fetch('/api/projects', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(p),
    })
    if (!res.ok) throw new Error(`POST /api/projects → ${res.status}`)
    const { id } = await res.json()
    await load()
    return id
  }

  async function setStatus(id: string, status: ProjectStatus): Promise<void> {
    const res = await fetch(`/api/projects/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status }),
    })
    if (!res.ok) throw new Error(`PATCH /api/projects/${id} → ${res.status}`)
    await load()
  }

  async function update(
    id: string,
    // An omitted field is left alone; `tags: []` clears them, so a partial edit
    // must not send the key at all.
    patch: { title?: string; description?: string; status?: ProjectStatus; tags?: string[] }
  ): Promise<void> {
    const res = await fetch(`/api/projects/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    })
    if (!res.ok) throw new Error(`PATCH /api/projects/${id} → ${res.status}`)
    await load()
  }

  // Removes the project and its companion note; tasks stay, unassigned.
  async function remove(id: string): Promise<void> {
    const res = await fetch(`/api/projects/${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error(`DELETE /api/projects/${id} → ${res.status}`)
    await load()
  }

  return { projects, loading, error, load, create, setStatus, update, remove }
}
