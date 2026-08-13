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
    tags: [],
    dueDate: '',
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

const listProps = { projectTitles: {}, collapsed: { done: true, closed: true }, sortField: 'updated' as const, sortDir: 'desc' as const }
const mountList = (tasks: Task[], props = {}) => mount(TaskList, { props: { tasks, ...listProps, ...props } })
const rows = (w: ReturnType<typeof mount>) => w.findAll('.task-row')
const doneToggle = (w: ReturnType<typeof mount>) =>
  w.findAll('button').find((b) => b.text().includes('Done'))

describe('TaskList', () => {
  it('renders only the open tasks until the done section is opened', () => {
    const w = mountList(tasks)
    expect(rows(w)).toHaveLength(OPEN)
    expect(w.text()).not.toContain('done-1')
  })

  it('names the done section with its count, so the omission is visible', () => {
    const w = mountList(tasks)
    const toggle = doneToggle(w)!
    expect(toggle.text()).toContain(String(DONE))
    expect(toggle.attributes('aria-expanded')).toBe('false')
  })

  it('reveals the done tasks below the open ones when expanded', async () => {
    const w = mountList(tasks)
    await doneToggle(w)!.trigger('click')
    await w.setProps({ collapsed: { done: false, closed: true } })
    expect(rows(w)).toHaveLength(tasks.length)
    expect(doneToggle(w)!.attributes('aria-expanded')).toBe('true')
    // Order matters: the archive goes underneath the open work, never above it.
    const titles = rows(w).map((r) => r.text())
    expect(titles[0]).toContain('open-')
    expect(titles[titles.length - 1]).toContain('done-')
  })

  it('omits the done section entirely when nothing is done', () => {
    const w = mountList([task('a', 'todo')])
    expect(doneToggle(w)).toBeUndefined()
  })

  // An empty section is dropped rather than rendered as a heading over nothing,
  // so a list where everything is finished would otherwise be one collapsed
  // header and blank space.
  it('says the open list is empty rather than showing only a collapsed header', () => {
    const w = mountList([task('a', 'done')])
    expect(w.text()).toContain('Nothing open.')
    expect(doneToggle(w)).toBeDefined()
  })

  it('does not say that when there is open work', () => {
    const w = mountList([task('a', 'todo')])
    expect(w.text()).not.toContain('Nothing open.')
  })

  it('lets an open section collapse and preserves its count', async () => {
    const w = mountList([task('a', 'todo')])
    const header = w.findAll('button').find((button) => button.text().includes('To do'))!
    await header.trigger('click')
    expect(w.emitted('toggle-section')?.[0]).toEqual(['todo'])
    await w.setProps({ collapsed: { done: true, closed: true, todo: true } })
    expect(rows(w)).toHaveLength(0)
    expect(header.text()).toContain('1')
    expect(header.attributes('aria-expanded')).toBe('false')
  })

  it('sorts tasks within their status section', () => {
    const w = mountList([task('z', 'todo'), task('a', 'todo'), task('p', 'in_progress')], {
      sortField: 'name', sortDir: 'asc',
    })
    expect(rows(w).map((row) => row.findAll('span')[1].text())).toEqual(['p', 'a', 'z'])
  })

  // In progress leads, because it is the shortest section and the one being
  // worked on; to do follows; done is last and closed.
  it('orders the sections in progress, to do, done', async () => {
    const w = mountList([task('a', 'done'), task('b', 'todo'), task('c', 'in_progress')])
    await doneToggle(w)!.trigger('click')
    await w.setProps({ collapsed: { done: false, closed: true } })
    const headings = w.findAll('section').map((s) => s.text())
    expect(headings[0]).toContain('In progress')
    expect(headings[1]).toContain('To do')
    expect(headings[2]).toContain('Done')
  })

  it('emits open with the task when a row is clicked', async () => {
    const w = mountList([task('a', 'todo')])
    await rows(w)[0].trigger('click')
    expect(w.emitted('open')?.[0][0]).toMatchObject({ id: 'a' })
  })

  it('emits open for a done task too, once it is visible', async () => {
    const w = mountList([task('a', 'todo'), task('b', 'done')])
    await doneToggle(w)!.trigger('click')
    await w.setProps({ collapsed: { done: false, closed: true } })
    const all = rows(w)
    await all[all.length - 1].trigger('click')
    expect(w.emitted('open')?.[0][0]).toMatchObject({ id: 'b' })
  })

  // The bullet is the one action reachable without hovering, and it must not
  // also open the panel behind it.
  it('toggles done from the bullet without opening the task', async () => {
    const w = mountList([task('a', 'todo')])
    await rows(w)[0].get('button').trigger('click')
    expect(w.emitted('toggle-done')?.[0][0]).toMatchObject({ id: 'a' })
    expect(w.emitted('open')).toBeUndefined()
  })

  // Deleting is two-step and inline: the row asks, and the confirmation replaces
  // the action cluster rather than opening a dialog over the list.
  it('shows the inline confirmation only on the row being deleted', () => {
    const w = mountList([task('a', 'todo'), task('b', 'todo')], { confirmId: 'b' })
    expect(rows(w)[0].text()).not.toContain('Delete?')
    expect(rows(w)[1].text()).toContain('Delete?')
  })

  // The panel takes the right-hand column, so the row drops the metadata that
  // no longer fits rather than truncating every title to nothing.
  it('drops the metadata columns while the panel is open', () => {
    const withPanel = mountList([task('a', 'todo')], { projectTitles: { '': 'kernl' }, compact: true })
    expect(withPanel.get('.task-row').classes()).toContain('task-row--compact')
  })
})

// Work that was called off is off the board like finished work, but it is not
// the same answer to "what happened to this", so the list keeps the two apart.
describe('called-off work', () => {
  const mixed = [task('open', 'todo'), task('finished', 'done'), task('abandoned', 'closed')]
  const header = (w: ReturnType<typeof mount>, label: string) =>
    w.findAll('button').find((b) => b.text().includes(label))!

  it('gives it a section of its own rather than folding it into Done', () => {
    const w = mountList(mixed)
    const labels = w.findAll('.font-label-caps').map((s) => s.text())
    expect(labels).toEqual(['To do', 'Done', 'Closed'])
  })

  it('starts collapsed, like the finished pile', () => {
    const w = mountList(mixed)
    expect(rows(w)).toHaveLength(1)
    expect(w.text()).not.toContain('abandoned')
  })

  it('collapses independently of the finished pile', async () => {
    const w = mountList(mixed)
    await header(w, 'Closed').trigger('click')
    await w.setProps({ collapsed: { done: true, closed: false } })
    expect(w.text()).toContain('abandoned')
    // Opening one archive is not a request to see the other.
    expect(w.text()).not.toContain('finished')

    await header(w, 'Done').trigger('click')
    await w.setProps({ collapsed: { done: false, closed: false } })
    expect(w.text()).toContain('finished')
    expect(w.text()).toContain('abandoned')
  })

  it('reads as terminal in the row, leaving the section to say which', async () => {
    const w = mountList([task('abandoned', 'closed')])
    await header(w, 'Closed').trigger('click')
    await w.setProps({ collapsed: { done: true, closed: false } })
    expect(w.get('.task-row').html()).toContain('line-through')
  })

  // A backlog whose remainder was called off has nothing open in it, and
  // rendering only collapsed headers over blank space reads as a failed load.
  it('says so when everything left was finished or called off', () => {
    const w = mountList([task('finished', 'done'), task('abandoned', 'closed')])
    expect(w.text()).toContain('Nothing open.')
  })
})
