import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProjectCard from '../components/projects/ProjectCard.vue'
import ProjectRow from '../components/projects/ProjectRow.vue'
import type { Project } from '../composables/useProjects'

function project(p: Partial<Project> = {}): Project {
  return {
    id: 'p1',
    title: 'My Project',
    description: '',
    status: 'active',
    pinned: false,
    tags: [],
    createdAt: '',
    updatedAt: '',
    taskCount: 0,
    doneCount: 0,
    ...p,
  }
}

// NuxtLink is injected by the framework, not by the component, so it does not
// resolve under a bare vitest mount. Rendering it as an anchor keeps the href
// assertions meaningful.
const global = { stubs: { NuxtLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } }

const mountCard = (p = project(), extra = {}) =>
  mount(ProjectCard, { props: { project: p, ...extra }, global })
const mountRow = (p = project(), extra = {}) =>
  mount(ProjectRow, { props: { project: p, ...extra }, global })

// Progress is reported by both surfaces, and both have to answer the same way
// for a project nobody has broken into tasks yet.
describe.each([
  ['card', mountCard] as const,
  ['row', mountRow] as const,
])('Project %s progress', (_name, mountIt) => {
  it('reports an empty bar and no percentage when there are no tasks', () => {
    const w = mountIt()
    expect(w.text()).toContain('0/0')
    expect(w.html()).toContain('width: 0%')
    // 0/0 is not 0% done, and stating a percentage over an absent denominator
    // is the invented completion figure the brief rules out.
    expect(w.text()).not.toContain('%')
  })

  it('fills the bar completely when every task is done', () => {
    const w = mountIt(project({ taskCount: 3, doneCount: 3 }))
    expect(w.text()).toContain('3/3')
    expect(w.html()).toContain('width: 100%')
  })

  it('rounds partial progress', () => {
    const w = mountIt(project({ taskCount: 3, doneCount: 1 }))
    expect(w.html()).toContain('width: 33%')
  })

  it('links the title to that project\'s tasks', () => {
    expect(mountIt().get('a').attributes('href')).toBe('/tasks?project=p1')
  })

  it('emits toggle-pin from the pin control', async () => {
    const w = mountIt()
    const pin = w.findAll('button').find((b) => b.attributes('title') === 'Pin')!
    await pin.trigger('click')
    expect(w.emitted('toggle-pin')?.[0][0]).toMatchObject({ id: 'p1' })
  })

  it('offers to unpin a project that is already pinned', () => {
    const w = mountIt(project({ pinned: true }))
    expect(w.findAll('button').some((b) => b.attributes('title') === 'Unpin')).toBe(true)
  })

  it('asks inline rather than opening a dialog over the list', () => {
    const w = mountIt(project(), { confirming: true })
    expect(w.text()).toContain('Delete?')
  })
})

// The two surfaces give the confirmation different room. A row has a fixed
// actions cell, so the question takes it over; a card has none, so the question
// takes the place of the progress footer instead.
describe('inline delete confirmation', () => {
  it('takes over the row\'s action cell', () => {
    const w = mountRow(project(), { confirming: true })
    expect(w.findAll('button').some((b) => b.attributes('title') === 'Edit')).toBe(false)
  })

  it('takes the place of the card\'s progress footer, leaving its actions', () => {
    const w = mountCard(project({ taskCount: 4, doneCount: 1 }), { confirming: true })
    expect(w.text()).not.toContain('1/4')
    expect(w.findAll('button').some((b) => b.attributes('title') === 'Edit')).toBe(true)
  })
})

describe('ProjectRow', () => {
  it('shows the status through a dot rather than a word', () => {
    expect(mountRow(project({ status: 'paused' })).html()).toContain('bg-status-gate')
    expect(mountRow(project({ status: 'paused' })).text()).not.toContain('Paused')
  })

  it('shows the first tag and hides the cell when there is none', () => {
    expect(mountRow(project({ tags: ['design', 'ui'] })).text()).toContain('design')
    expect(mountRow().text()).not.toContain('design')
  })

  // The panel takes the right-hand column, so the row drops the metadata that
  // no longer fits rather than truncating every title to nothing.
  it('drops the metadata columns while the panel is open', () => {
    const w = mountRow(project({ taskCount: 4, doneCount: 1, tags: ['design'] }), { compact: true })
    expect(w.get('.project-row').classes()).toContain('project-row--compact')
    expect(w.text()).not.toContain('1/4')
    expect(w.text()).not.toContain('design')
  })
})
