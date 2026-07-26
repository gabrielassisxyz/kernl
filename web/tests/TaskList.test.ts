import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TaskList from '../components/tasks/TaskList.vue'
import type { Task, TaskStatus } from '../composables/useTasks'

function task(id: string, status: TaskStatus): Task {
  return {
    id,
    title: id,
    description: '',
    status,
    projectId: '',
    createdAt: '',
    updatedAt: '',
  }
}

// The proportions the collapsed section exists for. A backlog with any history
// is mostly history: the import this was sized for carries 258 finished entries
// against 55 open ones. Three tasks would prove the split renders, not that it
// keeps the open work visible.
const OPEN = 55
const DONE = 258

// Interleaved, not grouped: a split that works on a body sorted by status works
// by accident.
const tasks: Task[] = Array.from({ length: OPEN + DONE }, (_, i) =>
  i % 5 === 0 && i / 5 < OPEN ? task(`open-${i}`, 'todo') : task(`done-${i}`, 'done'),
)

// Every row carries an id cell; the toggle row is the one with a button in it.
const taskRows = (w: ReturnType<typeof mount>) => w.findAll('tbody tr').filter((r) => !r.find('button').exists())

describe('TaskList', () => {
  const projectTitles = {}

  it('renders only the open tasks until the done section is opened', () => {
    const w = mount(TaskList, { props: { tasks, projectTitles } })
    expect(taskRows(w)).toHaveLength(tasks.filter((t) => t.status !== 'done').length)
    expect(w.text()).not.toContain('done-1')
  })

  it('names the done section with its count, so the omission is visible', () => {
    const w = mount(TaskList, { props: { tasks, projectTitles } })
    const toggle = w.get('tbody button')
    expect(toggle.text()).toContain('Done')
    expect(toggle.text()).toContain(String(DONE))
    expect(toggle.attributes('aria-expanded')).toBe('false')
  })

  it('reveals the done tasks below the open ones when expanded', async () => {
    const w = mount(TaskList, { props: { tasks, projectTitles } })
    await w.get('tbody button').trigger('click')
    expect(taskRows(w)).toHaveLength(tasks.length)
    expect(w.get('tbody button').attributes('aria-expanded')).toBe('true')
    // Order matters: the archive goes underneath the open work, never above it.
    const titles = taskRows(w).map((r) => r.text())
    expect(titles[0]).toContain('open-')
    expect(titles[titles.length - 1]).toContain('done-')
  })

  it('omits the done section entirely when nothing is done', () => {
    const w = mount(TaskList, { props: { tasks: [task('a', 'todo')], projectTitles } })
    expect(w.find('tbody button').exists()).toBe(false)
  })

  it('says the open list is empty rather than showing a bare header', () => {
    const w = mount(TaskList, { props: { tasks: [task('a', 'done')], projectTitles } })
    expect(w.text()).toContain('Nothing open.')
    expect(w.get('tbody button').exists()).toBe(true)
  })

  it('emits open with the task when a row is clicked', async () => {
    const w = mount(TaskList, { props: { tasks: [task('a', 'todo')], projectTitles } })
    await taskRows(w)[0].trigger('click')
    expect(w.emitted('open')?.[0][0]).toMatchObject({ id: 'a' })
  })

  it('emits open for a done task too, once it is visible', async () => {
    const w = mount(TaskList, { props: { tasks: [task('a', 'todo'), task('b', 'done')], projectTitles } })
    await w.get('tbody button').trigger('click')
    const rows = taskRows(w)
    await rows[rows.length - 1].trigger('click')
    expect(w.emitted('open')?.[0][0]).toMatchObject({ id: 'b' })
  })
})
