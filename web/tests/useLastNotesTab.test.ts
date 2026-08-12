import { beforeEach, describe, expect, it } from 'vitest'
import { useLastNotesTab } from '../composables/useLastNotesTab'

const STORAGE_KEY = 'kernl:notes-last-tab'

describe('useLastNotesTab', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('reopens the tab the screen was left on', () => {
    const { remember, recall } = useLastNotesTab()

    remember('files')

    expect(recall()).toBe('files')
  })

  it('survives being re-created, which is the whole point', () => {
    useLastNotesTab().remember('files')

    expect(useLastNotesTab().recall()).toBe('files')
  })

  it('has nothing to say before a tab was ever chosen', () => {
    expect(useLastNotesTab().recall()).toBeNull()
  })

  it('drops a stored value the screen has no tab for', () => {
    // Hand-edited, or left behind by a tab that no longer exists: selecting it
    // would light no tab at all and show an empty panel.
    window.localStorage.setItem(STORAGE_KEY, 'garbage')

    expect(useLastNotesTab().recall()).toBeNull()
    // And it is gone, rather than re-read and re-rejected on every visit.
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull()
  })

  it('forgets on request', () => {
    const { remember, recall, forget } = useLastNotesTab()
    remember('files')

    forget()

    expect(recall()).toBeNull()
  })
})
