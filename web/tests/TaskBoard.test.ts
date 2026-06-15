import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TaskBoard from '../components/tasks/TaskBoard.vue'
import TaskCard from '../components/tasks/TaskCard.vue'
import type { Task, TaskStatus } from '../composables/useTasks'

function task(id: string, status: TaskStatus, projectId = ''): Task {
  return {
    id,
    title: id,
    description: '',
    status,
    projectId,
    createdAt: '',
    updatedAt: '',
  }
}

describe('TaskBoard', () => {
  const tasks = [
    task('a', 'todo'),
    task('b', 'todo'),
    task('c', 'in_progress'),
    task('d', 'done'),
  ]
  const projectTitles = {}

  it('renders one card per task', () => {
    const w = mount(TaskBoard, { props: { tasks, projectTitles } })
    expect(w.findAllComponents(TaskCard)).toHaveLength(4)
  })

  it('buckets tasks into the three status columns', () => {
    const w = mount(TaskBoard, { props: { tasks, projectTitles } })
    const sections = w.findAll('section')
    expect(sections).toHaveLength(3)
    // Columns render in TASK_STATUSES order: To do, In progress, Done.
    const counts = sections.map((s) => s.get('.font-mono-data').text())
    expect(counts).toEqual(['2', '1', '1'])
  })

  it('shows an em dash placeholder for an empty column', () => {
    const w = mount(TaskBoard, { props: { tasks: [task('only', 'todo')], projectTitles } })
    const dashes = w.findAll('section').filter((s) => s.text().includes('—'))
    expect(dashes.length).toBe(2)
  })

  it('emits open with the task when a card is clicked', async () => {
    const w = mount(TaskBoard, { props: { tasks, projectTitles } })
    await w.findComponent(TaskCard).trigger('click')
    expect(w.emitted('open')).toBeTruthy()
  })
})
