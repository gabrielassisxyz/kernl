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
