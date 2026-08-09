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

  it('buckets tasks into the status columns', () => {
    const w = mount(TaskBoard, { props: { tasks: [...tasks, task('e', 'closed')], projectTitles } })
    const sections = w.findAll('section')
    expect(sections).toHaveLength(4)
    // Columns render in TASK_STATUSES order: To do, In progress, Done, Closed.
    const counts = sections.map((s) => s.get('.font-mono-data').text())
    expect(counts).toEqual(['2', '1', '1', '1'])
  })

  it('shows an em dash placeholder for an empty column', () => {
    const w = mount(TaskBoard, { props: { tasks: [task('only', 'todo')], projectTitles } })
    const emDash = String.fromCharCode(0x2014)
    const dashes = w.findAll('section').filter((s) => s.text().includes(emDash))
    expect(dashes.length).toBe(3)
  })

  it('emits open with the task when a card is clicked', async () => {
    const w = mount(TaskBoard, { props: { tasks, projectTitles } })
    await w.findComponent(TaskCard).trigger('click')
    expect(w.emitted('open')).toBeTruthy()
  })

  // Columns share the width rather than holding a fixed one. A board that
  // scrolls sideways hides a whole column behind an edge, which is the one
  // thing a board exists to prevent.
  it('splits the width between the columns instead of scrolling sideways', () => {
    const w = mount(TaskBoard, { props: { tasks, projectTitles } })
    for (const section of w.findAll('section')) {
      expect(section.classes()).toContain('flex-1')
      expect(section.classes()).not.toContain('shrink-0')
    }
    expect(w.html()).not.toContain('overflow-x-auto')
  })

  // The column already names the status, and the card already carries the
  // strikethrough; a dot on top of both is a third telling of the same fact.
  it('states the status through the column, not through a dot', () => {
    const w = mount(TaskBoard, { props: { tasks, projectTitles } })
    expect(w.html()).not.toContain('rounded-full')
  })
})
