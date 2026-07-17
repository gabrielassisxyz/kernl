import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import InboxRow from '../components/inbox/InboxRow.vue'
import type { InboxItemData, Proposal } from '../components/inbox/InboxRow.vue'

function item(over: Partial<InboxItemData> = {}): InboxItemData {
  return { id: 'c1', type: 'whatsapp', title: 'buy milk', subtitle: 'buy milk', ...over }
}

const SPINNER_TEXT = 'Reading the capture'
const RESTING_TEXT = 'Not classified'

function row(props: { proposals: Proposal[] | null; classifying?: boolean }) {
  return mount(InboxRow, { props: { item: item(), ...props } })
}

describe('InboxRow classification state', () => {
  // The bug: an unclassified capture spun the "reading…" animation forever when
  // nothing was ever going to classify it (switch off or no LLM).
  it('rests instead of spinning when classification is not active', () => {
    const w = row({ proposals: null, classifying: false })
    expect(w.text()).toContain(RESTING_TEXT)
    expect(w.text()).not.toContain(SPINNER_TEXT)
  })

  it('shows the spinner while classification is active and no proposal has landed', () => {
    const w = row({ proposals: null, classifying: true })
    expect(w.text()).toContain(SPINNER_TEXT)
    expect(w.text()).not.toContain(RESTING_TEXT)
  })

  it('shows the proposal once it lands, regardless of the classifying flag', () => {
    const proposals: Proposal[] = [{ target: 'task', title: 'Buy milk', projectLabel: '', dueLabel: '', tags: [] }]
    const w = row({ proposals, classifying: false })
    expect(w.text()).toContain('Buy milk')
    expect(w.text()).not.toContain(SPINNER_TEXT)
    expect(w.text()).not.toContain(RESTING_TEXT)
  })
})
