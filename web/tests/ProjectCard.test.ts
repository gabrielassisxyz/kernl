import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProjectCard from '../components/projects/ProjectCard.vue'
import type { Project } from '../composables/useProjects'

function project(p: Partial<Project> = {}): Project {
  return {
    id: 'p1',
    title: 'My Project',
    description: '',
    status: 'active',
    createdAt: '',
    updatedAt: '',
    taskCount: 0,
    doneCount: 0,
    ...p,
  }
}

describe('ProjectCard', () => {
  it('shows an em dash and an empty bar when there are no tasks', () => {
    const w = mount(ProjectCard, { props: { project: project() } })
    expect(w.text()).toContain('0/0 tasks')
    expect(w.text()).toContain('—')
    expect(w.html()).toContain('width: 0%')
  })

  it('shows 100% with the passed tone when complete', () => {
    const w = mount(ProjectCard, { props: { project: project({ taskCount: 3, doneCount: 3 }) } })
    expect(w.text()).toContain('3/3 tasks')
    expect(w.text()).toContain('100%')
    expect(w.html()).toContain('bg-status-passed')
  })

  it('rounds partial progress', () => {
    const w = mount(ProjectCard, { props: { project: project({ taskCount: 3, doneCount: 1 }) } })
    expect(w.text()).toContain('33%')
  })

  it('surfaces the status label', () => {
    const w = mount(ProjectCard, { props: { project: project({ status: 'paused' }) } })
    expect(w.text()).toContain('Paused')
  })

  it('emits open on click', async () => {
    const w = mount(ProjectCard, { props: { project: project() } })
    await w.trigger('click')
    expect(w.emitted('open')).toHaveLength(1)
  })
})
