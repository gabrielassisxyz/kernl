// Shared date formatting for the bead surfaces. Two registers: a compact
// absolute stamp for dense tables, and a relative "Nm ago" for cards.

// "Jun 5, 14:30" — compact absolute stamp. Empty string for missing/invalid.
export function formatTimestamp(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// --- Due dates ---
//
// A due date is a calendar day ("2026-04-02"), not an instant. `new Date(day)`
// would read it as UTC midnight and render the day BEFORE in every timezone
// west of London — so the parts are read by hand and the date is built in local
// time. Nothing here may go through Date(string).

const DAY = /^(\d{4})-(\d{2})-(\d{2})$/

function parseDay(day?: string): Date | null {
  const m = day ? DAY.exec(day) : null
  if (!m) return null
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
}

// "Apr 2" — with the year when it is not the current one, because a backlog
// months deep makes "Apr 2" ambiguous. Empty string for missing/invalid.
export function formatDueDate(day?: string): string {
  const d = parseDay(day)
  if (!d) return ''
  const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' }
  if (d.getFullYear() !== new Date().getFullYear()) opts.year = 'numeric'
  return d.toLocaleDateString([], opts)
}

// True when the day is strictly before today. Unlike the due date itself, this
// is judged against the real now: a deadline from April IS late in July.
export function isOverdue(day?: string): boolean {
  const d = parseDay(day)
  if (!d) return false
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  return d.getTime() < today.getTime()
}

// "just now" / "5m ago" / "3h ago" / "2d ago", falling back to an absolute
// date past a week. Empty string for missing/invalid.
export function formatRelativeTime(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const m = Math.round((Date.now() - d.getTime()) / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  if (h < 24) return `${h}h ago`
  const days = Math.round(h / 24)
  if (days < 7) return `${days}d ago`
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' })
}
