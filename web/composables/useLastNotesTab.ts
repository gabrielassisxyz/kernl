import type { VaultTab } from '~/composables/useVaultFilters'

// Which of the Notes screen's two lists was in front, per browser. Like the
// open note next to it, this is a view preference and not addressable state -
// nobody sends someone a link to "the Tags tab" - so it stays out of the url.
const STORAGE_KEY = 'kernl:notes-last-tab'

const TABS: readonly string[] = ['tags', 'files']

/**
 * Remembers which tab the Notes screen was on across reloads and across visits.
 *
 * ```ts
 * const { remember, recall } = useLastNotesTab()
 * remember('files')
 * recall() // → 'files', or null when nothing usable is stored
 * ```
 */
export function useLastNotesTab() {
  function remember(tab: VaultTab): void {
    if (typeof window === 'undefined') return
    try {
      window.localStorage.setItem(STORAGE_KEY, tab)
    } catch {
      // Ignore quota/availability errors - the screen just won't reopen on it.
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
   * The remembered tab, but only if it is still one this screen has. A renamed
   * tab, or a value edited by hand into the store, would otherwise select
   * nothing and leave the panel blank with no tab lit - so anything the screen
   * does not recognise is dropped and the caller keeps its own default.
   */
  function recall(): VaultTab | null {
    if (typeof window === 'undefined') return null
    let stored = ''
    try {
      stored = window.localStorage.getItem(STORAGE_KEY) || ''
    } catch {
      return null
    }
    if (!stored) return null
    if (!TABS.includes(stored)) {
      forget()
      return null
    }
    return stored as VaultTab
  }

  return { remember, recall, forget }
}
