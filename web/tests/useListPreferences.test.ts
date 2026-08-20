import { beforeEach, describe, expect, it } from 'vitest'
import { useListPreferences } from '../composables/useListPreferences'

describe('useListPreferences', () => {
  beforeEach(() => window.localStorage.clear())

  it('seeds the Projects lifecycle defaults without collapsing pinned or active sections', () => {
    const preferences = useListPreferences('kernl:projects-list-preferences', {
      paused: true,
      done: true,
      archived: true,
    })

    expect(preferences.collapsed.value).toEqual({ paused: true, done: true, archived: true })
  })

  it('keeps a stored Projects section preference instead of reseeding its defaults', () => {
    window.localStorage.setItem('kernl:projects-list-preferences', JSON.stringify({
      collapsed: { paused: false, done: false, archived: false },
    }))

    const preferences = useListPreferences('kernl:projects-list-preferences', {
      paused: true,
      done: true,
      archived: true,
    })

    expect(preferences.collapsed.value).toEqual({ paused: false, done: false, archived: false })
  })

  it('restores collapsed sections and sorting after a reload', () => {
    const first = useListPreferences('kernl:test-list', { done: true })
    first.toggleSection('active')
    first.setSortField('name')
    first.toggleSortDir()

    const reloaded = useListPreferences('kernl:test-list', { done: true })
    expect(reloaded.collapsed.value).toEqual({ done: true, active: true })
    expect(reloaded.sortField.value).toBe('name')
    expect(reloaded.sortDir.value).toBe('asc')
  })

  it('falls back to defaults when storage is malformed', () => {
    window.localStorage.setItem('kernl:test-list', '{')
    const preferences = useListPreferences('kernl:test-list', { done: true })
    expect(preferences.collapsed.value).toEqual({ done: true })
    expect(preferences.sortField.value).toBe('updated')
    expect(preferences.sortDir.value).toBe('desc')
  })
})
