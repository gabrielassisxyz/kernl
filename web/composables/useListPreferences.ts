import { ref } from 'vue'

export type ListSortField = 'updated' | 'created' | 'name'
export type ListSortDir = 'asc' | 'desc'

export const LIST_SORT_OPTIONS: { id: ListSortField; label: string; hint: string }[] = [
  { id: 'updated', label: 'Last updated', hint: 'updated_at' },
  { id: 'created', label: 'Date created', hint: 'created_at' },
  { id: 'name', label: 'Name', hint: 'A–Z' },
]

const SORT_LABELS: Record<ListSortField, string> = {
  updated: 'updated_at',
  created: 'created_at',
  name: 'name',
}

interface StoredPreferences {
  collapsed?: Record<string, boolean>
  sortField?: ListSortField
  sortDir?: ListSortDir
}

const isSortField = (value: unknown): value is ListSortField =>
  value === 'updated' || value === 'created' || value === 'name'

const isSortDir = (value: unknown): value is ListSortDir => value === 'asc' || value === 'desc'

/** Keeps list-only UI state local to one screen instead of making it URL state. */
export function useListPreferences(storageKey: string, initiallyCollapsed: Record<string, boolean> = {}) {
  const stored = load(storageKey)
  const collapsed = ref<Record<string, boolean>>(stored.collapsed ?? { ...initiallyCollapsed })
  const sortField = ref<ListSortField>(stored.sortField ?? 'updated')
  const sortDir = ref<ListSortDir>(stored.sortDir ?? 'desc')

  const persist = (): void => {
    if (typeof window === 'undefined') return
    try {
      window.localStorage.setItem(storageKey, JSON.stringify({
        collapsed: collapsed.value,
        sortField: sortField.value,
        sortDir: sortDir.value,
      }))
    } catch {
      // Ignore quota and availability errors; the list remains usable.
    }
  }

  const toggleSection = (id: string): void => {
    collapsed.value = { ...collapsed.value, [id]: !collapsed.value[id] }
    persist()
  }
  const setSortField = (field: ListSortField): void => {
    sortField.value = field
    persist()
  }
  const toggleSortDir = (): void => {
    sortDir.value = sortDir.value === 'desc' ? 'asc' : 'desc'
    persist()
  }

  return {
    collapsed,
    sortField,
    sortDir,
    sortLabel: () => SORT_LABELS[sortField.value],
    toggleSection,
    setSortField,
    toggleSortDir,
  }
}

function load(storageKey: string): StoredPreferences {
  if (typeof window === 'undefined') return {}
  try {
    const parsed = JSON.parse(window.localStorage.getItem(storageKey) || '{}') as StoredPreferences
    return {
      collapsed: parsed.collapsed && typeof parsed.collapsed === 'object' ? parsed.collapsed : undefined,
      sortField: isSortField(parsed.sortField) ? parsed.sortField : undefined,
      sortDir: isSortDir(parsed.sortDir) ? parsed.sortDir : undefined,
    }
  } catch {
    return {}
  }
}
