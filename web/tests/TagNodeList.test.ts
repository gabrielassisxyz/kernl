import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TagNodeList from '../components/tags/TagNodeList.vue'
import type { TaggedNode } from '../composables/useTags'

const NODES: TaggedNode[] = [
  { id: 'n1', title: 'NAS rebuild', type: 'note', updatedAt: '2026-07-11T10:00:00Z', path: 'homelab/nas.md' },
  { id: 't1', title: 'Replace the failing disk', type: 'task', updatedAt: '2026-07-11T09:00:00Z' },
  { id: 'b1', title: 'ZFS tuning guide', type: 'bookmark', updatedAt: '2026-07-10T09:00:00Z' },
]

let requested: string[] = []

beforeEach(() => {
  requested = []
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    requested.push(url)
    return { ok: true, json: async () => NODES } as Response
  }))
})

afterEach(() => vi.unstubAllGlobals())

describe('TagNodeList', () => {
  it('lists every type under the tag, not just one', async () => {
    const w = mount(TagNodeList, { props: { tag: 'homelab' } })
    await flushPromises()
    expect(w.text()).toContain('NAS rebuild')
    expect(w.text()).toContain('Replace the failing disk')
    expect(w.text()).toContain('ZFS tuning guide')
  })

  it('passes the tag as a query parameter, since names contain slashes', async () => {
    mount(TagNodeList, { props: { tag: 'homelab/nas', type: 'note' } })
    await flushPromises()
    expect(requested[0]).toBe('/api/tags/nodes?tag=homelab%2Fnas&type=note')
  })

  it('opens the types that have a surface, and leaves the rest inert', async () => {
    const w = mount(TagNodeList, { props: { tag: 'homelab' } })
    await flushPromises()

    const rows = w.findAll('button')
    expect(rows).toHaveLength(2) // the note and the task; the bookmark has nowhere to go

    await rows[0].trigger('click')
    expect(w.emitted('open')?.at(-1)).toEqual([NODES[0]])
  })

  it('refetches when the tag changes', async () => {
    const w = mount(TagNodeList, { props: { tag: 'homelab' } })
    await flushPromises()
    await w.setProps({ tag: 'reading' })
    await flushPromises()
    expect(requested).toHaveLength(2)
    expect(requested[1]).toContain('tag=reading')
  })
})
