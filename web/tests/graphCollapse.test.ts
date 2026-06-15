import { describe, it, expect } from 'vitest'
import { collapseGraph } from '../utils/graphCollapse'

// Helpers to keep the fixtures terse.
const node = (id: string, type: string, title = id) => ({ id, title, type })
const edge = (id: string, src: string, dst: string, label: string) => ({ id, src, dst, label })

describe('collapseGraph', () => {
  it('removes a companion note describing a project and flags the project hasNote', () => {
    const nodes = [node('p1', 'project'), node('n1', 'note')]
    const edges = [edge('e1', 'n1', 'p1', 'describes')]

    const out = collapseGraph(nodes, edges)

    const ids = out.nodes.map((n) => n.id)
    expect(ids).toContain('p1')
    expect(ids).not.toContain('n1') // companion note absorbed
    const p1 = out.nodes.find((n) => n.id === 'p1')!
    expect(p1.hasNote).toBe(true)
    expect(p1.type).toBe('project') // entity keeps its own type/color/title
    expect(p1.title).toBe('p1')
  })

  it('remaps a links_to edge from the companion note so it originates from the entity', () => {
    const nodes = [node('p1', 'project'), node('n1', 'note'), node('p2', 'project')]
    const edges = [
      edge('d1', 'n1', 'p1', 'describes'),
      edge('l1', 'n1', 'p2', 'links_to'), // wikilink authored inside the note
    ]

    const out = collapseGraph(nodes, edges)

    const remapped = out.edges.find((e) => e.id === 'l1')!
    expect(remapped.src).toBe('p1') // now emanates from the entity
    expect(remapped.dst).toBe('p2')
    expect(remapped.label).toBe('links_to')
  })

  it('never renders a describes edge', () => {
    const nodes = [node('p1', 'project'), node('n1', 'note')]
    const edges = [edge('d1', 'n1', 'p1', 'describes')]

    const out = collapseGraph(nodes, edges)

    expect(out.edges).toHaveLength(0)
  })

  it('drops a self-loop produced by the merge', () => {
    // note n1 describes p1, and also links_to p1 itself -> after remap src==dst.
    const nodes = [node('p1', 'project'), node('n1', 'note')]
    const edges = [
      edge('d1', 'n1', 'p1', 'describes'),
      edge('l1', 'n1', 'p1', 'links_to'),
    ]

    const out = collapseGraph(nodes, edges)

    expect(out.edges).toHaveLength(0)
  })

  it('dedupes parallel edges created by the merge', () => {
    // Two companion notes (n1 for p1, n2 for p2) each link to the other entity.
    // A pre-existing p1->p2 links_to and n1->p2 links_to collapse to the same edge.
    const nodes = [
      node('p1', 'project'),
      node('p2', 'project'),
      node('n1', 'note'),
    ]
    const edges = [
      edge('d1', 'n1', 'p1', 'describes'),
      edge('l1', 'p1', 'p2', 'links_to'),
      edge('l2', 'n1', 'p2', 'links_to'), // collapses to p1->p2 links_to, a duplicate
    ]

    const out = collapseGraph(nodes, edges)

    const p1p2 = out.edges.filter((e) => e.src === 'p1' && e.dst === 'p2' && e.label === 'links_to')
    expect(p1p2).toHaveLength(1)
  })

  it('computes degree on the collapsed graph', () => {
    const nodes = [node('p1', 'project'), node('n1', 'note'), node('p2', 'project')]
    const edges = [
      edge('d1', 'n1', 'p1', 'describes'),
      edge('l1', 'n1', 'p2', 'links_to'), // -> p1 -> p2
    ]

    const out = collapseGraph(nodes, edges)

    const p1 = out.nodes.find((n) => n.id === 'p1')!
    const p2 = out.nodes.find((n) => n.id === 'p2')!
    expect(p1.deg).toBe(1) // describes edge does NOT count; the remapped links_to does
    expect(p2.deg).toBe(1)
  })

  it('leaves nodes with no companion note untouched (no hasNote, plain degree)', () => {
    const nodes = [node('a', 'bookmark'), node('b', 'bookmark')]
    const edges = [edge('l1', 'a', 'b', 'links_to')]

    const out = collapseGraph(nodes, edges)

    expect(out.nodes).toHaveLength(2)
    const a = out.nodes.find((n) => n.id === 'a')!
    expect(a.hasNote).toBe(false)
    expect(a.deg).toBe(1)
    expect(out.edges).toHaveLength(1)
  })

  it('keeps a parallel edge that differs only by label', () => {
    const nodes = [node('a', 'project'), node('b', 'project')]
    const edges = [
      edge('e1', 'a', 'b', 'links_to'),
      edge('e2', 'a', 'b', 'blocks'),
    ]

    const out = collapseGraph(nodes, edges)

    expect(out.edges).toHaveLength(2)
  })
})
