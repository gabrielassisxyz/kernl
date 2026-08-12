import { ref, computed, type Ref } from 'vue'
import { isAgentAuthored, type VaultNote } from '~/composables/useVaultIndex'
import { labelForType } from '~/utils/nodeTypes'

export type VaultTab = 'tags' | 'files'
export type SortField = 'updated' | 'created' | 'name'
export type SortDir = 'asc' | 'desc'
/** merged: one list · split: grouped by who wrote it · me/da: only one of them. */
export type SourceMode = 'merged' | 'split' | 'me' | 'da'

export const SORT_OPTIONS: { id: SortField; label: string; hint: string }[] = [
  { id: 'updated', label: 'Last updated', hint: 'updated_at' },
  { id: 'created', label: 'Date created', hint: 'created_at' },
  { id: 'name', label: 'Name', hint: 'A–Z' },
]

export const SOURCE_OPTIONS: { id: SourceMode; label: string; hint: string }[] = [
  { id: 'merged', label: 'Merged', hint: 'one list' },
  { id: 'split', label: 'Split me / DA', hint: 'two groups' },
  { id: 'me', label: 'Only mine', hint: 'me' },
  { id: 'da', label: 'Only DA', hint: 'DA' },
]

const SORT_LABELS: Record<SortField, string> = {
  updated: 'updated_at',
  created: 'created_at',
  name: 'name',
}

const SOURCE_LABELS: Record<SourceMode, string> = {
  merged: 'merged',
  split: 'split',
  me: 'me only',
  da: 'DA only',
}

export interface CategoryOption {
  id: string
  label: string
  count: number
}

export interface NoteGroup {
  key: string
  label: string
  /** How much the group holds, which a collapsed group still reports. */
  count: number
  notes: VaultNote[]
}

export interface TagEntry {
  name: string
  count: number
  pinned: boolean
  notes: VaultNote[]
}

export interface TagGroup {
  key: string
  label: string
  count: number
  tags: TagEntry[]
}

const capitalize = (s: string): string => (s ? s[0].toUpperCase() + s.slice(1) : s)

/**
 * Categories the vault holds by the hundred: grouped by category they bury
 * everything else under a wall of rows, so they start folded. It is a seed, run
 * once when the panel is set up and never re-applied, so a group the user opens
 * stays open while they search, filter or re-sort.
 */
const FOLDED_CATEGORIES = ['bookmark', 'project', 'task']

// Split mode encodes the author into the group key, so the same category
// arrives under a second name and needs the same seed.
const seedFoldedGroups = (): Record<string, boolean> =>
  Object.fromEntries(
    FOLDED_CATEGORIES.flatMap((c) => [[c, true], [`${c}:me`, true], [`${c}:da`, true]]),
  )

/**
 * Everything the vault panel derives from the index: the search, the three
 * filter chips, and the grouping each tab shows. It lives beside the panel
 * rather than inside it because both tabs read the same filtered pool, and a
 * note that a filter removed has to disappear from the tag tree too.
 */
