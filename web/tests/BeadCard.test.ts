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
  it('renders the gate tint when a human is required', () => {
    const w = mount(BeadCard, { props: { bead: bead({ requiresHumanAction: true }) } })
    expect(w.classes()).toContain('bg-status-gate/10')
    expect(w.classes()).toContain('hover:bg-status-gate/20')
    expect(w.html()).toContain('bg-status-gate')
  })

  it('omits the gate tint when no human is required', () => {
    const w = mount(BeadCard, { props: { bead: bead() } })
    expect(w.classes()).not.toContain('bg-status-gate/10')
    expect(w.classes()).not.toContain('hover:bg-status-gate/20')
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
