import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { useVaultFilters } from '../composables/useVaultFilters'
import type { VaultNote } from '../composables/useVaultIndex'

function note(p: Partial<VaultNote> = {}): VaultNote {
  return {
    id: p.title ?? 'id',
    path: `${p.title ?? 'id'}.md`,
    title: 'Untitled',
    category: 'note',
    tags: [],
    author: 'human',
    pinned: false,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...p,
  }
}

const DA = 'agent:da'

function setup(notes: VaultNote[], pinnedTags: string[] = []) {
  return useVaultFilters(ref(notes), ref(pinnedTags))
}

const labelsOf = (groups: { label: string }[]) => groups.map((g) => g.label)
const titlesOf = (notes: VaultNote[]) => notes.map((n) => n.title)

describe('search', () => {
  const notes = [
    note({ title: 'Homelab migration', tags: ['ops'] }),
    note({ title: 'Reading list', tags: ['learning'] }),
    note({ title: 'A project', category: 'project' }),
  ]

  it('matches title, tags and category, case-insensitively', () => {
    const f = setup(notes)
    f.tab.value = 'files'

    f.query.value = 'HOMELAB'
    expect(titlesOf(f.pool.value)).toEqual(['Homelab migration'])

    f.query.value = 'learning'
    expect(titlesOf(f.pool.value)).toEqual(['Reading list'])

    f.query.value = 'project'
    expect(titlesOf(f.pool.value)).toEqual(['A project'])
  })

  it('force-opens every tag while a query is present', () => {
    const f = setup([note({ title: 'a', tags: ['ops'] })])
    expect(f.isTagOpen('ops')).toBe(false)
    f.query.value = 'a'
    expect(f.isTagOpen('ops')).toBe(true)
    // …and hands control back once the query is cleared.
    f.query.value = ''
    expect(f.isTagOpen('ops')).toBe(false)
  })

  it('counts what is shown against what the vault holds', () => {
    const f = setup(notes)
    expect(f.counter.value).toBe('3 / 3 notes')
    f.query.value = 'homelab'
    expect(f.counter.value).toBe('1 / 3 notes')
  })
})

describe('authorship filter', () => {
  const notes = [
    note({ title: 'mine' }),
    note({ title: 'named human', author: 'user' }),
    note({ title: 'the DA’s', author: DA }),
  ]

  it('treats only agent-prefixed authors as not mine', () => {
    const f = setup(notes)
    f.tab.value = 'files'

    f.sourceMode.value = 'me'
    expect(titlesOf(f.pool.value).sort()).toEqual(['mine', 'named human'])

    f.sourceMode.value = 'da'
    expect(titlesOf(f.pool.value)).toEqual(['the DA’s'])
  })

  it('splits into two groups without dropping anyone', () => {
    const f = setup(notes)
    f.tab.value = 'files'
    f.sourceMode.value = 'split'
    expect(labelsOf(f.fileGroups.value)).toEqual(['Written by me', 'Written by DA'])
    expect(f.fileGroups.value.reduce((n, g) => n + g.count, 0)).toBe(3)
  })
})

describe('categories', () => {
  const notes = [
    note({ title: 'p', category: 'project' }),
    note({ title: 't', category: 'task' }),
    note({ title: 'n' }),
  ]

  it('lists only the categories the vault holds, plus All', () => {
    const f = setup(notes)
    expect(f.categories.value.map((c) => c.label)).toEqual([
      'All categories', 'Note', 'Project', 'Task',
    ])
    expect(f.categories.value.find((c) => c.id === 'all')?.count).toBe(3)
  })

  it('keeps its counts against the whole vault while a filter narrows the list', () => {
    const f = setup(notes)
    f.tab.value = 'files'
    f.categoryFilter.value = 'project'
    expect(titlesOf(f.pool.value)).toEqual(['p'])
    expect(f.categories.value.find((c) => c.id === 'task')?.count).toBe(1)
  })

  it('never narrows the Tags tab, which has no category chip to undo it', () => {
    const f = setup(notes)
    f.categoryFilter.value = 'project'
    f.tab.value = 'tags'
    expect(f.pool.value).toHaveLength(3)
  })

  it('crosses grouping with the split when both are on', () => {
    const f = setup([
      note({ title: 'p-me', category: 'project' }),
      note({ title: 'p-da', category: 'project', author: DA }),
      note({ title: 'n-me' }),
    ])
    f.tab.value = 'files'
    f.groupByCategory.value = true
    f.sourceMode.value = 'split'
    // Empty crossings are dropped rather than rendered as empty headings.
    expect(labelsOf(f.fileGroups.value)).toEqual(['Note · me', 'Project · me', 'Project · DA'])
  })
})