export function useVaultFilters(notes: Ref<VaultNote[]>, pinnedTags: Ref<string[]>) {
  const tab = ref<VaultTab>('tags')
  const query = ref('')
  const sortField = ref<SortField>('updated')
  const sortDir = ref<SortDir>('desc')
  const sourceMode = ref<SourceMode>('merged')
  const categoryFilter = ref('all')
  const groupByCategory = ref(false)
  const expandedTags = ref<Record<string, boolean>>({})
  const collapsedGroups = ref<Record<string, boolean>>(seedFoldedGroups())

  const isTags = computed(() => tab.value === 'tags')
  const isSplit = computed(() => sourceMode.value === 'split')

  const sortLabel = computed(() => SORT_LABELS[sortField.value])
  const sourceLabel = computed(() => SOURCE_LABELS[sourceMode.value])
  const categoryLabel = computed(() => {
    if (categoryFilter.value !== 'all') return categoryFilter.value
    return groupByCategory.value ? 'grouped' : 'categories'
  })

  // Matching is title, category and tags - not the body. The body lives in
  // files the index deliberately does not carry; full-text search is the
  // separate Omnisearch surface.
  const matches = (note: VaultNote, q: string): boolean => {
    if (!q) return true
    return `${note.title} ${note.category} ${note.tags.join(' ')}`.toLowerCase().includes(q)
  }

  const pool = computed(() => {
    const q = query.value.trim().toLowerCase()
    let out = notes.value.filter((n) => matches(n, q))
    if (sourceMode.value === 'me') out = out.filter((n) => !isAgentAuthored(n))
    if (sourceMode.value === 'da') out = out.filter((n) => isAgentAuthored(n))
    // The category chip only exists on the Files tab, so it must not narrow
    // the tag tree behind the user's back.
    if (!isTags.value && categoryFilter.value !== 'all') {
      out = out.filter((n) => n.category === categoryFilter.value)
    }
    return out
  })

  const sorted = (list: VaultNote[]): VaultNote[] => {
    const dir = sortDir.value === 'asc' ? 1 : -1
    const field = sortField.value
    return [...list].sort((a, b) => {
      if (field === 'name') return a.title.localeCompare(b.title) * dir
      const key = field === 'created' ? 'createdAt' : 'updatedAt'
      return (a[key] < b[key] ? -1 : a[key] > b[key] ? 1 : 0) * dir
    })
  }

  // Counts come from the whole vault, not the filtered pool: a menu that
  // renumbers itself as you filter cannot tell you what filtering would do.
  const categories = computed<CategoryOption[]>(() => {
    const counts = new Map<string, number>()
    for (const note of notes.value) {
      counts.set(note.category, (counts.get(note.category) || 0) + 1)
    }
    const present = [...counts.entries()]
      .map(([id, count]) => ({ id, label: capitalize(labelForType(id)), count }))
      .sort((a, b) => a.label.localeCompare(b.label))
    return [{ id: 'all', label: 'All categories', count: notes.value.length }, ...present]
  })

  const isTagOpen = (name: string): boolean =>
    query.value.trim() ? true : !!expandedTags.value[name]

  const toggleTag = (name: string): void => {
    expandedTags.value = { ...expandedTags.value, [name]: !expandedTags.value[name] }
  }

  const isGroupOpen = (key: string): boolean => !collapsedGroups.value[key]

  const toggleGroup = (key: string): void => {
    collapsedGroups.value = { ...collapsedGroups.value, [key]: !collapsedGroups.value[key] }
  }

  const tagGroups = computed<TagGroup[]>(() => {
    const byTag = new Map<string, VaultNote[]>()
    for (const note of pool.value) {
      for (const tag of note.tags) {
        const bucket = byTag.get(tag)
        if (bucket) bucket.push(note)
        else byTag.set(tag, [note])
      }
    }

    const build = (name: string): TagEntry => {
      const held = byTag.get(name) || []
      return {
        name,
        count: held.length,
        pinned: pinnedTags.value.includes(name),
        // Children are only materialised for an open tag: the vault has ~200
        // tags and sorting every one of their note lists on each keystroke is
        // work the user cannot see.
        notes: isTagOpen(name) ? sorted(held) : [],
      }
    }

    const names = [...byTag.keys()].sort((a, b) => a.localeCompare(b))
    const pinned = names.filter((n) => pinnedTags.value.includes(n))
    const rest = names.filter((n) => !pinnedTags.value.includes(n))

    const groups: TagGroup[] = []
    if (pinned.length) {
      groups.push({ key: 'pinned', label: 'Pinned', count: pinned.length, tags: pinned.map(build) })
    }
    if (rest.length) {
      groups.push({ key: 'all', label: 'All tags', count: rest.length, tags: rest.map(build) })
    }
    return groups
  })

  const fileGroups = computed<NoteGroup[]>(() => {
    const groups: NoteGroup[] = []
    const push = (key: string, label: string, list: VaultNote[]): void => {
      if (!list.length) return
      groups.push({ key, label, count: list.length, notes: isGroupOpen(key) ? sorted(list) : [] })
    }

    const pinned = pool.value.filter((n) => n.pinned)
    const rest = pool.value.filter((n) => !n.pinned)
    push('pinned', 'Pinned', pinned)

    if (groupByCategory.value) {
      for (const category of categories.value) {
        if (category.id === 'all') continue
        const inCategory = rest.filter((n) => n.category === category.id)
        if (isSplit.value) {
          push(`${category.id}:me`, `${category.label} · me`, inCategory.filter((n) => !isAgentAuthored(n)))
          push(`${category.id}:da`, `${category.label} · DA`, inCategory.filter((n) => isAgentAuthored(n)))
        } else {
          push(category.id, category.label, inCategory)
        }
      }
    } else if (isSplit.value) {
      push('me', 'Written by me', rest.filter((n) => !isAgentAuthored(n)))
      push('da', 'Written by DA', rest.filter((n) => isAgentAuthored(n)))
    } else {
      push('all', 'All notes', rest)
    }
    return groups
  })

  const counter = computed(() => `${pool.value.length} / ${notes.value.length} notes`)

  return {
    tab,
    query,
    sortField,
    sortDir,
    sourceMode,
    categoryFilter,
    groupByCategory,
    isTags,
    sortLabel,
    sourceLabel,
    categoryLabel,
    categories,
    pool,
    tagGroups,
    fileGroups,
    counter,
    isTagOpen,
    toggleTag,
    isGroupOpen,
    toggleGroup,
  }
}
