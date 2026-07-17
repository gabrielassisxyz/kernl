import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import InboxBatchDump from '../components/inbox/InboxBatchDump.vue'

let fetchMock: ReturnType<typeof vi.fn>
// Routed by URL rather than call order: the component now hits several endpoints
// (preview, auto-classify, analyze, create), so positional mocks would couple
// every test to the exact call sequence. Tests override a handler to change one
// endpoint's behavior.
let handlers: Record<string, (opts?: { method?: string; body?: unknown }) => unknown>

beforeEach(() => {
  handlers = {
    '/api/inbox/batch/preview': () => twoMessagePreview,
    '/api/inbox/auto-classify': () => ({ enabled: true, llmConfigured: true }),
    '/api/inbox/batch/analyze': () => ({ ...twoMessagePreview, mergeProposals: [] }),
    '/api/inbox/batch': () => ({ batchId: 'batch-1', segments: [] }),
  }
  fetchMock = vi.fn((url: string, opts?: { method?: string; body?: unknown }) => {
    const handler = handlers[url]
    if (!handler) return Promise.reject(new Error(`unexpected fetch: ${url}`))
    return Promise.resolve(handler(opts))
  })
  vi.stubGlobal('$fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const twoMessagePreview = {
  source: 'whatsapp',
  separator: 'whatsapp',
  suggestedContextTitle: 'Project idea',
  segments: [
    { body: 'Project idea', timestamp: '7/4/2026 14:12', sequence: 0, parseConfidence: 'high' },
    { body: 'Task idea', timestamp: '7/4/2026 14:57', sequence: 1, parseConfidence: 'high' },
  ],
  finalSegments: [
    { body: 'Project idea', timestamp: '7/4/2026 14:12', sequence: 0, sourceSequences: [0], confidence: 'high' },
    { body: 'Task idea', timestamp: '7/4/2026 14:57', sequence: 1, sourceSequences: [1], confidence: 'high' },
  ],
}

function clickButton(w: ReturnType<typeof mount>, label: string) {
  return w.findAll('button').find(button => button.text().includes(label))!.trigger('click')
}

function callTo(url: string) {
  return fetchMock.mock.calls.find(call => call[0] === url)
}

describe('InboxBatchDump', () => {
  it('uses the shared capture input affordances', () => {
    const w = mount(InboxBatchDump)

    expect(w.text()).toContain('Upload file')
    expect(w.text()).toContain('[ESC] cancel')
    expect(w.text()).toContain('[SHIFT+ENTER] new line')
    expect(w.text()).toContain('[ENTER] new line')
    expect(w.text()).not.toContain('Review split before creating')
  })

  // Reading your own messages must not wait on a model: the modal opens on the
  // mechanical split, and enrichment lands on top of it afterwards.
  it('opens the review modal on the mechanical split, before the LLM answers', async () => {
    let releaseAnalyze: (value: unknown) => void = () => {}
    handlers['/api/inbox/batch/analyze'] = () => new Promise(resolve => { releaseAnalyze = resolve })

    const w = mount(InboxBatchDump)
    await w.find('textarea').setValue('[7/4/2026, 14:12] Gabriel: Project idea')
    await clickButton(w, 'Create captures')
    await flushPromises()

    expect(callTo('/api/inbox/batch/preview')).toEqual([
      '/api/inbox/batch/preview',
      {
        method: 'POST',
        body: {
          text: '[7/4/2026, 14:12] Gabriel: Project idea',
          source: '',
          separator: 'auto',
          contextTitle: '',
        },
      },
    ])
    // Modal is up with the real messages while /analyze is still in flight.
    expect(w.text()).toContain('2 captures will be created')
    expect(w.text()).toContain('14:12')
    expect(w.text()).toContain('14:57')
    expect(w.text()).toContain('Looking for messages that say the same thing')

    releaseAnalyze({ ...twoMessagePreview, mergeProposals: [] })
    await flushPromises()
    expect(w.text()).toContain('every message stays its own capture')
  })

  // The batch modal reflects the live auto-classify switch and writes it back.
  it('reads the auto-classify switch on open and toggles it', async () => {
    handlers['/api/inbox/auto-classify'] = (opts) => ({
      enabled: opts?.method === 'PUT' ? (opts.body as { enabled: boolean }).enabled : false,
      llmConfigured: true,
    })

    const w = mount(InboxBatchDump)
    await w.find('textarea').setValue('[7/4/2026, 14:12] Gabriel: Project idea')
    await clickButton(w, 'Create captures')
    await flushPromises()

    const checkbox = w.find('input[type="checkbox"]')
    expect((checkbox.element as HTMLInputElement).checked).toBe(false)

    await checkbox.setValue(true)
    await flushPromises()
    const putCall = fetchMock.mock.calls.find(
      call => call[0] === '/api/inbox/auto-classify' && call[1]?.method === 'PUT',
    )
    expect(putCall?.[1]).toEqual({ method: 'PUT', body: { enabled: true } })
    expect((checkbox.element as HTMLInputElement).checked).toBe(true)
  })

  // A merge deletes a message from the record. It is offered, never applied.
  it('offers a suggested merge without applying it', async () => {
    handlers['/api/inbox/batch/analyze'] = () => ({
      ...twoMessagePreview,
      mergeProposals: [{ sourceSequences: [0, 1], reason: 'same request, restated' }],
    })

    const w = mount(InboxBatchDump)
    await w.find('textarea').setValue('[7/4/2026, 14:12] Gabriel: Project idea')
    await clickButton(w, 'Create captures')
    await flushPromises()

    expect(w.text()).toContain('same request, restated')
    expect(w.text()).toContain('14:12 + 14:57')
    // Still two captures: nobody accepted the merge.
    expect(w.text()).toContain('2 captures will be created')

    await clickButton(w, 'Merge')
    expect(w.text()).toContain('1 captures will be created')

    await clickButton(w, 'Keep separate')
    expect(w.text()).toContain('2 captures will be created')
  })

  it('posts the reviewed split and the accepted merges on create', async () => {
    handlers['/api/inbox/batch/analyze'] = () => ({
      ...twoMessagePreview,
      mergeProposals: [{ sourceSequences: [0, 1], reason: 'same request, restated' }],
    })

    const w = mount(InboxBatchDump)
    const textarea = w.find('textarea')
    await textarea.setValue('[7/4/2026, 14:12] Gabriel: Project idea')
    await clickButton(w, 'Create captures')
    await flushPromises()

    await clickButton(w, 'Merge')
    await clickButton(w, 'Create 1 captures')
    await flushPromises()

    const [url, options] = callTo('/api/inbox/batch')!
    expect(url).toBe('/api/inbox/batch')
    expect(options.body.rawSegments).toEqual(twoMessagePreview.segments)
    // One candidate, carrying the merge the user accepted. The body is a preview:
    // the server rebuilds it from the pasted text.
    expect(options.body.finalSegments).toHaveLength(1)
    expect(options.body.finalSegments[0].sourceSequences).toEqual([0, 1])
    expect(options.body.finalSegments[0].body).toBe('Project idea\n\nTask idea')

    expect(w.emitted('created')?.[0]).toEqual(['batch-1'])
    expect((textarea.element as HTMLTextAreaElement).value).toBe('')
  })

  // Enrichment is a bonus, not a dependency: if it fails, the messages as pasted
  // are still exactly what gets created.
  it('still creates the captures when enrichment fails', async () => {
    handlers['/api/inbox/batch/analyze'] = () => Promise.reject(new Error('llm down'))

    const w = mount(InboxBatchDump)
    await w.find('textarea').setValue('[7/4/2026, 14:12] Gabriel: Project idea')
    await clickButton(w, 'Create captures')
    await flushPromises()

    expect(w.text()).toContain('2 captures will be created')

    await clickButton(w, 'Create 2 captures')
    await flushPromises()

    expect(w.emitted('created')?.[0]).toEqual(['batch-1'])
  })
})
