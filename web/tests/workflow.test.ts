import { describe, it, expect } from 'vitest'
import {
  stageBucket,
  normalizeTaskStatus,
  prettyState,
  statusTone,
  statusDotClass,
  ORCHESTRATOR_COLUMNS,
  TASK_COLUMNS,
} from '../utils/workflow'

describe('stageBucket', () => {
  it('maps the four planning sub-states to planning', () => {
    for (const s of ['ready_for_planning', 'planning', 'ready_for_plan_review', 'plan_review']) {
      expect(stageBucket(s)).toBe('planning')
    }
  })

  it('maps implementation sub-states to implementation', () => {
    for (const s of [
      'ready_for_implementation',
      'implementation',
      'ready_for_implementation_review',
      'implementation_review',
    ]) {
      expect(stageBucket(s)).toBe('implementation')
    }
  })

  it('maps integration and shipment sub-states', () => {
    expect(stageBucket('integration')).toBe('integration')
    expect(stageBucket('integration_review')).toBe('integration')
    expect(stageBucket('shipment')).toBe('shipment')
    expect(stageBucket('shipment_review')).toBe('shipment')
  })

  it('maps terminal states to done', () => {
    for (const s of ['shipped', 'deferred', 'abandoned', 'closed', 'done']) {
      expect(stageBucket(s)).toBe('done')
    }
  })

  it('maps legacy/drift states to the correct column (regression)', () => {
    expect(stageBucket('awaiting_integration')).toBe('integration')
    expect(stageBucket('awaiting_pr_review')).toBe('shipment')
    expect(stageBucket('in_progress')).toBe('implementation')
    expect(stageBucket('implementing')).toBe('implementation')
    expect(stageBucket('reviewing')).toBe('implementation')
  })

  it('falls back to planning for unknown states', () => {
    expect(stageBucket('totally_unknown_state')).toBe('planning')
    expect(stageBucket('')).toBe('planning')
  })

  it('every column id is reachable by some state', () => {
    const produced = new Set([
      stageBucket('planning'),
      stageBucket('implementation'),
      stageBucket('integration'),
      stageBucket('shipment'),
      stageBucket('shipped'),
    ])
    for (const col of ORCHESTRATOR_COLUMNS) {
      expect(produced.has(col.id)).toBe(true)
    }
  })
})

describe('normalizeTaskStatus', () => {
  it('maps terminal states to done', () => {
    for (const s of ['closed', 'deferred', 'shipped', 'done', 'abandoned']) {
      expect(normalizeTaskStatus(s)).toBe('done')
    }
  })

  it('maps blocked to blocked', () => {
    expect(normalizeTaskStatus('blocked')).toBe('blocked')
  })

  it('treats any ready_for_* state as open queued work (regression)', () => {
    expect(normalizeTaskStatus('ready_for_planning')).toBe('open')
    expect(normalizeTaskStatus('ready_for_implementation')).toBe('open')
    expect(normalizeTaskStatus('ready_for_shipment')).toBe('open')
    expect(normalizeTaskStatus('open')).toBe('open')
    expect(normalizeTaskStatus('ready')).toBe('open')
  })

  it('treats active pipeline states as in_progress', () => {
    for (const s of ['planning', 'implementation', 'integration', 'awaiting_pr_review']) {
      expect(normalizeTaskStatus(s)).toBe('in_progress')
    }
  })

  it('only produces declared task columns', () => {
    const ids = new Set(TASK_COLUMNS.map((c) => c.id))
    for (const s of ['closed', 'blocked', 'ready_for_implementation', 'implementation', 'weird']) {
      expect(ids.has(normalizeTaskStatus(s))).toBe(true)
    }
  })
})

describe('prettyState', () => {
  it('title-cases snake_case states', () => {
    expect(prettyState('ready_for_plan_review')).toBe('Ready For Plan Review')
    expect(prettyState('shipped')).toBe('Shipped')
  })

  it('returns empty string for empty input', () => {
    expect(prettyState('')).toBe('')
  })
})

describe('statusTone', () => {
  it('gate wins over everything when a human is required', () => {
    expect(statusTone({ state: 'blocked', requiresHumanAction: true })).toBe('gate')
    expect(statusTone({ state: 'shipped', requiresHumanAction: true })).toBe('gate')
  })

  it('terminal-good is passed', () => {
    expect(statusTone({ state: 'shipped' })).toBe('passed')
    expect(statusTone({ state: 'closed' })).toBe('passed')
  })

  it('failure-ish states are failed', () => {
    expect(statusTone({ state: 'abandoned' })).toBe('failed')
    expect(statusTone({ state: 'blocked' })).toBe('failed')
  })

  it('other terminal states are neutral', () => {
    expect(statusTone({ state: 'deferred' })).toBe('neutral')
  })

  it('active pipeline states are running', () => {
    expect(statusTone({ state: 'implementation' })).toBe('running')
    expect(statusTone({ state: 'planning' })).toBe('running')
  })
})

describe('statusDotClass', () => {
  it('maps each tone to a tailwind bg token', () => {
    expect(statusDotClass('gate')).toBe('bg-status-gate')
    expect(statusDotClass('failed')).toBe('bg-status-failed')
    expect(statusDotClass('passed')).toBe('bg-status-passed')
    expect(statusDotClass('running')).toBe('bg-status-running')
    expect(statusDotClass('neutral')).toBe('bg-text-faint')
  })
})
