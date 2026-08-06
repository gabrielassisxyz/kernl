import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import AppSidebar from '~/components/AppSidebar.vue'

const NuxtLink = defineComponent({
  props: {
    to: { type: [String, Object], required: true },
  },
  template: '<a><slot /></a>',
})

function mountSidebar() {
  return mount(AppSidebar, {
    global: {
      components: { NuxtLink },
    },
  })
}

describe('AppSidebar', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.stubGlobal('useRoute', () => ({ path: '/', query: {} }))
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('starts expanded with grouped destinations and no account entry', () => {
    const wrapper = mountSidebar()

    expect(wrapper.text()).toContain('Overview')
    expect(wrapper.text()).toContain('Bookmarks')
    expect(wrapper.text()).toContain('Orchestrator')
    expect(wrapper.text()).toContain('Settings')
    expect(wrapper.text()).not.toContain('Account')
  })

  it('persists the collapsed rail preference', async () => {
    const wrapper = mountSidebar()

    await wrapper.get('button[aria-label="Collapse sidebar"]').trigger('click')

    expect(wrapper.classes()).toContain('app-sidebar--collapsed')
    expect(localStorage.getItem('kernl:sidebar-collapsed')).toBe('1')
    expect(wrapper.text()).not.toContain('Overview')
  })

  it('loads real shortcuts on disclosure and closes them with Escape', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => [
        {
          id: 'note-1',
          path: 'today.md',
          title: 'Today',
          type: 'note',
          updatedAt: '2026-08-02T12:00:00Z',
        },
      ],
    } as Response)
    const wrapper = mountSidebar()

    await wrapper.get('button[aria-label="Show Notes shortcuts"]').trigger('click')
    await flushPromises()

    expect(fetch).toHaveBeenCalledWith('/api/vault/notes')
    expect(wrapper.text()).toContain('Today')
    expect(wrapper.text()).not.toContain('View more')

    await wrapper.get('nav').trigger('keydown', { key: 'Escape' })
    expect(wrapper.text()).not.toContain('Today')
  })

  it('reveals shortcuts in batches until the collection is exhausted', async () => {
    const notes = Array.from({ length: 56 }, (_, index) => ({
      id: `note-${index}`,
      path: `note-${index}.md`,
      title: `Note ${index}`,
      type: 'note',
      updatedAt: new Date(Date.UTC(2026, 7, 2, 12, index)).toISOString(),
    }))
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => notes,
    } as Response)
    const wrapper = mountSidebar()

    await wrapper.get('button[aria-label="Show Notes shortcuts"]').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('.sidebar-shortcut')).toHaveLength(5)
    expect(wrapper.get('.sidebar-view-more').text()).toBe('View more')

    await wrapper.get('.sidebar-view-more').trigger('click')
    expect(wrapper.findAll('.sidebar-shortcut')).toHaveLength(55)
    expect(wrapper.get('.sidebar-view-more').text()).toBe('View more')

    await wrapper.get('.sidebar-view-more').trigger('click')
    expect(wrapper.findAll('.sidebar-shortcut')).toHaveLength(56)
    expect(wrapper.find('.sidebar-view-more').exists()).toBe(false)
  })

  it('keeps multiple contextual sections open', async () => {
    vi.mocked(fetch).mockImplementation(async (input) => ({
      ok: true,
      json: async () => String(input).includes('memory') ? { topics: ['Architecture'] } : [],
    } as Response))
    const wrapper = mountSidebar()

    await wrapper.get('button[aria-label="Show Notes shortcuts"]').trigger('click')
    await wrapper.get('button[aria-label="Show Memory shortcuts"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('button[aria-label="Hide Notes shortcuts"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('button[aria-label="Hide Memory shortcuts"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.text()).toContain('Architecture')
  })
})
