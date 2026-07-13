import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import InboxFocusCard from '../components/inbox/InboxFocusCard.vue'
import type { InboxItemData } from '../components/inbox/InboxRow.vue'
import type { Project } from '../composables/useProjects'

// In focus mode the keyboard IS the interface, so the tests drive it the way the
// user does: real keydown events on window, never the component's internals.
const press = (key: string) => window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))

function capture(over: Partial<InboxItemData> = {}): InboxItemData {
  return {
    id: 'c1',
    type: 'whatsapp',
    title: 'amanhã',
    subtitle: 'ligar pro dentista\ncomprar café',
    suggestedActions: [
      { target: 'task', title: 'Ligar pro dentista', body: 'ligar pro dentista' },
      { target: 'note', title: 'Café', body: 'comprar café' },
    ],
    ...over,
  }
}

const projects: Project[] = [
  { id: 'p1', title: 'Casa', description: '', status: 'active', createdAt: '', updatedAt: '', taskCount: 0, doneCount: 0 },
]

function focusCard(items: InboxItemData[] = [capture()]) {
  return mount(InboxFocusCard, { props: { items, projects } })
}

describe('InboxFocusCard', () => {
  let w: VueWrapper

  afterEach(() => w?.unmount())

  it('shows the capture whole, never the truncated title', () => {
    w = focusCard()
    expect(w.text()).toContain('ligar pro dentista')
    expect(w.text()).toContain('comprar café')
    expect(w.text()).toContain('01 / 01')
  })

  it('retypes the node under the cursor with a number key', async () => {
    w = focusCard()
    press('6') // Discard, the last target
    await w.vm.$nextTick()

    press('Enter')
    const [payload] = w.emitted('process')![0] as [{ actions: { target: string }[] }]
    expect(payload.actions[0].target).toBe('discard')
    expect(payload.actions[1].target).toBe('note') // the second node is untouched
  })

  it('j moves the cursor, so the number key lands on the second node', async () => {
    w = focusCard()
    press('j')
    await w.vm.$nextTick()
    press('4') // Task
    await w.vm.$nextTick()

    press('Enter')
    const [payload] = w.emitted('process')![0] as [{ actions: { target: string }[] }]
    expect(payload.actions[0].target).toBe('task')
    expect(payload.actions[1].target).toBe('task')
  })

  it('A adds a node seeded from the capture and X drops it again', async () => {
    w = focusCard()
    press('a')
    await w.vm.$nextTick()
    expect(w.text()).toContain('3 nodes')

    press('x')
    await w.vm.$nextTick()
    expect(w.text()).toContain('2 nodes')
  })

  it('an update cannot ride along with another node', async () => {
    w = focusCard()
    press('2') // Update, on the first of two nodes
    await w.vm.$nextTick()
    expect(w.text()).toContain('has to be the only node')

    press('Enter')
    expect(w.emitted('process')).toBeUndefined()
  })

  it('D discards the capture and S walks past it without processing', async () => {
    w = focusCard([capture(), capture({ id: 'c2', title: 'segunda captura' })])

    press('s')
    await w.vm.$nextTick()
    expect(w.text()).toContain('02 / 02')
    expect(w.emitted('process')).toBeUndefined()

    press('d')
    expect((w.emitted('discard')![0][0] as InboxItemData).id).toBe('c2')
  })

  it('keeps a capture edited then skipped past, so coming back is not a reset', async () => {
    w = focusCard([capture(), capture({ id: 'c2' })])
    press('6')                    // retype the first node of capture 1
    await w.vm.$nextTick()
    press('s')                    // walk to capture 2
    await w.vm.$nextTick()
    press('ArrowLeft')            // and back
    await w.vm.$nextTick()

    press('Enter')
    const [payload] = w.emitted('process')![0] as [{ actions: { target: string }[] }]
    expect(payload.actions[0].target).toBe('discard')
  })

  it('Escape leaves the mode and U reaches the shared undo stack', () => {
    w = focusCard()
    press('u')
    expect(w.emitted('undo')).toHaveLength(1)
    press('Escape')
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('lands on the empty state once the pile is gone', () => {
    w = focusCard([])
    expect(w.text()).toContain('Nothing left to triage')
  })
})
