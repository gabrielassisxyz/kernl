import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TagTree from '../components/tags/TagTree.vue'
import type { TagNode } from '../composables/useTags'

function node(name: string, segment: string, count: number, byType: Record<string, number>, children: TagNode[] = []): TagNode {
  return { name, segment, count, byType, children }
}

// homelab (2 notes, 1 task) > nas (1 note) ; reading (1 task)
const TREE: TagNode[] = [
  node('homelab', 'homelab', 3, { note: 2, task: 1 }, [
    node('homelab/nas', 'nas', 1, { note: 1 }),
  ]),
  node('reading', 'reading', 1, { task: 1 }),
]

describe('TagTree', () => {
  it('shows roots collapsed, with subtree-inclusive counts', () => {
    const w = mount(TagTree, { props: { tree: TREE } })
    expect(w.text()).toContain('homelab')
    expect(w.text()).toContain('3')
    expect(w.text()).not.toContain('nas')
  })

  it('reveals a child under its parent segment when expanded', async () => {
    const w = mount(TagTree, { props: { tree: TREE } })
    await w.get('[aria-label="Expand homelab"]').trigger('click')
    expect(w.text()).toContain('nas')
  })

  it('hides branches with nothing of the filtered type', () => {
    const w = mount(TagTree, { props: { tree: TREE, type: 'note' } })
    expect(w.text()).toContain('homelab')
    // reading holds a task only, so a note-only pane has no business showing it.
    expect(w.text()).not.toContain('reading')
  })

  it('counts only the filtered type', () => {
    const w = mount(TagTree, { props: { tree: TREE, type: 'task' } })
    expect(w.text()).toContain('1')
    expect(w.text()).not.toContain('3')
  })

  it('emits the full tag name, not the segment', async () => {
    const w = mount(TagTree, { props: { tree: TREE } })
    await w.get('[aria-label="Expand homelab"]').trigger('click')
    const buttons = w.findAll('button').filter((b) => b.text().includes('nas'))
    await buttons[0].trigger('click')
    expect(w.emitted('select')?.at(-1)).toEqual(['homelab/nas'])
  })

  it('says so when there is nothing tagged', () => {
    const w = mount(TagTree, { props: { tree: [], emptyText: 'No tags yet.' } })
    expect(w.text()).toContain('No tags yet.')
  })
})
