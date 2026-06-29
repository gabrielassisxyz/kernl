import { describe, expect, it } from 'vitest'
import { frontmatterConcealRange } from '../utils/frontmatterConceal'

describe('frontmatterConcealRange', () => {
  it('covers the whole frontmatter block including the trailing newline', () => {
    const doc = '---\ntitle: Note\ntags: []\n---\n\n# Body\n'
    const range = frontmatterConcealRange(doc)
    expect(range).not.toBeNull()
    expect(range?.from).toBe(0)
    // Concealed range ends at the start of the body, leaving '\n# Body\n'.
    expect(doc.slice(range!.to)).toBe('\n# Body\n')
  })

  it('returns null when there is no frontmatter', () => {
    expect(frontmatterConcealRange('# Just a heading\n')).toBeNull()
    expect(frontmatterConcealRange('plain text')).toBeNull()
  })

  it('returns null when the closing delimiter is missing', () => {
    expect(frontmatterConcealRange('---\ntitle: Note\n# no close')).toBeNull()
  })

  it('handles CRLF line endings', () => {
    const doc = '---\r\ntitle: Note\r\n---\r\nBody'
    const range = frontmatterConcealRange(doc)
    expect(range).not.toBeNull()
    expect(doc.slice(range!.to)).toBe('Body')
  })
})
