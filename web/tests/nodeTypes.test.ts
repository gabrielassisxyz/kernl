import { describe, it, expect } from 'vitest'
import {
  NODE_TYPES,
  metaForType,
  colorForType,
  labelForType,
  typeForPrefix,
} from '../utils/nodeTypes'

describe('metaForType', () => {
  it('returns the registry entry for a known type', () => {
    const note = metaForType('note')
    expect(note.color).toBe('var(--color-node-note)')
    expect(note.icon).toBe('description')
    expect(note.label).toBe('note')
  })

  it('returns a fallback for an unknown type', () => {
    const meta = metaForType('totally_unknown')
    expect(meta.color).toBe('var(--color-text-muted)')
    expect(meta.icon).toBe('circle')
    expect(meta.label).toBe('totally unknown')
  })
})

describe('colorForType / labelForType', () => {
  it('reads color and label off the registry', () => {
    expect(colorForType('bookmark')).toBe('var(--color-tertiary)')
    expect(labelForType('memory_claim')).toBe('memory claim')
  })
})

describe('typeForPrefix', () => {
  it('resolves known prefixes', () => {
    expect(typeForPrefix('b')).toBe('bookmark')
    expect(typeForPrefix('p')).toBe('project')
  })

  it('returns undefined for an unknown prefix', () => {
    expect(typeForPrefix('zzz')).toBeUndefined()
  })
})

describe('NODE_TYPES', () => {
  it('has unique prefixes', () => {
    const prefixes = NODE_TYPES.map((m) => m.prefix)
    expect(new Set(prefixes).size).toBe(prefixes.length)
  })
})
