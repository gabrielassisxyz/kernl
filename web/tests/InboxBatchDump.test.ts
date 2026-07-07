import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import InboxBatchDump from '../components/inbox/InboxBatchDump.vue'

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn()
  vi.stubGlobal('$fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('InboxBatchDump', () => {
  it('uses the shared capture input affordances', () => {
    const w = mount(InboxBatchDump)

    expect(w.text()).toContain('Upload file')
    expect(w.text()).toContain('[ESC] cancel')
    expect(w.text()).toContain('[SHIFT+ENTER] new line')
    expect(w.text()).toContain('[ENTER] new line')
    expect(w.text()).not.toContain('Review split before creating')
  })

  it('analyzes a dump before opening the review modal', async () => {
    fetchMock.mockResolvedValueOnce({
      source: 'whatsapp',
      separator: 'whatsapp',
      suggestedContextTitle: 'Project idea',
      segments: [
        { body: 'Project idea', timestamp: '7/4/2026 14:12', sequence: 0, parseConfidence: 'high' },
        { body: 'Task idea', timestamp: '7/4/2026 14:57', sequence: 1, parseConfidence: 'high' },
      ],
    })

    const w = mount(InboxBatchDump)
    await w.find('textarea').setValue('[7/4/2026, 14:12] Gabriel: Project idea')
    await w.findAll('button').find(button => button.text().includes('Create captures'))!.trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith('/api/inbox/batch/analyze', {
      method: 'POST',
      body: {
        text: '[7/4/2026, 14:12] Gabriel: Project idea',
        source: '',
        separator: 'auto',
        contextTitle: '',
      },
    })
    expect(w.text()).toContain('2 captures will be created')
    expect(w.text()).toContain('14:12')
    expect(w.text()).toContain('14:57')
    expect(w.text()).not.toContain('Gabriel')
  })

  it('creates captures from the reviewed modal state', async () => {
    fetchMock
      .mockResolvedValueOnce({
        source: 'text',
        separator: 'lines',
        suggestedContextTitle: 'Project idea',
        segments: [{ body: 'Project idea', timestamp: '', sequence: 0, parseConfidence: 'medium' }],
      })
      .mockResolvedValueOnce({
        batchId: 'batch-1',
        segments: [{ body: 'Project idea', timestamp: '', sequence: 0, parseConfidence: 'medium' }],
      })

    const w = mount(InboxBatchDump)
    const textarea = w.find('textarea')
    await textarea.setValue('Project idea')
    await w.findAll('button').find(button => button.text().includes('Create captures'))!.trigger('click')
    await flushPromises()

    await w.findAll('button').find(button => button.text().includes('Create 1 captures'))!.trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenLastCalledWith('/api/inbox/batch', {
      method: 'POST',
      body: {
        text: 'Project idea',
        source: 'text',
        separator: 'lines',
        contextTitle: 'Project idea',
      },
    })
    expect(w.emitted('created')?.[0]).toEqual(['batch-1'])
    expect((textarea.element as HTMLTextAreaElement).value).toBe('')
  })
})