describe('pinning', () => {
  it('lifts pinned notes into their own group', () => {
    const f = setup([note({ title: 'kept', pinned: true }), note({ title: 'ordinary' })])
    f.tab.value = 'files'
    expect(labelsOf(f.fileGroups.value)).toEqual(['Pinned', 'All notes'])
    expect(titlesOf(f.fileGroups.value[0].notes)).toEqual(['kept'])
  })

  it('lifts pinned tags into their own section', () => {
    const f = setup([note({ title: 'a', tags: ['kernl', 'misc'] })], ['kernl'])
    expect(labelsOf(f.tagGroups.value)).toEqual(['Pinned', 'All tags'])
    expect(f.tagGroups.value[0].tags.map((t) => t.name)).toEqual(['kernl'])
  })

  it('shows one unlabelled list when nothing is pinned', () => {
    const f = setup([note({ title: 'a', tags: ['kernl'] })])
    expect(labelsOf(f.tagGroups.value)).toEqual(['All tags'])
  })
})

describe('sorting', () => {
  const notes = [
    note({ title: 'B', updatedAt: '2026-03-01T00:00:00Z', createdAt: '2026-01-03T00:00:00Z' }),
    note({ title: 'A', updatedAt: '2026-02-01T00:00:00Z', createdAt: '2026-01-01T00:00:00Z' }),
    note({ title: 'C', updatedAt: '2026-01-01T00:00:00Z', createdAt: '2026-01-02T00:00:00Z' }),
  ]

  it('orders by the chosen field and direction', () => {
    const f = setup(notes)
    f.tab.value = 'files'

    expect(titlesOf(f.fileGroups.value[0].notes)).toEqual(['B', 'A', 'C'])

    f.sortDir.value = 'asc'
    expect(titlesOf(f.fileGroups.value[0].notes)).toEqual(['C', 'A', 'B'])

    f.sortField.value = 'name'
    expect(titlesOf(f.fileGroups.value[0].notes)).toEqual(['A', 'B', 'C'])

    f.sortField.value = 'created'
    f.sortDir.value = 'desc'
    expect(titlesOf(f.fileGroups.value[0].notes)).toEqual(['B', 'C', 'A'])
  })
})

describe('collapsing', () => {
  it('empties a collapsed group but keeps reporting its size', () => {
    const f = setup([note({ title: 'a' }), note({ title: 'b' })])
    f.tab.value = 'files'
    f.toggleGroup('all')
    expect(f.isGroupOpen('all')).toBe(false)
    expect(f.fileGroups.value[0].notes).toEqual([])
    expect(f.fileGroups.value[0].count).toBe(2)
  })

  const grouped = () => {
    const f = setup([
      note({ title: 'a note' }),
      note({ title: 'a bookmark', category: 'bookmark' }),
      note({ title: 'a project', category: 'project' }),
      note({ title: 'a task', category: 'task' }),
    ])
    f.tab.value = 'files'
    f.groupByCategory.value = true
    return f
  }

  it('starts the bulk categories folded and every other one open', () => {
    const f = grouped()
    expect(f.isGroupOpen('bookmark')).toBe(false)
    expect(f.isGroupOpen('project')).toBe(false)
    expect(f.isGroupOpen('task')).toBe(false)
    expect(f.isGroupOpen('note')).toBe(true)

    const folded = f.fileGroups.value.find((g) => g.key === 'task')
    expect(folded?.notes).toEqual([])
    expect(folded?.count).toBe(1)
    expect(titlesOf(f.fileGroups.value.find((g) => g.key === 'note')!.notes)).toEqual(['a note'])
  })

  it('keeps a folded category open once the user opens it, through search and sort', () => {
    const f = grouped()
    f.toggleGroup('task')
    expect(f.isGroupOpen('task')).toBe(true)

    f.query.value = 'a'
    f.sortField.value = 'name'
    f.sortDir.value = 'asc'
    expect(f.isGroupOpen('task')).toBe(true)
    expect(titlesOf(f.fileGroups.value.find((g) => g.key === 'task')!.notes)).toEqual(['a task'])
  })

  it('folds a bulk category under split mode too, where the key carries the author', () => {
    const f = grouped()
    f.sourceMode.value = 'split'
    expect(f.isGroupOpen('task:me')).toBe(false)
    expect(f.isGroupOpen('note:me')).toBe(true)
  })

  it('materialises a tag\'s children only once it is open', () => {
    const f = setup([note({ title: 'a', tags: ['ops'] })])
    expect(f.tagGroups.value[0].tags[0].notes).toEqual([])
    expect(f.tagGroups.value[0].tags[0].count).toBe(1)
    f.toggleTag('ops')
    expect(titlesOf(f.tagGroups.value[0].tags[0].notes)).toEqual(['a'])
  })
})
