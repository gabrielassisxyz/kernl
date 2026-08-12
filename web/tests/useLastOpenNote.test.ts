import { beforeEach, describe, expect, it } from 'vitest'
import { useLastOpenNote } from '../composables/useLastOpenNote'

const vault = (...paths: string[]) => paths.map((path) => ({ path }))

describe('useLastOpenNote', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('reopens the note the screen had open', () => {
    const { remember, recall } = useLastOpenNote()

    remember('plans/scratchpad.md')

    expect(recall(vault('plans/scratchpad.md', 'other.md'))).toBe('plans/scratchpad.md')
  })

  it('survives being re-created, which is the whole point', () => {
    useLastOpenNote().remember('plans/scratchpad.md')

    expect(useLastOpenNote().recall(vault('plans/scratchpad.md'))).toBe('plans/scratchpad.md')
  })

  it('drops a note the vault no longer holds', () => {
    const { remember, recall } = useLastOpenNote()
    remember('deleted-outside.md')

    // Renamed or deleted behind the app's back: reopening it would show an
    // empty document the editor cannot explain.
    expect(recall(vault('still-here.md'))).toBeNull()
    // And it is gone for good, not re-checked on every visit.
    expect(recall(vault('deleted-outside.md'))).toBeNull()
  })

  it('has nothing to say before a note was ever opened', () => {
    expect(useLastOpenNote().recall(vault('anything.md'))).toBeNull()
  })

  it('forgets on request, for the note that was just deleted', () => {
    const { remember, recall, forget } = useLastOpenNote()
    remember('doomed.md')

    forget()

    expect(recall(vault('doomed.md'))).toBeNull()
  })
})
