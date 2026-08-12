import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import MarkdownEditor from '../components/notes/MarkdownEditor.vue'

const NOTE_PATH = 'notes/subject.md'

// The vault file endpoint answers with whatever the test last wrote to disk;
// every other endpoint the editor touches answers empty.
const stubVault = (diskContent: () => string) => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    if (url.startsWith('/api/vault/file')) {
      return new Response(diskContent(), {
        status: 200,
        headers: { 'Last-Modified': 'Wed, 12 Aug 2026 10:00:00 GMT' },
      })
    }
    if (url.startsWith('/api/notes/save')) {
      return new Response(JSON.stringify({ last_modified: 'saved' }), { status: 200 })
    }
    return new Response('[]', { status: 200 })
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const clickReload = async (wrapper: ReturnType<typeof mount>) => {
  await wrapper.get('button[aria-label="Reload from disk"]').trigger('click')
  await new Promise((resolve) => setTimeout(resolve, 0))
  await wrapper.vm.$nextTick()
}

const mountEditor = async () => {
  const wrapper = mount(MarkdownEditor, {
    props: { path: NOTE_PATH },
    attachTo: document.body,
  })
  await new Promise((resolve) => setTimeout(resolve, 0))
  await wrapper.vm.$nextTick()
  return wrapper
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('MarkdownEditor reload', () => {
  it('shows what the file says now, not what it said when it was opened', async () => {
    let disk = '# Before\n'
    stubVault(() => disk)

    const wrapper = await mountEditor()
    expect(wrapper.text()).toContain('Before')

    disk = '# After the DA wrote\n'
    await clickReload(wrapper)

    expect(wrapper.text()).toContain('After the DA wrote')
    wrapper.unmount()
  })

  it('leaves the file alone once the save raised a conflict', async () => {
    const disk = '# Before\n'
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/api/vault/file')) {
        return new Response(disk, {
          status: 200,
          headers: { 'Last-Modified': 'Wed, 12 Aug 2026 10:00:00 GMT' },
        })
      }
      // The file moved under us: the save is refused, and the reload must not
      // paper over the decision the conflict modal is about to ask for.
      if (url.startsWith('/api/notes/save')) return new Response('', { status: 409 })
      return new Response('[]', { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = await mountEditor()
    // An unsaved edit, which the reload has to flush before it can re-read.
    ;(wrapper.vm as unknown as { isDirty: boolean }).isDirty = true

    fetchMock.mockClear()
    await clickReload(wrapper)

    const reread = fetchMock.mock.calls.filter((call) => String(call[0]).startsWith('/api/vault/file'))
    expect(reread).toHaveLength(0)
    expect(wrapper.text()).toContain('Save Conflict')
    wrapper.unmount()
  })
})
