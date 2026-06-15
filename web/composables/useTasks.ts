import { ref } from 'vue'

// Mirrors api.taskDTO (internal/api/tasks.go) — JSON is camelCase.
export interface Task {
  id: string
  title: string
  description: string
  status: TaskStatus
  projectId: string
  createdAt: string
  updatedAt: string
}

export type TaskStatus = 'todo' | 'in_progress' | 'done'

export const TASK_STATUSES: { id: TaskStatus; label: string }[] = [
  { id: 'todo', label: 'To do' },
  { id: 'in_progress', label: 'In progress' },
  { id: 'done', label: 'Done' },
]

export interface NewTask {
  title: string
  description?: string
  status?: TaskStatus
  projectId?: string
}

/**
 * Tasks are human-created organizational nodes in the graph (type "task"),
 * distinct from orchestrator beads. A task may belong to a project. Backed by
 * /api/tasks (optionally filtered by ?project=).
 */
export function useTasks() {
  const tasks = ref<Task[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(projectId?: string): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const url = projectId
        ? `/api/tasks?project=${encodeURIComponent(projectId)}`
        : '/api/tasks'
      const res = await fetch(url)
      if (!res.ok) throw new Error(`GET ${url} → ${res.status}`)
      tasks.value = await res.json()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function create(t: NewTask): Promise<string> {
    const res = await fetch('/api/tasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(t),
    })
    if (!res.ok) throw new Error(`POST /api/tasks → ${res.status}`)
    const { id } = await res.json()
    return id
  }

  async function setStatus(id: string, status: TaskStatus): Promise<void> {
    const res = await fetch(`/api/tasks/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status }),
    })
    if (!res.ok) throw new Error(`PATCH /api/tasks/${id} → ${res.status}`)
  }

  return { tasks, loading, error, load, create, setStatus }
}
