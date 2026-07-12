import { describe, it, expect, vi, afterEach } from 'vitest'
import { formatTimestamp, formatRelativeTime, formatDueDate, isOverdue } from '../utils/time'

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

describe('formatDueDate', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns empty string for missing or invalid input', () => {
    expect(formatDueDate('')).toBe('')
    expect(formatDueDate(undefined)).toBe('')
    expect(formatDueDate('2026-04-02T00:00:00Z')).toBe('')
    expect(formatDueDate('tomorrow')).toBe('')
  })

  // The bug this guards: `new Date('2026-04-02')` is UTC midnight, which is
  // April 1st at 21:00 in São Paulo. A due date must render as the day it says.
  it('renders the day it was given, whatever the timezone', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-04-01T12:00:00Z'))
    expect(formatDueDate('2026-04-02')).toMatch(/2/)
    expect(formatDueDate('2026-04-02')).not.toMatch(/1/)
  })

  it('names the year when the due date is not from this one', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2027-01-10T12:00:00Z'))
    expect(formatDueDate('2026-04-02')).toMatch(/2026/)
  })
})

describe('isOverdue', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('is judged against the real today, so a stale backlog reads as late', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-12T12:00:00Z'))
    expect(isOverdue('2026-04-02')).toBe(true)
    expect(isOverdue('2026-07-12')).toBe(false) // due today is not yet late
    expect(isOverdue('2026-07-13')).toBe(false)
  })

  it('is false without a due date', () => {
    expect(isOverdue('')).toBe(false)
    expect(isOverdue(undefined)).toBe(false)
  })
})
