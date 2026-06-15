import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BeadCard from '../components/orchestrator/BeadCard.vue'

function bead(p: Record<string, unknown> = {}) {
  return {
    id: 'kernl-x',
    type: 'task',
    state: 'implementation',
    title: 'Do the thing',
    priority: 1,
    labels: [],
    createdAt: '',
    updatedAt: '',
    ...p,
  }
}

describe('BeadCard', () => {
  it('renders the 2px amber gate mark when a human is required', () => {
    const w = mount(BeadCard, { props: { bead: bead({ requiresHumanAction: true }) } })
    expect(w.html()).toContain('w-[2px]')
    expect(w.html()).toContain('bg-status-gate')
  })

  it('omits the gate mark when no human is required', () => {
    const w = mount(BeadCard, { props: { bead: bead() } })
    expect(w.html()).not.toContain('w-[2px]')
  })

  it('pulses the dot for a running state', () => {
    const w = mount(BeadCard, { props: { bead: bead({ state: 'implementation' }) } })
    expect(w.html()).toContain('animate-pulse')
  })

  it('does not pulse for a terminal state', () => {
    const w = mount(BeadCard, { props: { bead: bead({ state: 'shipped' }) } })
    expect(w.html()).not.toContain('animate-pulse')
  })

  it('shows the bead id and a human-readable state', () => {
    const w = mount(BeadCard, { props: { bead: bead({ id: 'kernl-abc', state: 'plan_review' }) } })
    expect(w.text()).toContain('kernl-abc')
    expect(w.text()).toContain('Plan Review')
  })

  it('emits open with the bead on click', async () => {
    const b = bead()
    const w = mount(BeadCard, { props: { bead: b } })
    await w.trigger('click')
    expect(w.emitted('open')?.[0]).toEqual([b])
  })
})
