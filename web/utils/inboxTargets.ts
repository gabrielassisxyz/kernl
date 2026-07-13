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

// ---- task descriptions ----

// The DA gives a fanned-out task only the fragment it owns ("plainenglish
// PDFs"). Read three weeks later that fragment means nothing, so the capture it
// came from is carried in verbatim underneath it. The capture is never rewritten
// — it is quoted whole, below a rule, and the user can edit the result before it
// is written.
const SOURCE_RULE = '---'

export function withSourceContext(body: string, captureBody: string, provenance = ''): string {
  const fragment = (body || '').trim()
  const source = (captureBody || '').trim()
  if (!source) return fragment
  if (!fragment) return source
  // The action already owns the whole capture, or the source is already quoted
  // inside it (this body has been composed once and edited since). Appending
  // again would duplicate the capture, so the body stands as it is.
  if (fragment === source || fragment.includes(source)) return fragment
  const header = provenance ? `${SOURCE_RULE}\nFrom the capture · ${provenance}` : `${SOURCE_RULE}\nFrom the capture`
  return `${fragment}\n\n${header}\n${source}`
}

/** "WHATSAPP · 4/1/26 21:08" — where a capture came from, for the source block. */
export function captureProvenance(source?: string, timestamp?: string): string {
  return [source, timestamp].map(s => (s || '').trim()).filter(Boolean).join(' · ')
}

// Only a task carries the capture into its own body. A note's body IS the
// capture already, and a bookmark's body is a URL.
export function applySourceContext(actions: CaptureAction[], captureBody: string, provenance = ''): CaptureAction[] {
  return actions.map(action =>
    action.target === 'task'
      ? { ...action, body: withSourceContext(action.body || '', captureBody, provenance) }
      : action,
  )
}

// ---- the editable draft ----

// DraftAction is one action as the inbox drawer edits it: the wire shape plus
// the textarea/input-friendly forms of its list fields.
export interface DraftAction {
  target: Target
  title: string
  body: string
  projectId: string
  projectDescription: string
  initialTasksText: string
  tagsText: string
  dueDate: string
}

const parseList = (text: string, sep: RegExp): string[] =>
  text.split(sep).map(s => s.trim()).filter(Boolean)

export function toDraft(action: CaptureAction, captureBody: string, provenance = ''): DraftAction {
  return {
    target: action.target,
    title: action.title || '',
    body: action.target === 'task'
      ? withSourceContext(action.body || '', captureBody, provenance)
      : action.body || '',
    projectId: action.projectId || action.linkTo || '',
    projectDescription: action.projectDescription || action.body || captureBody,
    initialTasksText: (action.initialTasks || []).join('\n'),
    tagsText: (action.tags || []).join(', '),
    dueDate: action.dueDate || '',
  }
}

/** A fresh row, seeded from the capture so "Add node" starts from something usable. */
export function blankDraft(captureBody: string): DraftAction {
  return {
    target: 'note',
    title: '',
    body: captureBody,
    projectId: '',
    projectDescription: captureBody,
    initialTasksText: '',
    tagsText: '',
    dueDate: '',
  }
}

// fromDraft projects a draft row back onto the wire shape, sending only the
// fields its target actually uses.
export function fromDraft(draft: DraftAction): CaptureAction {
  const { target, title, body, projectId } = draft
  const tags = parseList(draft.tagsText, /,/)
  if (target === 'update' || target === 'discard') return { target, title }
  if (target === 'project') {
    return {
      target,
      title,
      body,
      projectTitle: title,
      projectDescription: draft.projectDescription,
      initialTasks: parseList(draft.initialTasksText, /\n/).slice(0, 6),
      tags,
    }
  }
  const isTask = target === 'task'
  return {
    target,
    title,
    body,
    projectId: isTask ? projectId : '',
    linkTo: isTask ? '' : projectId,
    tags,
    // Only a task has a deadline: a date left behind by a target the user
    // changed mid-review is dropped rather than written onto a note.
    dueDate: isTask ? draft.dueDate : '',
  }
}
