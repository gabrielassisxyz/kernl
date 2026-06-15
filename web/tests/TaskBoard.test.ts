import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TaskBoard from '../components/tasks/TaskBoard.vue'
import TaskCard from '../components/tasks/TaskCard.vue'

function task(id: string, state: string) {
  return {
    id,
    type: 'task',
    state,
    title: id,
    priority: 1,
    labels: [],
    createdAt: '',
    updatedAt: '',
  }
}

describe('TaskBoard', () => {
  const tasks = [
    task('a', 'open'),
    task('b', 'ready_for_implementation'), // → open
    task('c', 'implementation'), // → in_progress
    task('d', 'blocked'), // → blocked
    task('e', 'closed'), // → done
  ]

  it('renders one card per task', () => {
    const w = mount(TaskBoard, { props: { tasks } })
    expect(w.findAllComponents(TaskCard)).toHaveLength(5)
  })

  it('buckets tasks into the four normalized columns', () => {
    const w = mount(TaskBoard, { props: { tasks } })
    const sections = w.findAll('section')
    expect(sections).toHaveLength(4)
    // Columns are rendered in TASK_COLUMNS order: Open, In Progress, Blocked, Done.
    const counts = sections.map((s) => s.get('.font-mono-data').text())
    expect(counts).toEqual(['2', '1', '1', '1'])
  })

  it('shows an em dash placeholder for an empty column', () => {
    const w = mount(TaskBoard, { props: { tasks: [task('only', 'open')] } })
    // Open has 1, the other three columns each show the empty placeholder.
    const dashes = w.findAll('section').filter((s) => s.text().includes('—'))
    expect(dashes.length).toBe(3)
  })

  it('emits open with the bead when a card is clicked', async () => {
    const w = mount(TaskBoard, { props: { tasks } })
    await w.findComponent(TaskCard).trigger('click')
    expect(w.emitted('open')).toBeTruthy()
  })
})
