import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import IngestItem from '../components/ingest/IngestItem.vue'
import type { IngestReviewData } from '../components/ingest/IngestItem.vue'

function item(over: Partial<IngestReviewData> = {}): IngestReviewData {
  return {
    id: 'r1',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    title: 'Meeting notes',
    sourceNodeId: 'n42',
    action: 'Create Page',
    payload: 'body',
    contentHash: 'deadbeef',
    tags: [],
    ...over,
  }
}

// GET /api/ingest/queue serializes the Go IngestReview struct directly. It once
// shipped Go field names (Action, Title, SourceNodeID) because the struct had no
// json tags; every read here was PascalCase to match. These assertions pin the
// camelCase contract so a regression on either side fails loudly instead of
// silently rendering blank badges.
describe('IngestItem camelCase payload contract', () => {
  it('renders the action, title and source node id from camelCase keys', () => {
    const w = mount(IngestItem, { props: { item: item() } })
    expect(w.text()).toContain('Create Page')
    expect(w.text()).toContain('Meeting notes')
    expect(w.text()).toContain('n42')
  })

  it('falls back to REVIEW when the action is empty', () => {
    const w = mount(IngestItem, { props: { item: item({ action: '' }) } })
    expect(w.text()).toContain('REVIEW')
  })

  it('ignores a legacy PascalCase payload instead of rendering it', () => {
    const legacy = { Action: 'Update', Title: 'Legacy', SourceNodeID: 'n99' }
    const w = mount(IngestItem, { props: { item: legacy as unknown as IngestReviewData } })
    expect(w.text()).toContain('REVIEW')
    expect(w.text()).not.toContain('Legacy')
    expect(w.text()).not.toContain('n99')
  })
})
