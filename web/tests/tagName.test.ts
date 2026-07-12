import { describe, it, expect } from 'vitest'
import { isSystemTag, normalizeTag } from '../utils/tagName'

describe('normalizeTag', () => {
  it('lowercases and trims, so Homelab and homelab converge', () => {
    expect(normalizeTag('  Homelab ').name).toBe('homelab')
    expect(normalizeTag('homelab').name).toBe('homelab')
  })

  it('trims each segment of a nested name', () => {
    expect(normalizeTag('Homelab / NAS').name).toBe('homelab/nas')
  })

  it('rejects empty segments, which would break the nesting convention', () => {
    for (const bad of ['', '   ', '/foo', 'foo/', 'foo//bar']) {
      const { name, error } = normalizeTag(bad)
      expect(name, bad).toBeUndefined()
      expect(error, bad).toBeTruthy()
    }
  })

  it('rejects the reserved system prefix', () => {
    expect(normalizeTag('sys/pending').name).toBeUndefined()
    expect(normalizeTag('SYS/pending').error).toContain('reserved')
  })

  it('does not reject a tag that merely starts with the letters sys', () => {
    expect(normalizeTag('system-design').name).toBe('system-design')
  })
})

describe('isSystemTag', () => {
  it('is a prefix test, not a substring one', () => {
    expect(isSystemTag('sys/pending')).toBe(true)
    expect(isSystemTag('system-design')).toBe(false)
  })
})
