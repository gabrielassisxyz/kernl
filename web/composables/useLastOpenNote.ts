// Which note the Notes screen had open, per browser. It is a view preference,
// not addressable state: a vault path is not something anyone types or reads
// out, so it is kept here rather than in the url, which stays clean.
const STORAGE_KEY = 'kernl:notes-last-open'

/**
 * Remembers the open note across reloads and across visits to the screen.
 *
 * ```ts
 * const { remember, recall, forget } = useLastOpenNote()
 * remember('plans/scratchpad.md')
 * recall(notes.value) // → 'plans/scratchpad.md', or null if the vault lost it
 * ```
 */
export function useLastOpenNote() {
  function read(): string {
    if (typeof window === 'undefined') return ''
    try {
      return window.localStorage.getItem(STORAGE_KEY) || ''
    } catch {
      return ''
    }
  }

  function remember(path: string): void {
    if (typeof window === 'undefined' || !path) return
    try {
      window.localStorage.setItem(STORAGE_KEY, path)
    } catch {
      // Ignore quota/availability errors - the screen just won't reopen it.
    }
  }

  function forget(): void {
    if (typeof window === 'undefined') return
    try {
      window.localStorage.removeItem(STORAGE_KEY)
    } catch {
      // Same: nothing to do if the store refuses us.
    }
  }

  /**
   * The remembered note, but only if the vault still holds it. A note deleted
   * or renamed outside the app would otherwise be reopened as a file the editor
   * cannot load, which shows an empty document and explains nothing - so a
   * pointer the vault no longer recognises is dropped instead of returned.
   */
  function recall(notes: readonly { path: string }[]): string | null {
    const stored = read()
    if (!stored) return null
    if (!notes.some((note) => note.path === stored)) {
      forget()
      return null
    }
    return stored
  }

  return { remember, recall, forget }
}
