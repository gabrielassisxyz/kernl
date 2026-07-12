// Shared presentation for inbox processing targets, used by the row chip, the
// process modal, and the processed list.

export type Target = 'note' | 'update' | 'bookmark' | 'task' | 'project' | 'discard'

export const TARGETS: Target[] = ['note', 'update', 'bookmark', 'task', 'project', 'discard']

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
  update: { label: 'Update', icon: 'sync', chip: 'border-primary/40 text-primary bg-primary/10', text: 'text-primary' },
  bookmark: { label: 'Bookmark', icon: 'bookmark', chip: 'border-tertiary/40 text-tertiary bg-tertiary/10', text: 'text-tertiary' },
  task: { label: 'Task', icon: 'check_circle', chip: 'border-status-active/40 text-status-active bg-status-active/10', text: 'text-status-active' },
  project: { label: 'Project', icon: 'account_tree', chip: 'border-status-gate/40 text-tertiary bg-status-gate/10', text: 'text-tertiary' },
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

// CaptureAction is one node a capture becomes. A capture is routinely several
// things at once, so both the DA's suggestion and what the user posts back are
// a list of these. Mirrors the API's camelCase captureActionDTO.
export interface CaptureAction {
  target: Target
  title: string
  body?: string
  projectId?: string
  projectTitle?: string
  projectDescription?: string
  initialTasks?: string[]
  tags?: string[]
  /** Calendar day "YYYY-MM-DD", on a task only. Empty when none was proposed. */
  dueDate?: string
  linkTo?: string
}

/** The raw wire shape: target is any string until we validate it. */
export interface RawCaptureAction extends Omit<CaptureAction, 'target'> {
  target: string
}

// normalizeActions drops anything whose target we cannot render, so one bad
// suggestion never blanks out the whole row.
export function normalizeActions(raw: RawCaptureAction[] | undefined | null): CaptureAction[] {
  if (!raw) return []
  const out: CaptureAction[] = []
  for (const action of raw) {
    const target = normalizeTarget(action.target)
    if (!target) continue
    out.push({ ...action, target })
  }
  return out
}

// An update merges into an existing note, reviewed hunk by hunk — the backend
// rejects it alongside any other action.
export const isUpdateOnly = (actions: CaptureAction[]): boolean =>
  actions.length === 1 && actions[0].target === 'update'

export const hasConflictingUpdate = (actions: CaptureAction[]): boolean =>
  actions.length > 1 && actions.some(a => a.target === 'update')
