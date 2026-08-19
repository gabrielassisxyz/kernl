import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProjectList from '../components/projects/ProjectList.vue'
import type { Project, ProjectStatus } from '../composables/useProjects'

function project(id: string, status: ProjectStatus, pinned = false): Project {
  return {
    id,
    title: id,
    description: '',
    status,
    pinned,
    tags: [],
    createdAt: '',
    updatedAt: '',
    taskCount: 0,
    doneCount: 0,
  }
}

const global = { stubs: { NuxtLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } }

const mountList = (projects: Project[], view: 'list' | 'card' = 'list') =>
  mount(ProjectList, {
    props: { projects, view, collapsed: {}, sortField: 'updated', sortDir: 'desc' },
    global,
  })

// The heading is the section's first child; reading the whole section would
// pick up every row's text with no separators between them.
const headings = (w: ReturnType<typeof mount>) =>
  w.findAll('section > button').map((h) => h.findAll('span').slice(1).map((s) => s.text()).join(' '))

describe('ProjectList sections', () => {
  it('orders the lifecycle sections active, stalled, paused, not started, done, archived', () => {
    const w = mountList([
      project('f', 'archived'),
      project('e', 'done'),
      project('d', 'not_started'),
      project('c', 'paused'),
      project('b', 'stalled'),
      project('a', 'active'),
    ])
    expect(headings(w)).toEqual(['Active 1', 'Stalled 1', 'Paused 1', 'Not started 1', 'Done 1', 'Archived 1'])
  })

  it('puts pinned first, above every lifecycle section', () => {
    const w = mountList([project('a', 'active'), project('b', 'paused', true)])
    expect(headings(w)[0]).toBe('Pinned 1')
  })

  // Listing it twice would make the section counts add up to more than the
  // project count, and pinning exists so the project stops being wherever its
  // status put it.
  it('lists a pinned project only under Pinned', () => {
    const w = mountList([project('a', 'active', true)])
    expect(headings(w)).toEqual(['Pinned 1'])
  })

  it('drops a section with nothing in it', () => {
    const w = mountList([project('a', 'active')])
    expect(headings(w)).toEqual(['Active 1'])
  })

  it('renders rows in list view and cards in card view', () => {
    const projects = [project('a', 'active')]
    expect(mountList(projects, 'list').findAll('.project-row')).toHaveLength(1)
    expect(mountList(projects, 'card').findAll('.project-row')).toHaveLength(0)
  })

  it('collapses a section without hiding its count', async () => {
    const w = mountList([project('a', 'active')])
    await w.get('section > button').trigger('click')
    expect(w.emitted('toggle-section')?.[0]).toEqual(['active'])
    await w.setProps({ collapsed: { active: true } })
    expect(w.findAll('.project-row')).toHaveLength(0)
    expect(w.get('section > button').text()).toContain('1')
    expect(w.get('section > button').attributes('aria-expanded')).toBe('false')
  })

  it('sorts projects within their lifecycle section', () => {
    const w = mount(ProjectList, {
      props: {
        projects: [project('z', 'active'), project('a', 'active'), project('p', 'paused')],
        view: 'list', collapsed: {}, sortField: 'name', sortDir: 'asc',
      },
      global,
    })
    expect(w.findAll('.project-row').map((row) => row.get('.row-title').text())).toEqual(['a', 'z', 'p'])
  })

  it('marks only the row being deleted', () => {
    const w = mount(ProjectList, {
      props: {
        projects: [project('a', 'active'), project('b', 'active')],
        view: 'list',
        confirmId: 'b',
        collapsed: {}, sortField: 'updated', sortDir: 'desc',
      },
      global,
    })
    const rows = w.findAll('.project-row')
    expect(rows[0].text()).not.toContain('Delete?')
    expect(rows[1].text()).toContain('Delete?')
  })
})
