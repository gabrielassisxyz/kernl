import { ref } from 'vue'

// Mirrors api.taskDTO (internal/api/tasks.go) - JSON is camelCase.
export interface Task {
  id: string
  title: string
  description: string
  status: TaskStatus
  projectId: string
  tags: string[]
  /** Calendar day "YYYY-MM-DD", empty when the task has no deadline. */
  dueDate: string
  createdAt: string
  updatedAt: string
}

export type TaskStatus = 'todo' | 'in_progress' | 'done' | 'closed'

export const TASK_STATUSES: { id: TaskStatus; label: string }[] = [
  { id: 'todo', label: 'To do' },
  { id: 'in_progress', label: 'In progress' },
  { id: 'done', label: 'Done' },
  { id: 'closed', label: 'Closed' },
]

/** Terminal states: work that is off the board, finished or called off. What
 *  separates them is why, which is the whole reason closed exists, so nothing
 *  here collapses the two into one status. */
export const isFinished = (status: TaskStatus) => status === 'done' || status === 'closed'

export interface TaskPatch {
  title?: string
  description?: string
  status?: TaskStatus
  projectId?: string
  tags?: string[]
  dueDate?: string
}

export interface NewTask {
  title: string
  description?: string
  status?: TaskStatus
  projectId?: string
  tags?: string[]
  dueDate?: string
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

  // One PATCH for every field, because the endpoint takes them together and a
  // per-field wrapper only multiplied the ways to spell the same request. An
  // omitted key leaves the field alone; an empty string is a real value the
  // handler distinguishes with a pointer field: `description: ""` clears the
  // text, `dueDate: ""` removes the deadline, `projectId: ""` unassigns the
  // task. A projectId naming no project is refused with 400.
  async function update(id: string, patch: TaskPatch): Promise<void> {
    const res = await fetch(`/api/tasks/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    })
    if (!res.ok) throw new Error(`PATCH /api/tasks/${id} → ${res.status}`)
  }

  // Removes the task and its companion note.
  async function remove(id: string): Promise<void> {
    const res = await fetch(`/api/tasks/${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error(`DELETE /api/tasks/${id} → ${res.status}`)
  }

  return { tasks, loading, error, load, create, update, remove }
}
