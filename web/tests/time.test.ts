import { describe, it, expect, vi, afterEach } from 'vitest'
import { formatTimestamp, formatRelativeTime } from '../utils/time'

describe('formatTimestamp', () => {
  it('returns empty string for missing or invalid input', () => {
    expect(formatTimestamp('')).toBe('')
    expect(formatTimestamp(undefined)).toBe('')
    expect(formatTimestamp('not-a-date')).toBe('')
  })

  it('formats a valid ISO string', () => {
    expect(formatTimestamp('2026-06-14T14:30:00Z')).not.toBe('')
  })
})

describe('formatRelativeTime', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns empty string for missing or invalid input', () => {
    expect(formatRelativeTime('')).toBe('')
    expect(formatRelativeTime(undefined)).toBe('')
    expect(formatRelativeTime('nonsense')).toBe('')
  })

  it('describes recent times relatively', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-06-14T12:00:00Z'))
    expect(formatRelativeTime('2026-06-14T11:59:50Z')).toBe('just now')
    expect(formatRelativeTime('2026-06-14T11:30:00Z')).toBe('30m ago')
    expect(formatRelativeTime('2026-06-14T09:00:00Z')).toBe('3h ago')
    expect(formatRelativeTime('2026-06-12T12:00:00Z')).toBe('2d ago')
  })

  it('falls back to an absolute date past a week', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-06-14T12:00:00Z'))
    const out = formatRelativeTime('2026-05-01T12:00:00Z')
    expect(out).not.toMatch(/ago|just now/)
  })
})
