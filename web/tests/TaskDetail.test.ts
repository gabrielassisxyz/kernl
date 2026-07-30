import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import TaskDetail from '../components/tasks/TaskDetail.vue'
import type { Task } from '../composables/useTasks'

// The panel fetches a DA briefing on open; a 404 is the normal case and the one
// the description tests want, so stub it rejecting rather than leaving $fetch
// undefined and failing for the wrong reason.
vi.stubGlobal('$fetch', vi.fn(() => Promise.reject(new Error('404'))))

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: 't1',
    title: 'A task',
    description: 'the original description',
    status: 'todo',
    projectId: '',
    dueDate: '',
    createdAt: '',
    updatedAt: '2026-07-30T12:00:00Z',
    ...overrides,
  }
}

// The description is the only editable field with no dedicated control - it is
// prose the user clicks into, like the title above it. These tests pin the two
// ways that differs from the title: Enter is a newline here, and empty is a
// legitimate value rather than a rejected one.
async function openEditor(w: ReturnType<typeof mount>) {
  await w.get('[data-test="description-display"]').trigger('click')
  return w.get('[data-test="description-input"]')
}

describe('TaskDetail description editing', () => {
  let wrapper: ReturnType<typeof mount>

  beforeEach(() => {
    wrapper = mount(TaskDetail, { props: { task: task() } })
  })

  it('shows the description as prose until it is clicked', async () => {
    expect(wrapper.get('[data-test="description-display"]').text()).toContain('the original description')
    expect(wrapper.find('[data-test="description-input"]').exists()).toBe(false)
    await openEditor(wrapper)
    expect(wrapper.find('[data-test="description-input"]').exists()).toBe(true)
  })

  it('emits the edited text on blur', async () => {
    const input = await openEditor(wrapper)
    await input.setValue('a second draft')
    await input.trigger('blur')
    expect(wrapper.emitted('set-description')).toEqual([['t1', 'a second draft']])
  })

  it('commits on Ctrl+Enter, because a bare Enter has to stay a newline', async () => {
    const input = await openEditor(wrapper)
    await input.setValue('line one\nline two')
    await input.trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(wrapper.emitted('set-description')).toEqual([['t1', 'line one\nline two']])
  })

  it('leaves a bare Enter to the textarea instead of committing', async () => {
    const input = await openEditor(wrapper)
    await input.setValue('half a thought')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('set-description')).toBeUndefined()
    expect(wrapper.find('[data-test="description-input"]').exists()).toBe(true)
  })

  it('discards the draft on Escape', async () => {
    const input = await openEditor(wrapper)
    await input.setValue('typed then regretted')
    await input.trigger('keydown.esc')
    expect(wrapper.emitted('set-description')).toBeUndefined()
    expect(wrapper.get('[data-test="description-display"]').text()).toContain('the original description')
  })

  it('does not PATCH an unchanged description', async () => {
    const input = await openEditor(wrapper)
    await input.trigger('blur')
    expect(wrapper.emitted('set-description')).toBeUndefined()
  })

  // Unlike the title, blank is a real value: it is how a description written by
  // mistake gets removed. The API accepts it; the UI must not swallow it.
  it('emits an empty string, because clearing a description is a legitimate edit', async () => {
    const input = await openEditor(wrapper)
    await input.setValue('   ')
    await input.trigger('blur')
    expect(wrapper.emitted('set-description')).toEqual([['t1', '']])
  })

  // A task with no description still needs somewhere to click, or the only way
  // to add one is to delete the task and recreate it.
  it('offers the empty state as the click target when there is no description', async () => {
    const w = mount(TaskDetail, { props: { task: task({ description: '' }) } })
    const display = w.get('[data-test="description-display"]')
    expect(display.text()).toContain('No description')
    await display.trigger('click')
    const input = w.get('[data-test="description-input"]')
    await input.setValue('written from empty')
    await input.trigger('blur')
    expect(w.emitted('set-description')).toEqual([['t1', 'written from empty']])
  })

  // Switching tasks with an editor open would otherwise show the previous
  // task's draft over the new task's field - the same guard the title has.
  it('drops a half-finished edit when the panel switches task', async () => {
    const input = await openEditor(wrapper)
    await input.setValue('belongs to t1')
    await wrapper.setProps({ task: task({ id: 't2', description: 'another task' }) })
    expect(wrapper.find('[data-test="description-input"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="description-display"]').text()).toContain('another task')
  })
})
