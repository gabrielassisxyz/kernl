import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import TaskPanel from '../components/tasks/TaskPanel.vue'
import type { Task } from '../composables/useTasks'

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: 't1',
    title: 'A task',
    description: 'the original description',
    status: 'todo',
    projectId: '',
    tags: [],
    dueDate: '',
    createdAt: '2026-07-01T09:00:00Z',
    updatedAt: '2026-07-30T12:00:00Z',
    ...overrides,
  }
}

const mountPanel = (t: Task | null = task()) =>
  mount(TaskPanel, { props: { task: t, projects: [] } })

const titleField = (w: ReturnType<typeof mount>) => w.get('textarea[placeholder="What needs doing?"]')
const descField = (w: ReturnType<typeof mount>) =>
  w.get('textarea[placeholder="Optional context, links, acceptance criteria."]')
const tagsField = (w: ReturnType<typeof mount>) => w.get('input[placeholder="comma, separated"]')

// The panel autosaves per field on blur. Every test below is about what counts
// as a change worth sending, because a patch per keystroke-then-blur would
// rewrite the task's updatedAt for edits that changed nothing.
describe('TaskPanel autosave', () => {
  let w: ReturnType<typeof mount>
  beforeEach(() => {
    w = mountPanel()
  })

  it('patches the description on blur', async () => {
    await descField(w).setValue('a second draft')
    await descField(w).trigger('blur')
    expect(w.emitted('patch')).toEqual([['t1', { description: 'a second draft' }]])
  })

  it('does not patch an unchanged field', async () => {
    await descField(w).trigger('blur')
    await titleField(w).trigger('blur')
    expect(w.emitted('patch')).toBeUndefined()
  })

  // Blank is a real value here: it is how a description written by mistake gets
  // removed. The API accepts it; the panel must not swallow it.
  it('patches an emptied description, because clearing one is a legitimate edit', async () => {
    await descField(w).setValue('')
    await descField(w).trigger('blur')
    expect(w.emitted('patch')).toEqual([['t1', { description: '' }]])
  })

  // The API refuses a blank title, so sending one would just fail. The panel
  // restores the last good value instead of leaving the field empty on screen.
  it('restores the title rather than patching it blank', async () => {
    await titleField(w).setValue('   ')
    await titleField(w).trigger('blur')
    expect(w.emitted('patch')).toBeUndefined()
    expect((titleField(w).element as HTMLTextAreaElement).value).toBe('A task')
  })

  it('splits the tag field on commas and drops the blanks', async () => {
    await tagsField(w).setValue('ui , , design,')
    await tagsField(w).trigger('blur')
    expect(w.emitted('patch')).toEqual([['t1', { tags: ['ui', 'design'] }]])
  })

  it('does not patch tags that only changed spacing', async () => {
    const withTags = mountPanel(task({ tags: ['ui', 'design'] }))
    await tagsField(withTags).setValue('ui,design')
    await tagsField(withTags).trigger('blur')
    expect(withTags.emitted('patch')).toBeUndefined()
  })
})

describe('TaskPanel keyboard', () => {
  // The prototype this ports has no keyboard path to save at all - the only way
  // out of its panel is a click. These two are the replacement.
  it('closes on Ctrl+Enter when editing', async () => {
    const w = mountPanel()
    await descField(w).trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('creates on Ctrl+Enter when there is no task yet', async () => {
    const w = mountPanel(null)
    await titleField(w).setValue('typed and committed')
    await titleField(w).trigger('keydown', { key: 'Enter', metaKey: true })
    expect(w.emitted('create')?.[0][0]).toMatchObject({ title: 'typed and committed' })
    expect(w.emitted('close')).toBeUndefined()
  })

  it('leaves a bare Enter to the textarea', async () => {
    const w = mountPanel()
    await descField(w).trigger('keydown', { key: 'Enter' })
    expect(w.emitted('close')).toBeUndefined()
    expect(w.emitted('patch')).toBeUndefined()
  })

  it('closes on Escape', async () => {
    const w = mountPanel()
    await descField(w).trigger('keydown', { key: 'Escape' })
    expect(w.emitted('close')).toHaveLength(1)
  })
})

describe('TaskPanel create mode', () => {
  it('emits nothing until Create task, since there is no node to patch', async () => {
    const w = mountPanel(null)
    await titleField(w).setValue('a new one')
    await titleField(w).trigger('blur')
    await descField(w).setValue('with context')
    await descField(w).trigger('blur')
    expect(w.emitted('patch')).toBeUndefined()
  })

  it('refuses to create without a title', async () => {
    const w = mountPanel(null)
    await w.get('button[disabled]').trigger('click')
    expect(w.emitted('create')).toBeUndefined()
  })
})

describe('TaskPanel briefing', () => {
  it('shows the DA briefing when the task came from a capture', () => {
    const w = mount(TaskPanel, {
      props: { task: task(), projects: [], briefing: { body: 'why this was captured' } },
    })
    expect(w.text()).toContain('DA briefing')
    expect(w.text()).toContain('why this was captured')
  })

  it('shows nothing when there is no briefing', () => {
    expect(mountPanel().text()).not.toContain('DA briefing')
  })
})

describe('TaskPanel due date', () => {
  it('flags an overdue task', () => {
    const w = mountPanel(task({ dueDate: '2020-01-01' }))
    expect(w.text()).toContain('Overdue')
  })

  it('does not flag a finished task, however old its deadline', () => {
    const w = mountPanel(task({ dueDate: '2020-01-01', status: 'done' }))
    expect(w.text()).not.toContain('Overdue')
  })

  it('clears the deadline with an empty string, which is how the API removes it', async () => {
    const w = mountPanel(task({ dueDate: '2026-09-01' }))
    const clear = w.findAll('button').find((b) => b.text() === 'Clear')!
    await clear.trigger('click')
    expect(w.emitted('patch')).toEqual([['t1', { dueDate: '' }]])
  })
})
