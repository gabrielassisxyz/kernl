import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import NoteFrontmatter from '../components/notes/NoteFrontmatter.vue'
import { useEditorSettings, type FrontmatterMode } from '../composables/useEditorSettings'
import type { VaultNote } from '../composables/useVaultIndex'

const { settings } = useEditorSettings()

const NOTE: VaultNote = {
  id: 'n1',
  path: 'plans/scratchpad.md',
  title: 'Floating scratchpad',
  category: 'project',
  tags: ['plans', 'kernl'],
  author: 'human',
  pinned: false,
  createdAt: '2026-08-07T20:06:00Z',
  updatedAt: '2026-08-09T09:30:00Z',
}

const mountBlock = (mode: FrontmatterMode, extra: Record<string, unknown> = {}) => {
  settings.frontmatter = mode
  return mount(NoteFrontmatter, {
    props: {
      data: { title: 'Floating scratchpad', tags: ['plans', 'kernl'] },
      note: NOTE,
      ...extra,
    },
  })
}

beforeEach(() => {
  settings.frontmatter = 'inline'
})

describe('NoteFrontmatter: inline mode', () => {
  it('renders the title and a metadata line instead of the panel', () => {
    const wrapper = mountBlock('inline')

    expect(wrapper.get('.note-fm__title').text()).toBe('Floating scratchpad')
    expect(wrapper.find('.note-fm__inline').exists()).toBe(true)
    expect(wrapper.find('.note-fm__card').exists()).toBe(false)
    expect(wrapper.text()).toContain('# plans')
    expect(wrapper.text()).toContain('project')
    expect(wrapper.text()).toContain('created')
    expect(wrapper.text()).toContain('updated')
  })

  it('swaps the metadata line for the panel when the chevron is used', async () => {
    const wrapper = mountBlock('inline')

    await wrapper.get('button[aria-label="Show properties"]').trigger('click')

    expect(wrapper.find('.note-fm__inline').exists()).toBe(false)
    expect(wrapper.find('.note-properties').exists()).toBe(true)
    // The title survives the swap - it belongs to the mode, not to the header.
    expect(wrapper.find('.note-fm__title').exists()).toBe(true)
  })

  it('opens the panel already expanded, and its chevron returns to the line', async () => {
    const wrapper = mountBlock('inline')

    await wrapper.get('button[aria-label="Show properties"]').trigger('click')
    expect(wrapper.find('.note-properties').exists()).toBe(true)

    await wrapper.get('.note-fm__card-head').trigger('click')
    expect(wrapper.find('.note-fm__inline').exists()).toBe(true)
  })

  it('reads tags from the live frontmatter, not from the index entry', async () => {
    const wrapper = mountBlock('inline')

    await wrapper.setProps({ data: { title: 'Floating scratchpad', tags: ['plans'] } })

    expect(wrapper.text()).toContain('# plans')
    expect(wrapper.text()).not.toContain('# kernl')
  })
})

describe('NoteFrontmatter: panel mode', () => {
  it('renders the panel without a title heading', () => {
    const wrapper = mountBlock('panel')

    expect(wrapper.find('.note-fm__title').exists()).toBe(false)
    expect(wrapper.find('.note-fm__inline').exists()).toBe(false)
    expect(wrapper.find('.note-properties').exists()).toBe(true)
  })

  it('collapses to a one-line summary carrying the tag count and the category', async () => {
    const wrapper = mountBlock('panel')

    await wrapper.get('.note-fm__card-head').trigger('click')

    expect(wrapper.find('.note-properties').exists()).toBe(false)
    expect(wrapper.get('.note-fm__summary').text()).toContain('2 tags')
    expect(wrapper.get('.note-fm__category').text()).toBe('project')
  })

  it('singularizes a lone tag in the summary', async () => {
    const wrapper = mountBlock('panel', { data: { tags: ['plans'] } })

    await wrapper.get('.note-fm__card-head').trigger('click')

    expect(wrapper.get('.note-fm__summary').text()).toContain('1 tag')
  })
})

describe('NoteFrontmatter: degraded inputs', () => {
  it('forces the panel open on invalid YAML, in either mode', () => {
    for (const mode of ['inline', 'panel'] as FrontmatterMode[]) {
      const wrapper = mountBlock(mode, { parseError: 'bad yaml' })

      expect(wrapper.find('.note-fm__inline').exists()).toBe(false)
      expect(wrapper.text()).toContain('source mode')
    }
  })

  it('keeps the panel open when the head is clicked while YAML is invalid', async () => {
    const wrapper = mountBlock('panel', { parseError: 'bad yaml' })

    await wrapper.get('.note-fm__card-head').trigger('click')

    expect(wrapper.text()).toContain('source mode')
  })

  it('omits category and dates when the note is not in the index yet', () => {
    const wrapper = mountBlock('inline', { note: null })

    expect(wrapper.find('.note-fm__category').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('created')
    expect(wrapper.find('.note-fm__inline').exists()).toBe(true)
  })

  it('falls back to the index title when the frontmatter carries none', () => {
    const wrapper = mountBlock('inline', { data: { tags: [] } })

    expect(wrapper.get('.note-fm__title').text()).toBe('Floating scratchpad')
  })

  it('renders nothing at all when reading a note with no frontmatter', () => {
    const wrapper = mountBlock('inline', { data: {}, readonly: true })

    expect(wrapper.find('.note-fm').exists()).toBe(false)
  })
})
