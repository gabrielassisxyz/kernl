// Shared presentation for inbox processing targets, used by the row chip, the
// process modal, and the processed list.

export type Target = 'note' | 'bookmark' | 'task' | 'discard'

export const TARGETS: Target[] = ['note', 'bookmark', 'task', 'discard']

interface TargetMeta {
  label: string
  icon: string
  /** chip classes: border + text + faint bg */
  chip: string
  /** standalone text colour */
  text: string
}

export const TARGET_META: Record<Target, TargetMeta> = {
  note: { label: 'Note', icon: 'description', chip: 'border-da-accent/40 text-da-accent-text bg-da-accent/10', text: 'text-da-accent-text' },
  bookmark: { label: 'Bookmark', icon: 'bookmark', chip: 'border-tertiary/40 text-tertiary bg-tertiary/10', text: 'text-tertiary' },
  task: { label: 'Task', icon: 'check_circle', chip: 'border-status-active/40 text-status-active bg-status-active/10', text: 'text-status-active' },
  discard: { label: 'Discard', icon: 'delete', chip: 'border-status-failed/40 text-status-failed-text bg-status-failed/10', text: 'text-status-failed-text' },
}

// normalizeTarget maps a raw suggestedAction onto a clean Target. The classifier
// now emits clean names, but captures classified before the Phase 1 rework still
// carry the legacy "convert_to_*" form — strip it so old rows render correctly.
export function normalizeTarget(raw: string | undefined | null): Target | null {
  if (!raw) return null
  const s = raw.startsWith('convert_to_') ? raw.slice('convert_to_'.length) : raw
  return (TARGETS as string[]).includes(s) ? (s as Target) : null
}
