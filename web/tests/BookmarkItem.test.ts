import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BookmarkItem from '../components/bookmarks/BookmarkItem.vue'
import type { BookmarkItemData } from '../components/bookmarks/BookmarkItem.vue'

function bookmark(over: Partial<BookmarkItemData> = {}): BookmarkItemData {
  return {
    id: 'b1',
    createdAt: '2026-01-01T00:00:00Z',
    title: 'Go generics',
    url: 'https://www.example.com/generics',
    description: 'A description',
    excerpt: 'An excerpt',
    tags: ['go', 'lang'],
    ...over,
  }
}

// GET /api/bookmarks serializes the Go Bookmark struct directly. It once shipped
// Go field names (Title, URL, Tags) because the struct had no json tags; every
// read here was PascalCase to match. These assertions pin the camelCase contract
// so a regression on either side fails loudly instead of silently rendering a
// blank row.
describe('BookmarkItem camelCase payload contract', () => {
  it('renders the title, description and domain from camelCase keys', () => {
    const w = mount(BookmarkItem, { props: { item: bookmark() } })
    expect(w.text()).toContain('Go generics')
    expect(w.text()).toContain('A description')
    expect(w.text()).toContain('example.com')
  })

  it('falls back through description then excerpt then url', () => {
    const w = mount(BookmarkItem, { props: { item: bookmark({ description: '' }) } })
    expect(w.text()).toContain('An excerpt')
  })

  it('renders tags from the camelCase key', () => {
    const w = mount(BookmarkItem, { props: { item: bookmark() } })
    expect(w.text()).toContain('go')
    expect(w.text()).toContain('lang')
  })

  it('ignores a legacy PascalCase payload instead of rendering it', () => {
    const legacy = {
      ID: 'b1',
      Title: 'Legacy Title',
      URL: 'https://legacy.example.org/x',
      Description: 'Legacy description',
      Excerpt: 'Legacy excerpt',
      Tags: ['legacytag'],
    }
    const w = mount(BookmarkItem, { props: { item: legacy as unknown as BookmarkItemData } })
    expect(w.text()).toContain('Untitled')
    expect(w.text()).not.toContain('Legacy Title')
    expect(w.text()).not.toContain('Legacy description')
    expect(w.text()).not.toContain('Legacy excerpt')
    expect(w.text()).not.toContain('legacytag')
  })
})
