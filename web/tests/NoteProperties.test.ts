import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import NoteProperties from '../components/notes/NoteProperties.vue'

const mountBlock = (data: Record<string, unknown>, extra: Record<string, unknown> = {}) =>
  mount(NoteProperties, { props: { data, ...extra } })

describe('NoteProperties', () => {
  it('renders text, tag list, and date rows', () => {
    const wrapper = mountBlock({
      title: 'Typed note',
      tags: ['telos', 'da'],
      reviewed: '2026-06-28',
    })

    expect(wrapper.text()).toContain('title')
    expect(wrapper.get('input[aria-label="title value"]').element).toHaveProperty('value', 'Typed note')
    expect(wrapper.text()).toContain('telos')
    expect(wrapper.find('input[type="date"]').element).toHaveProperty('value', '2026-06-28')
  })

  it('emits a full frontmatter update when a text value changes', async () => {
    const wrapper = mountBlock({ title: 'Old', tags: [] })

    const input = wrapper.get('input[aria-label="title value"]')
    await input.setValue('New')
    await input.trigger('change')
    await new Promise(r => setTimeout(r, 0))

    expect(wrapper.emitted('update:data')?.[0]?.[0]).toEqual({ title: 'New', tags: [] })
  })

  it('adds and removes tags without dropping other properties', async () => {
    const wrapper = mountBlock({ title: 'Note', tags: ['da'] })

    await wrapper.get('input[aria-label="Add value to tags"]').setValue('telos')
    await wrapper.get('input[aria-label="Add value to tags"]').trigger('keydown.enter')
    await new Promise(r => setTimeout(r, 0))

    expect(wrapper.emitted('update:data')?.[0]?.[0]).toEqual({ title: 'Note', tags: ['da', 'telos'] })

    await wrapper.setProps({ data: { title: 'Note', tags: ['da', 'telos'] } })
    await wrapper.get('button[aria-label="Remove telos"]').trigger('click')
    await new Promise(r => setTimeout(r, 0))

    expect(wrapper.emitted('update:data')?.[1]?.[0]).toEqual({ title: 'Note', tags: ['da'] })
  })

  it('adds the first property through the add affordance', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-06-28T12:00:00Z'))
    const wrapper = mountBlock({})

    await wrapper.get('.note-properties__add-trigger').trigger('click')
    await wrapper.get('input[aria-label="New property name"]').setValue('reviewed')
    await wrapper.get('select[aria-label="New property type"]').setValue('date')
    await wrapper.get('.note-properties__add-form').trigger('submit')
    vi.runAllTimers()

    expect(wrapper.emitted('update:data')?.[0]?.[0]).toEqual({ reviewed: '2026-06-28' })
    vi.useRealTimers()
  })

  it('hides the internal uuid, and hides id by default', () => {
    const wrapper = mountBlock({ id: 'abc123', uuid: 'zzz', title: 'Note' })

    // uuid is internal — never rendered; id is hidden unless showId is set.
    expect(wrapper.text()).not.toContain('zzz')
    expect(wrapper.find('input[aria-label="id value"]').exists()).toBe(false)
  })

  it('shows the node id locked when showId is enabled', () => {
    const wrapper = mountBlock({ id: 'abc123', title: 'Note' }, { showId: true })

    // id is shown but locked: present, not editable, not removable, with a lock.
    expect(wrapper.get('input[aria-label="id value"]').attributes('readonly')).toBeDefined()
    expect(wrapper.find('button[aria-label="Remove property id"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('lock')
  })

  it('is non-editable in reading (readonly) mode', () => {
    const wrapper = mountBlock({ title: 'Note', tags: ['da'] }, { readonly: true })

    expect(wrapper.get('input[aria-label="title value"]').attributes('readonly')).toBeDefined()
    expect(wrapper.find('input[aria-label="Add value to tags"]').exists()).toBe(false)
    expect(wrapper.find('.note-properties__add-trigger').exists()).toBe(false)
  })

  it('shows a notice when the frontmatter YAML is invalid', () => {
    const wrapper = mountBlock({}, { parseError: 'bad yaml' })

    expect(wrapper.text()).toContain('source mode')
  })
})
