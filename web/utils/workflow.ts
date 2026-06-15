// Shared workflow/status helpers for the Orchestrator, Tasks and Projects
// surfaces. Maps the 18 canonical workflow states (autopilot_with_pr,
// internal/backend/workflows/canonical.yaml) onto the legible buckets the UI
// renders. Defensive: unknown states fall back sensibly rather than vanishing.

export type StageBucket =
  | 'planning'
  | 'implementation'
  | 'integration'
  | 'shipment'
  | 'done'

export interface OrchestratorColumn {
  id: StageBucket
  label: string
}

// Ordered left-to-right Kanban columns for the Orchestrator pipeline.
export const ORCHESTRATOR_COLUMNS: OrchestratorColumn[] = [
  { id: 'planning', label: 'Planning' },
  { id: 'implementation', label: 'Implementation' },
  { id: 'integration', label: 'Integration' },
  { id: 'shipment', label: 'Shipment' },
  { id: 'done', label: 'Shipped / Closed' },
]

const STAGE_BY_STATE: Record<string, StageBucket> = {
  ready_for_planning: 'planning',
  planning: 'planning',
  ready_for_plan_review: 'planning',
  plan_review: 'planning',
  ready_for_implementation: 'implementation',
  implementation: 'implementation',
  ready_for_implementation_review: 'implementation',
  implementation_review: 'implementation',
  ready_for_integration: 'integration',
  integration: 'integration',
  ready_for_integration_review: 'integration',
  integration_review: 'integration',
  ready_for_shipment: 'shipment',
  shipment: 'shipment',
  ready_for_shipment_review: 'shipment',
  shipment_review: 'shipment',
  shipped: 'done',
  deferred: 'done',
  abandoned: 'done',
  closed: 'done',
  done: 'done',
  // Legacy / drift states still present in live data (see bd-status-drift).
  in_progress: 'implementation',
  implementing: 'implementation',
  implemented: 'implementation',
  reviewing: 'implementation',
  awaiting_review: 'implementation',
  awaiting_integration: 'integration',
  awaiting_pr_review: 'shipment',
}

export function stageBucket(state: string): StageBucket {
  return STAGE_BY_STATE[state] ?? 'planning'
}

export type TaskStatus = 'open' | 'in_progress' | 'blocked' | 'done'

export interface TaskColumn {
  id: TaskStatus
  label: string
}

export const TASK_COLUMNS: TaskColumn[] = [
  { id: 'open', label: 'Open' },
  { id: 'in_progress', label: 'In Progress' },
  { id: 'blocked', label: 'Blocked' },
  { id: 'done', label: 'Done' },
]

const DONE_STATES = new Set(['shipped', 'closed', 'done', 'abandoned', 'deferred'])
const BLOCKED_STATES = new Set(['blocked'])

/**
 * Collapse any bead state (workflow state or plain bd status) into the four
 * Tasks Kanban buckets. Anything mid-pipeline counts as in_progress; anything
 * not yet started (open / ready) counts as open.
 */
export function normalizeTaskStatus(state: string): TaskStatus {
  if (DONE_STATES.has(state)) return 'done'
  if (BLOCKED_STATES.has(state)) return 'blocked'
  // Any "ready_for_*" state is queued work that hasn't started yet → Open.
  if (state.startsWith('ready_for_') || state === 'open' || state === 'ready') {
    return 'open'
  }
  return 'in_progress'
}

// Human-readable label for a raw state literal: snake_case → Title Case.
export function prettyState(state: string): string {
  if (!state) return ''
  return state
    .split('_')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ')
}

export type StatusTone = 'running' | 'gate' | 'failed' | 'passed' | 'neutral'

/**
 * Map a bead to a status tone for its 6px dot. Gate (amber) wins when the bead
 * needs a human; terminal-good is sage; failed/abandoned is brick; an active
 * pipeline bead is the neutral running slate.
 */
export function statusTone(bead: {
  state: string
  requiresHumanAction?: boolean
}): StatusTone {
  if (bead.requiresHumanAction) return 'gate'
  if (bead.state === 'shipped' || bead.state === 'closed' || bead.state === 'done') {
    return 'passed'
  }
  if (bead.state === 'abandoned' || bead.state === 'failed' || bead.state === 'blocked') {
    return 'failed'
  }
  if (DONE_STATES.has(bead.state)) return 'neutral'
  return 'running'
}

// Tailwind bg token (see web/assets/css/tailwind.css) for a status dot tone.
export function statusDotClass(tone: StatusTone): string {
  switch (tone) {
    case 'gate':
      return 'bg-status-gate'
    case 'failed':
      return 'bg-status-failed'
    case 'passed':
      return 'bg-status-passed'
    case 'running':
      return 'bg-status-running'
    default:
      return 'bg-text-faint'
  }
}
