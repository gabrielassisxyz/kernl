import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

// The gate that decides whether a write is offered link suggestions reads the
// X-Kernl-Client header, and it excludes the web UI because finding the
// connections is part of why a person writes their own note. That exclusion is
// only real if the UI says who it is: an unidentified client is deliberately
// offered suggestions, so a write with no header lands on the wrong side of the
// gate. Asserted against the source because these calls are plain fetch inside
// a page, with no seam to mock.
describe('the notes page identifies itself to the vault write route', () => {
  const src = readFileSync(resolve(__dirname, '../pages/notes.vue'), 'utf8')

  it('sends X-Kernl-Client on every POST to the vault file route', () => {
    const writes = src.split('\n').reduce<string[]>((acc, line, i, all) => {
      if (line.includes('/api/vault/file') && all.slice(i, i + 4).some(l => l.includes("method: 'POST'"))) {
        acc.push(all.slice(i, i + 4).join('\n'))
      }
      return acc
    }, [])
    expect(writes.length).toBeGreaterThan(0)
    for (const w of writes) {
      expect(w).toContain("'X-Kernl-Client': 'ui'")
    }
  })
})
