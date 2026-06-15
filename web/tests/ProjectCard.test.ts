import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProjectCard from '../components/projects/ProjectCard.vue'

function epic(p: Record<string, unknown> = {}) {
  return {
    id: 'e1',
    type: 'epic',
    state: 'ready_for_implementation',
    title: 'My Epic',
    priority: 1,
    labels: [],
    createdAt: '',
    updatedAt: '',
    ...p,
  }
}

describe('ProjectCard', () => {
  it('shows an em dash and an empty bar when there are no children', () => {
    const w = mount(ProjectCard, { props: { epic: epic(), done: 0, total: 0 } })
    expect(w.text()).toContain('0/0')
    expect(w.text()).toContain('—')
    expect(w.html()).toContain('width: 0%')
  })

  it('shows 100% with the passed tone when complete', () => {
    const w = mount(ProjectCard, { props: { epic: epic(), done: 3, total: 3 } })
    expect(w.text()).toContain('3/3')
    expect(w.text()).toContain('100%')
    expect(w.html()).toContain('bg-status-passed')
  })

  it('rounds partial progress', () => {
    const w = mount(ProjectCard, { props: { epic: epic(), done: 1, total: 3 } })
    expect(w.text()).toContain('33%')
  })

  it('renders the amber gate styling when the epic needs a human', () => {
    const w = mount(ProjectCard, {
      props: { epic: epic({ requiresHumanAction: true }), done: 0, total: 2 },
    })
    expect(w.html()).toContain('border-status-gate')
  })

  it('emits open on click', async () => {
    const w = mount(ProjectCard, { props: { epic: epic(), done: 0, total: 1 } })
    await w.trigger('click')
    expect(w.emitted('open')).toHaveLength(1)
  })
})
