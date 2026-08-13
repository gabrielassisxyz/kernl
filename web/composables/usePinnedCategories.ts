// Category pins are a local view preference: they order an existing list, but
// do not change a note or become state shared with another browser.
const STORAGE_KEY = 'kernl:notes-pinned-categories'

export function usePinnedCategories() {
  function recall(): string[] {
    if (typeof window === 'undefined') return []
    try {
      const stored = JSON.parse(window.localStorage.getItem(STORAGE_KEY) || '[]')
      return Array.isArray(stored) && stored.every((category) => typeof category === 'string')
        ? [...new Set(stored)]
        : []
    } catch {
      return []
    }
  }

  function remember(categories: readonly string[]): void {
    if (typeof window === 'undefined') return
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify([...new Set(categories)]))
    } catch {
      // Ignore quota/availability errors - the categories simply stay unpinned.
    }
  }

  return { recall, remember }
}
