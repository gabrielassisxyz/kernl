import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EditorView } from '@codemirror/view'
import { deleteCharBackward } from '@codemirror/commands'
import MarkdownEditor from '../components/notes/MarkdownEditor.vue'
import { useEditorSettings } from '../composables/useEditorSettings'

// Reading mode used to be read-only only by omission: almost no commands were
// bound, so there was nothing to run. Now that the editor carries a full keymap,
// a document that is merely `EditorView.editable: false` still accepts every
// command a keystroke can reach - and the autosave would persist the result.
const DOC = '# Title\n'

// The stubbed file content is a parameter so a suite can load a note whose
// body exercises what the mode flip has to keep working.
const stubVault = (doc: string = DOC) => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    if (url.startsWith('/api/vault/file')) {
      return new Response(doc, {
        status: 200,
        headers: { 'Last-Modified': 'Wed, 12 Aug 2026 10:00:00 GMT' },
      })
    }
    return new Response('[]', { status: 200 })
  }))
}

const mountEditor = async () => {
  const wrapper = mount(MarkdownEditor, { props: { path: 'notes/x.md' }, attachTo: document.body })
  await new Promise((resolve) => setTimeout(resolve, 0))
  await wrapper.vm.$nextTick()
  return wrapper
}

const viewOf = (wrapper: ReturnType<typeof mount>): EditorView => {
  const dom = wrapper.element.querySelector('.cm-editor')
  const view = dom ? EditorView.findFromDOM(dom as HTMLElement) : null
  if (!view) throw new Error('no CodeMirror view mounted')
  return view
}

const setMode = async (wrapper: ReturnType<typeof mount>, mode: 'live' | 'reading') => {
  useEditorSettings().settings.viewMode = mode
  await wrapper.vm.$nextTick()
}

afterEach(() => {
  vi.unstubAllGlobals()
  useEditorSettings().settings.viewMode = 'live'
})

describe('MarkdownEditor reading mode', () => {
  it('refuses an editing command, while live mode accepts the same one', async () => {
    stubVault()
    const wrapper = await mountEditor()
    const view = viewOf(wrapper)
    // Away from position 0, so there is always a character to delete.
    view.dispatch({ selection: { anchor: 5 } })

    await setMode(wrapper, 'reading')
    expect(deleteCharBackward(view)).toBe(false)
    expect(view.state.doc.toString()).toBe(DOC)

    await setMode(wrapper, 'live')
    expect(deleteCharBackward(view)).toBe(true)
    expect(view.state.doc.toString()).not.toBe(DOC)

    wrapper.unmount()
  })

  it('renders the task checkbox disabled in reading mode and a click changes nothing', async () => {
    // The reading mode is the same document faceted uneditable, so the checkbox
    // still renders there - showing its state, not asking to act as a control.
    const tasks = '# Title\n\n- [ ] pending\n- [x] done\n'
    stubVault(tasks)
    const wrapper = await mountEditor()
    const view = viewOf(wrapper)
    expect(view.state.doc.toString()).toBe(tasks)

    await setMode(wrapper, 'reading')
    const box = wrapper.element.querySelector<HTMLInputElement>('.cm-md-task-check input')
    expect(box).not.toBeNull()
    expect(box?.disabled).toBe(true)

    // happy-dom dispatches handlers even on disabled inputs; the widget guard
    // makes the click a no-op either way. Real browsers skip a disabled control.
    box?.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    expect(view.state.doc.toString()).toBe(tasks)

    wrapper.unmount()
  })
})
