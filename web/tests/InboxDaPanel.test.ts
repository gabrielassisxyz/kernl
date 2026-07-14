import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import InboxDaPanel from '../components/inbox/InboxDaPanel.vue'
import type { CaptureAction } from '../utils/inboxTargets'

// The panel drives the real composable, so the fetch it makes IS the contract
// under test: the DA has to be told what is on screen, not just what was asked.
const fetchMock = vi.fn()
vi.stubGlobal('fetch', fetchMock)
vi.stubGlobal('EventSource', class {
  close() {}
  set onmessage(_: unknown) {}
  set onerror(_: unknown) {}
} as unknown as typeof EventSource)

const draft: CaptureAction[] = [
  { target: 'task', title: 'Ligar pro dentista', dueDate: '2026-04-02' },
]

function bodyOf(call: unknown[]): Record<string, unknown> {
  return JSON.parse((call[1] as { body: string }).body)
}

describe('InboxDaPanel', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    fetchMock.mockResolvedValue({ json: () => Promise.resolve({ id: 'session-1' }) })
  })

  // The trap the backend work exists to close: the chat runs from the persisted
  // session, so a draft that stays in the browser never reaches the DA.
  it('sends the on-screen routing along with the question', async () => {
    const w = mount(InboxDaPanel, { props: { captureId: 'c1', draft } })

    await w.find('input').setValue('why a task?')
    await w.find('input').trigger('keydown.enter')
    await flushPromises()

    const message = fetchMock.mock.calls.find(c => String(c[0]).endsWith('/messages'))
    expect(message, 'no message was posted').toBeTruthy()

    const body = bodyOf(message!)
    expect(body.content).toBe('why a task?')
    expect(body.scope_node_id).toBe('c1')
    expect(body.draftActions).toEqual(draft)
  })

  it('does not ask an empty question', async () => {
    const w = mount(InboxDaPanel, { props: { captureId: 'c1', draft } })
    await w.find('input').trigger('keydown.enter')
    await flushPromises()

    expect(fetchMock.mock.calls.some(c => String(c[0]).endsWith('/messages'))).toBe(false)
  })
})
