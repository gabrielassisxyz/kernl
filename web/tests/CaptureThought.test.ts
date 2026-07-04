import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CaptureThought from '../components/CaptureThought.vue'

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn().mockResolvedValue({})
  vi.stubGlobal('$fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('CaptureThought', () => {
  it('saves on Enter: posts the trimmed body, clears the field, emits submit', async () => {
    const w = mount(CaptureThought)
    const ta = w.find('textarea')
    await ta.setValue('  a new thought  ')
    await ta.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith('/api/inbox', { method: 'POST', body: { body: 'a new thought' } })
    expect((ta.element as HTMLTextAreaElement).value).toBe('')
    expect(w.emitted('submit')?.[0]).toEqual(['a new thought'])
  })

  it('Shift+Enter inserts a newline instead of saving', async () => {
    const w = mount(CaptureThought)
    const ta = w.find('textarea')
    await ta.setValue('line one')
    await ta.trigger('keydown', { key: 'Enter', shiftKey: true })
    await flushPromises()

    expect(fetchMock).not.toHaveBeenCalled()
    expect(w.emitted('submit')).toBeUndefined()
  })

  it('Escape clears the field and emits cancel without saving', async () => {
    const w = mount(CaptureThought)
    const ta = w.find('textarea')
    await ta.setValue('discard me')
    await ta.trigger('keydown', { key: 'Escape' })
    await flushPromises()

    expect(fetchMock).not.toHaveBeenCalled()
    expect(w.emitted('cancel')).toHaveLength(1)
    expect((ta.element as HTMLTextAreaElement).value).toBe('')
  })

  it('does not save an empty or whitespace-only thought', async () => {
    const w = mount(CaptureThought)
    const ta = w.find('textarea')
    await ta.setValue('   ')
    await ta.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(fetchMock).not.toHaveBeenCalled()
    expect(w.emitted('submit')).toBeUndefined()
  })

  it('shows the idle terminal cursor only while the field is empty', async () => {
    const w = mount(CaptureThought)
    expect(w.find('.blinking-cursor').attributes('style') || '').not.toContain('display: none')
    await w.find('textarea').setValue('typing')
    expect(w.find('.blinking-cursor').attributes('style') || '').toContain('display: none')
  })

  it('focuses the textarea on mount so the user can type immediately', () => {
    const w = mount(CaptureThought, { attachTo: document.body })
    expect(document.activeElement).toBe(w.find('textarea').element)
    w.unmount()
  })
})
