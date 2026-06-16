import { describe, it, expect } from 'vitest'
import { parseWikilinkQuery, buildWikilinkInsert } from '../utils/wikilinkComplete'

describe('parseWikilinkQuery', () => {
  it('is not active outside an open wikilink', () => {
    expect(parseWikilinkQuery('just some text').active).toBe(false)
    expect(parseWikilinkQuery('a single [ bracket').active).toBe(false)
  })

  it('is active with an empty query right after [[', () => {
    const r = parseWikilinkQuery('see [[')
    expect(r.active).toBe(true)
    expect(r.query).toBe('')
    expect(r.type).toBeUndefined()
  })

  it('extracts the query text typed inside the wikilink', () => {
    const r = parseWikilinkQuery('see [[lin')
    expect(r.active).toBe(true)
    expect(r.query).toBe('lin')
    expect(r.type).toBeUndefined()
  })

  it('strips a "!<letter> " type prefix and resolves it to a node type', () => {
    const r = parseWikilinkQuery('see [[!b lin')
    expect(r.active).toBe(true)
    expect(r.type).toBe('bookmark')
    expect(r.query).toBe('lin')
  })

  it('treats an unknown prefix letter as a literal query (active, no type)', () => {
    const r = parseWikilinkQuery('see [[!z lin')
    expect(r.active).toBe(true)
    expect(r.type).toBeUndefined()
    expect(r.query).toBe('!z lin')
  })

  it('is not active once the wikilink has been closed', () => {
    expect(parseWikilinkQuery('see [[done]] and more').active).toBe(false)
  })

  it('is not active across a newline (wikilinks are single-line)', () => {
    expect(parseWikilinkQuery('[[ start\nthen').active).toBe(false)
  })
})

describe('buildWikilinkInsert', () => {
  it('formats as [[<id>|<title>]]', () => {
    expect(buildWikilinkInsert({ id: 'abc-123', title: 'Linear Algebra' }))
      .toBe('[[abc-123|Linear Algebra]]')
  })
})
