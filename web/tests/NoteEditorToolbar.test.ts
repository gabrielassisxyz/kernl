import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import NoteEditorToolbar from '../components/notes/NoteEditorToolbar.vue'

const reloadButton = (wrapper: ReturnType<typeof mount>) =>
  wrapper.get('button[aria-label="Reload from disk"]')

describe('NoteEditorToolbar reload', () => {
  it('asks the editor to re-read the file', async () => {
    const wrapper = mount(NoteEditorToolbar, { props: { saveState: 'saved' } })

    await reloadButton(wrapper).trigger('click')

    expect(wrapper.emitted('reload-note')).toHaveLength(1)
  })

  it('refuses a second click while a reload is in flight', async () => {
    const wrapper = mount(NoteEditorToolbar, { props: { saveState: 'saved', reloading: true } })
    const button = reloadButton(wrapper)

    expect(button.attributes('disabled')).toBeDefined()
    expect(button.get('span').classes()).toContain('tbtn__icon--spinning')

    await button.trigger('click')
    expect(wrapper.emitted('reload-note')).toBeUndefined()
  })

  it('is available with unsaved edits, next to the save button', async () => {
    const wrapper = mount(NoteEditorToolbar, { props: { saveState: 'dirty' } })

    expect(reloadButton(wrapper).attributes('disabled')).toBeUndefined()
    await reloadButton(wrapper).trigger('click')
    expect(wrapper.emitted('reload-note')).toHaveLength(1)
  })
})
