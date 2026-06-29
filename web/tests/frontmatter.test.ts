import { describe, expect, it } from 'vitest'
import {
  inferPropertyType,
  replaceFrontmatter,
  splitFrontmatter,
} from '../utils/frontmatter'

describe('frontmatter utilities', () => {
  it('parses typed YAML values from a note', () => {
    const note = '---\ntitle: Typed note\ntags:\n  - telos\n  - da\nreviewed: 2026-06-28\n---\n\n# Body\n'
    const split = splitFrontmatter(note)

    expect(split.data.title).toBe('Typed note')
    expect(split.data.tags).toEqual(['telos', 'da'])
    expect(split.data.reviewed).toBe('2026-06-28')
    expect(split.body).toBe('\n# Body\n')
  })

  it('replaces frontmatter without changing the body', () => {
    const note = '---\ntitle: Old\ntags: []\n---\n\n# Old\nBody\n'
    const next = replaceFrontmatter(note, {
      title: 'New',
      tags: ['telos'],
      reviewed: '2026-06-28',
    })

    expect(next).toContain('title: New')
    expect(next).toContain('- telos')
    expect(next).toContain('reviewed: 2026-06-28')
    expect(next.endsWith('# Old\nBody\n')).toBe(true)
  })

  it('adds a frontmatter block to a note that does not have one', () => {
    const next = replaceFrontmatter('# Body\n', { title: 'Body', tags: [] })

    expect(next.startsWith('---\ntitle: Body\ntags: []\n---\n\n# Body\n')).toBe(true)
  })

  it('infers property controls from actual values', () => {
    expect(inferPropertyType('tags', [])).toBe('list')
    expect(inferPropertyType('reviewed', '2026-06-28')).toBe('date')
    expect(inferPropertyType('title', 'A note')).toBe('text')
  })
})
