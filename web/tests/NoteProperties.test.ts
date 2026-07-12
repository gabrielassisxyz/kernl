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

  // A note's tags are graph objects: they share one namespace with every task,
  // project and bookmark the user tagged. The panel must therefore show — and
  // write — the same canonical name the tag tree is keyed by, or a note filed
  // under `homelab` would read as if it were filed under `Homelab`.
  describe('tags obey the canonical tag rules', () => {
    const addTag = async (wrapper: ReturnType<typeof mountBlock>, value: string) => {
      const input = wrapper.get('input[aria-label="Add value to tags"]')
      await input.setValue(value)
      await input.trigger('keydown.enter')
      await new Promise(r => setTimeout(r, 0))
    }

    it('renders a hand-edited tag under its canonical name', () => {
      const wrapper = mountBlock({ tags: ['Homelab', 'homelab/NAS'] })

      expect(wrapper.text()).toContain('homelab')
      expect(wrapper.text()).toContain('homelab/nas')
      expect(wrapper.text()).not.toContain('Homelab')
      expect(wrapper.text()).not.toContain('NAS')
    })

    it('writes the canonical name into the frontmatter', async () => {
      const wrapper = mountBlock({ tags: [] })
      await addTag(wrapper, 'Homelab/NAS')

      expect(wrapper.emitted('update:data')?.[0]?.[0]).toEqual({ tags: ['homelab/nas'] })
    })

    it('canonicalises the existing list on the next edit, so the file converges', async () => {
      const wrapper = mountBlock({ tags: ['Homelab'] })
      await addTag(wrapper, 'storage')

      expect(wrapper.emitted('update:data')?.[0]?.[0]).toEqual({ tags: ['homelab', 'storage'] })
    })

    it('removes the pill it rendered, even when the file spelled it differently', async () => {
      const wrapper = mountBlock({ tags: ['Homelab', 'storage'] })
      await wrapper.get('button[aria-label="Remove homelab"]').trigger('click')

      expect(wrapper.emitted('update:data')?.[0]?.[0]).toEqual({ tags: ['storage'] })
    })

    it('refuses a reserved sys/ tag instead of letting reconcile drop it in silence', async () => {
      const wrapper = mountBlock({ tags: [] })
      await addTag(wrapper, 'sys/pending')

      expect(wrapper.emitted('update:data')).toBeUndefined()
      expect(wrapper.text()).toContain('reserved')
    })

    it('refuses a name that breaks the nesting convention', async () => {
      const wrapper = mountBlock({ tags: [] })
      await addTag(wrapper, 'foo//bar')

      expect(wrapper.emitted('update:data')).toBeUndefined()
      expect(wrapper.text()).toContain('empty segment')
    })

    // Casing is the point of an alias — only `tags` is a graph namespace.
    it('leaves other list properties alone', async () => {
      const wrapper = mountBlock({ aliases: [] })
      const input = wrapper.get('input[aria-label="Add value to aliases"]')
      await input.setValue('Kernl HQ')
      await input.trigger('keydown.enter')
      await new Promise(r => setTimeout(r, 0))

      expect(wrapper.emitted('update:data')?.[0]?.[0]).toEqual({ aliases: ['Kernl HQ'] })
    })
  })
})
