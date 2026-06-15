import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useBeads } from '../composables/useBeads'

const mockFetch = vi.fn()
global.fetch = mockFetch as unknown as typeof fetch

function bead(partial: Record<string, unknown>) {
  return {
    id: 'x',
    type: 'task',
    state: 'open',
    title: 't',
    priority: 1,
    labels: [],
    createdAt: '',
    updatedAt: '',
    ...partial,
  }
}

describe('useBeads', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads beads from /api/beads', async () => {
    const data = [bead({ id: 'a' }), bead({ id: 'b' })]
    mockFetch.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(data) })

    const { beads, load, loading } = useBeads()
    expect(loading.value).toBe(false)
    await load()

    expect(mockFetch).toHaveBeenCalledWith('/api/beads')
    expect(beads.value).toEqual(data)
    expect(loading.value).toBe(false)
  })

  it('splits epics and tasks by type', async () => {
    const data = [
      bead({ id: 'e1', type: 'epic' }),
      bead({ id: 't1', type: 'task' }),
      bead({ id: 't2', type: 'bug' }),
    ]
    mockFetch.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(data) })

    const { load, epics, tasks } = useBeads()
    await load()

    expect(epics.value.map((b) => b.id)).toEqual(['e1'])
    expect(tasks.value.map((b) => b.id)).toEqual(['t1', 't2'])
  })

  it('childrenOf returns beads with the matching parentId', async () => {
    const data = [
      bead({ id: 'e1', type: 'epic' }),
      bead({ id: 'c1', parentId: 'e1' }),
      bead({ id: 'c2', parentId: 'e1' }),
      bead({ id: 'c3', parentId: 'other' }),
    ]
    mockFetch.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(data) })

    const { load, childrenOf } = useBeads()
    await load()

    expect(childrenOf('e1').map((b) => b.id)).toEqual(['c1', 'c2'])
    expect(childrenOf('none')).toEqual([])
  })

  it('records an error on a non-ok response and leaves beads empty', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 500, json: () => Promise.resolve([]) })

    const { load, error, beads } = useBeads()
    await load()

    expect(error.value).toContain('500')
    expect(beads.value).toEqual([])
  })

  it('records an error when fetch rejects', async () => {
    mockFetch.mockRejectedValueOnce(new Error('network down'))

    const { load, error } = useBeads()
    await load()

    expect(error.value).toBe('network down')
  })
})
