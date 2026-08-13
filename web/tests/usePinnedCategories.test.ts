import { beforeEach, describe, expect, it } from 'vitest'
import { usePinnedCategories } from '../composables/usePinnedCategories'

describe('usePinnedCategories', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('restores a pinned category after the composable is re-created', () => {
    usePinnedCategories().remember(['project'])

    expect(usePinnedCategories().recall()).toEqual(['project'])
  })

  it('removes a category when its updated pinned set omits it', () => {
    const categories = usePinnedCategories()
    categories.remember(['project', 'task'])
    categories.remember(['task'])

    expect(categories.recall()).toEqual(['task'])
  })
})
