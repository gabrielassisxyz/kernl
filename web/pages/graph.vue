<template>
  <div class="absolute inset-0 flex bg-bg-base text-text-primary overflow-hidden select-none">
    <div class="flex-1 min-w-0 relative" ref="wrapRef">
      <div class="pointer-events-none absolute inset-0 graph-grid opacity-[0.35]"></div>

      <header class="absolute top-0 left-0 right-0 z-dropdown flex items-center gap-section px-margin py-component pointer-events-none">
        <div class="flex flex-col gap-1 pointer-events-auto">
          <h1 class="font-display text-display tracking-tight leading-none">Constellation</h1>
          <p class="font-mono-data text-mono-data text-text-muted">
            <span class="text-text-primary">{{ visibleNodeCount }}</span> nodes
            <span class="mx-1 text-text-faint">·</span>
            <span class="text-text-primary">{{ visibleLinkCount }}</span> connections
            <span v-if="orphanCount" class="ml-2 text-status-gate">{{ orphanCount }} orphaned</span>
          </p>
        </div>

        <div class="ml-auto flex items-center gap-tight pointer-events-auto">
          <div class="relative">
            <span class="material-symbols-outlined absolute left-2 top-1/2 -translate-y-1/2 text-[16px] text-text-faint">search</span>
            <input
              v-model="query"
              @keydown.enter="focusSearch"
              @keydown.esc="query = ''"
              aria-label="Find a node"
              placeholder="find a node…"
              class="w-52 bg-surface border border-border-hairline rounded pl-8 pr-3 py-1.5 text-body text-text-primary placeholder:text-text-muted focus:outline-none focus:border-da-accent transition-colors"
            />
          </div>
          <button @click="fitView" title="Fit to view" aria-label="Fit to view" class="ctrl-btn"><span class="material-symbols-outlined text-[18px]">fit_screen</span></button>
          <button @click="reload" title="Reload graph" aria-label="Reload graph" class="ctrl-btn"><span class="material-symbols-outlined text-[18px]" :class="loading && 'animate-spin'">{{ loading ? 'progress_activity' : 'refresh' }}</span></button>
        </div>
      </header>

      <div class="absolute bottom-0 left-0 z-dropdown px-margin py-component flex flex-wrap items-center gap-x-component gap-y-tight max-w-[70%]">
        <button
          v-for="t in legend"
          :key="t.type"
          @click="toggleType(t.type)"
          class="legend-btn group flex items-center gap-tight font-mono-data text-mono-data transition-opacity"
          :class="hiddenTypes.has(t.type) ? 'opacity-35' : 'opacity-100'"
        >
          <span class="w-2.5 h-2.5 rounded-full transition-transform group-hover:scale-125" :style="{ background: t.color }"></span>
          <span class="text-text-muted group-hover:text-text-primary">{{ label(t.type) }}</span>
          <span class="text-text-faint">{{ t.count }}</span>
        </button>
      </div>

      <svg
        ref="svgRef"
        class="absolute inset-0 w-full h-full"
        :class="dragNode ? 'cursor-grabbing' : 'cursor-grab'"
        @pointerdown="startPan"
        @wheel.prevent="onWheel"
      >
        <g :transform="`translate(${panX},${panY}) scale(${scale})`">
          <g :stroke-width="1.6 / scale" fill="none">
            <line
              v-for="l in displayLinks"
              :key="l.id"
              :ref="el => setLinkRef(el, l.id)"
              :x1="l.s.x" :y1="l.s.y" :x2="l.t.x" :y2="l.t.y"
              :stroke="l.active ? colorFor(l.s.type) : 'var(--color-outline-variant)'"
              :stroke-opacity="dim(l.active, l.faded)"
              :style="{ transition: 'stroke-opacity 120ms' }"
            />
          </g>

          <g>
            <g
              v-for="n in displayNodes"
              :key="n.id"
              :ref="el => setNodeRef(el, n.id)"
              :transform="`translate(${n.x},${n.y})`"
              :opacity="dim(n.active, n.faded)"
              class="cursor-pointer"
              style="transition: opacity 120ms"
              @pointerdown.stop="startDrag(n, $event)"
              @pointerenter="hoverId = n.id"
              @pointerleave="hoverId = ''"
              @click.stop="selectedId = n.id"
            >
              <circle :r="n.r * 2.6" :fill="colorFor(n.type)" :opacity="(selectedId === n.id || n.id === hoverId) ? 0.32 : 0.16" :style="{ transition: 'opacity 150ms' }" />
              <circle
                v-if="n.hasNote"
                :r="n.r + 4 / scale + n.r * 0.28"
                fill="none"
                :stroke="colorFor(n.type)"
                :stroke-width="0.9 / scale"
                :stroke-dasharray="`${2.2 / scale} ${2.6 / scale}`"
                :opacity="(selectedId === n.id || n.id === hoverId) ? 0.85 : 0.5"
                :style="{ transition: 'opacity 150ms' }"
              />
              <circle
                :r="n.r"
                :fill="colorFor(n.type)"
                :stroke="selectedId === n.id ? 'var(--color-on-surface)' : 'var(--color-bg-base)'"
                :stroke-width="(selectedId === n.id ? 2.5 : 1) / scale"
              />
              <circle :cx="-n.r * 0.32" :cy="-n.r * 0.32" :r="n.r * 0.42" fill="var(--color-on-surface)" :opacity="0.35" />
            </g>
          </g>
        </g>
      </svg>

      <div
        v-if="hoverNode && !dragNode"
        class="pointer-events-none absolute z-tooltip max-w-xs"
        :style="{ left: tip.x + 'px', top: tip.y + 'px' }"
      >
        <div class="-translate-x-1/2 -translate-y-[calc(100%+14px)] bg-surface border border-border-default rounded px-3 py-2">
          <div class="flex items-center gap-tight mb-0.5">
            <span class="w-2 h-2 rounded-full" :style="{ background: colorFor(hoverNode.type) }"></span>
            <span class="font-label-caps text-label-caps text-text-muted">{{ label(hoverNode.type) }}</span>
          </div>
          <div class="text-body text-text-primary leading-snug max-w-[240px] truncate">{{ hoverNode.title || '(untitled)' }}</div>
          <div class="font-mono-data text-mono-data text-text-faint mt-0.5">{{ hoverNode.deg }} connection{{ hoverNode.deg === 1 ? '' : 's' }}</div>
        </div>
      </div>

      <div v-if="loading" class="absolute inset-0 flex items-center justify-center font-mono-data text-mono-data text-text-muted">
        <span class="material-symbols-outlined animate-spin mr-2">progress_activity</span> Mapping…
      </div>
      <div v-else-if="error" class="absolute inset-0 flex items-center justify-center font-mono-data text-mono-data text-status-failed-text">
        {{ error }}
      </div>
      <div v-else-if="!simNodes.length" class="absolute inset-0 flex flex-col items-center justify-center gap-tight text-text-muted">
        <span class="material-symbols-outlined text-[40px] text-text-faint">hub</span>
        <span class="text-body">No nodes in the graph yet.</span>
      </div>
    </div>

    <Transition name="panel">
      <aside
        v-if="selectedNode"
        class="w-full max-w-[340px] shrink-0 h-full border-l border-border-hairline bg-surface flex flex-col z-dropdown"
      >
        <div class="flex items-start gap-component px-component py-component border-b border-border-hairline">
          <span class="mt-1 w-3 h-3 rounded-full shrink-0" :style="{ background: colorFor(selectedNode.type) }"></span>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-tight">
              <span class="font-label-caps text-label-caps text-text-muted">{{ label(selectedNode.type) }}</span>
              <span
                v-if="selectedNode.hasNote"
                title="has an associated note"
                class="inline-flex items-center gap-0.5 font-label-caps text-label-caps text-text-faint border border-border-hairline rounded px-1.5 leading-[1.4]"
              >
                <span class="material-symbols-outlined text-[12px]">description</span>noted
              </span>
            </div>
            <h2 class="text-headline text-text-primary leading-snug mt-0.5 break-words">{{ selectedNode.title || '(untitled)' }}</h2>
            <code class="block font-mono-data text-mono-data text-text-faint mt-1 truncate">{{ selectedNode.id }}</code>
          </div>
          <button @click="selectedId = ''" aria-label="Close" class="ctrl-btn shrink-0"><span class="material-symbols-outlined text-[18px]">close</span></button>
        </div>

        <div class="px-component py-component grid grid-cols-2 gap-tight border-b border-border-hairline">
          <div class="stat">
            <span class="stat-num">{{ outgoing.length }}</span>
            <span class="stat-lbl">outgoing</span>
          </div>
          <div class="stat">
            <span class="stat-num">{{ incoming.length }}</span>
            <span class="stat-lbl">incoming</span>
          </div>
        </div>

        <div class="flex-1 overflow-y-auto px-component py-component flex flex-col gap-section">
          <section v-if="outgoing.length">
            <h3 class="conn-head">Outgoing</h3>
            <ul class="flex flex-col gap-tight">
              <li v-for="c in outgoing" :key="c.edgeId">
                <button @click="selectedId = c.node.id" class="conn-row group">
                  <span class="conn-label">{{ c.label }}</span>
                  <span class="material-symbols-outlined text-[14px] text-text-faint">arrow_forward</span>
                  <span class="w-2 h-2 rounded-full shrink-0" :style="{ background: colorFor(c.node.type) }"></span>
                  <span class="conn-title">{{ c.node.title || '(untitled)' }}</span>
                </button>
              </li>
            </ul>
          </section>

          <section v-if="incoming.length">
            <h3 class="conn-head">Incoming</h3>
            <ul class="flex flex-col gap-tight">
              <li v-for="c in incoming" :key="c.edgeId">
                <button @click="selectedId = c.node.id" class="conn-row group">
                  <span class="w-2 h-2 rounded-full shrink-0" :style="{ background: colorFor(c.node.type) }"></span>
                  <span class="conn-title">{{ c.node.title || '(untitled)' }}</span>
                  <span class="material-symbols-outlined text-[14px] text-text-faint">arrow_forward</span>
                  <span class="conn-label">{{ c.label }}</span>
                </button>
              </li>
            </ul>
          </section>

          <div v-if="!outgoing.length && !incoming.length" class="text-body text-text-muted flex items-center gap-tight">
            <span class="material-symbols-outlined text-[18px] text-status-gate">link_off</span>
            No connections — this node is orphaned.
          </div>
        </div>
      </aside>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { colorForType, labelForType } from '~/utils/nodeTypes'
import { collapseGraph } from '~/utils/graphCollapse'

// ---- type → color/label sourced from the shared node-type registry ----
const colorFor = colorForType
const label = labelForType

// ---- state ----
const wrapRef = ref(null)
const svgRef = ref(null)
const loading = ref(true)
const error = ref('')
const hoverId = ref('')
const selectedId = ref('')
const query = ref('')
const hiddenTypes = reactive(new Set())

const panX = ref(0)
const panY = ref(0)
const scale = ref(1)
const tip = reactive({ x: 0, y: 0 })

const nodeEls = new Map()
const linkEls = new Map()
const setNodeRef = (el, id) => { if (el) nodeEls.set(id, el); else nodeEls.delete(id) }
const setLinkRef = (el, id) => { if (el) linkEls.set(id, el); else linkEls.delete(id) }

// plain (non-reactive) simulation arrays for cheap per-frame mutation
let simNodesRaw = []
let simLinksRaw = []
const simNodes = ref([]) // identity-stable mirror used for reactive counts
const byId = new Map()

// ---- rendering (depends on `frame` so it re-evaluates each tick) ----
const activeId = computed(() => hoverId.value || selectedId.value)

const neighborSet = computed(() => {
  const set = new Set()
  const id = activeId.value
  if (!id) return set
  set.add(id)
  for (const l of simLinksRaw) {
    if (l.s.id === id) set.add(l.t.id)
    if (l.t.id === id) set.add(l.s.id)
  }
  return set
})

const displayNodes = computed(() => {
  const id = activeId.value
  const nb = neighborSet.value
  return simNodesRaw
    .filter((n) => !hiddenTypes.has(n.type))
    .map((n) => ({
      ...n,
      active: !id || nb.has(n.id),
      faded: !!id && !nb.has(n.id),
    }))
})

const displayLinks = computed(() => {
  const id = activeId.value
  return simLinksRaw
    .filter((l) => !hiddenTypes.has(l.s.type) && !hiddenTypes.has(l.t.type))
    .map((l) => {
      const active = !!id && (l.s.id === id || l.t.id === id)
      return { id: l.id, s: l.s, t: l.t, active, faded: !!id && !active }
    })
})

// Connection legibility: keep edges readable at rest and only de-emphasize
// (never near-hide) non-neighbours when a node is focused, so the user can still
// trace the wider structure. (graph canary, U1)
const dim = (active, faded) => (faded ? 0.22 : active ? 1 : 0.85)

const visibleNodeCount = computed(() => simNodes.value.filter((n) => !hiddenTypes.has(n.type)).length)
const visibleLinkCount = computed(() => displayLinks.value.length)
const orphanCount = computed(() => simNodes.value.filter((n) => n.deg === 0).length)

const legend = computed(() => {
  const counts = {}
  for (const n of simNodes.value) counts[n.type] = (counts[n.type] || 0) + 1
  return Object.entries(counts)
    .sort((a, b) => b[1] - a[1])
    .map(([type, count]) => ({ type, count, color: colorFor(type) }))
})

const hoverNode = computed(() => byId.get(hoverId.value) || null)
const selectedNode = computed(() => byId.get(selectedId.value) || null)

const outgoing = computed(() => connectionsFor(selectedId.value, 'out'))
const incoming = computed(() => connectionsFor(selectedId.value, 'in'))
function connectionsFor(id, dir) {
  if (!id) return []
  const out = []
  for (const l of simLinksRaw) {
    if (dir === 'out' && l.s.id === id) out.push({ edgeId: l.id, label: l.label, node: l.t })
    if (dir === 'in' && l.t.id === id) out.push({ edgeId: l.id, label: l.label, node: l.s })
  }
  return out
}

const toggleType = (t) => (hiddenTypes.has(t) ? hiddenTypes.delete(t) : hiddenTypes.add(t))

// keep tooltip glued to the hovered node in screen space
watch([hoverId, panX, panY, scale], () => {
  const n = byId.get(hoverId.value)
  if (!n) return
  tip.x = panX.value + n.x * scale.value
  tip.y = panY.value + n.y * scale.value
})

// ---- data ----
async function load() {
  loading.value = true
  error.value = ''
  try {
    const [nRes, eRes] = await Promise.all([fetch('/api/nodes'), fetch('/api/edges')])
    if (!nRes.ok) throw new Error(`nodes: ${nRes.status}`)
    if (!eRes.ok) throw new Error(`edges: ${eRes.status}`)
    const rawNodes = (await nRes.json()) || []
    const rawEdges = (await eRes.json()) || []

    // Collapse companion notes into the entity they describe, BEFORE building the
    // sim. Surviving nodes carry collapsed `deg` + `hasNote`; edges are remapped,
    // self-loops/duplicates dropped, `describes` edges removed.
    const collapsed = collapseGraph(rawNodes, rawEdges)

    const w = wrapRef.value?.clientWidth || 960
    const h = wrapRef.value?.clientHeight || 720
    byId.clear()
    simNodesRaw = collapsed.nodes.map((n) => {
      const node = {
        id: n.id,
        title: n.title,
        type: n.type,
        deg: n.deg,
        hasNote: n.hasNote,
        r: Math.min(6.5 + Math.sqrt(n.deg) * 3, 24),
        x: w / 2 + (Math.random() - 0.5) * 220,
        y: h / 2 + (Math.random() - 0.5) * 220,
        vx: 0,
        vy: 0,
      }
      byId.set(n.id, node)
      return node
    })
    simLinksRaw = []
    for (const e of collapsed.edges) {
      const s = byId.get(e.src)
      const t = byId.get(e.dst)
      if (!s || !t) continue // defensive: skip dangling
      simLinksRaw.push({ id: e.id, label: e.label, s, t })
    }
    simNodes.value = simNodesRaw
    loading.value = false
    await nextTick()
    runSim(w, h)
  } catch (e) {
    error.value = String(e?.message || e)
    loading.value = false
  }
}

// ---- force simulation (charge + spring + gravity, cooling) ----
let raf = 0
let didFit = false
function runSim(w, h) {
  cancelAnimationFrame(raf)
  let alpha = 1
  const cx = w / 2
  const cy = h / 2
  didFit = false

  const syncDOM = () => {
    for (const n of simNodesRaw) {
      const el = nodeEls.get(n.id)
      if (el) el.setAttribute('transform', `translate(${n.x},${n.y})`)
    }
    for (const l of simLinksRaw) {
      const el = linkEls.get(l.id)
      if (el) {
        el.setAttribute('x1', l.s.x)
        el.setAttribute('y1', l.s.y)
        el.setAttribute('x2', l.t.x)
        el.setAttribute('y2', l.t.y)
      }
    }
    if (hoverId.value) {
      const hn = byId.get(hoverId.value)
      if (hn) {
        tip.x = panX.value + hn.x * scale.value
        tip.y = panY.value + hn.y * scale.value
      }
    }
  }

  // Pure physics step — advances node positions by one tick; no DOM access.
  const step = () => {
    const N = simNodesRaw.length
    // charge: pairwise repulsion (O(n²); fine for personal-vault scale)
    for (let i = 0; i < N; i++) {
      const a = simNodesRaw[i]
      for (let j = i + 1; j < N; j++) {
        const b = simNodesRaw[j]
        let dx = a.x - b.x
        let dy = a.y - b.y
        let d2 = dx * dx + dy * dy
        if (d2 < 0.01) { dx = Math.random() - 0.5; dy = Math.random() - 0.5; d2 = 1 }
        const f = (1200 * alpha) / d2
        const fx = dx * f
        const fy = dy * f
        a.vx += fx; a.vy += fy
        b.vx -= fx; b.vy -= fy
      }
      // gravity toward center keeps disconnected components clustered on screen;
      // orphans (no links) pull harder so they don't drift to the edges
      const g = (a.deg === 0 ? 0.09 : 0.06) * alpha
      a.vx += (cx - a.x) * g
      a.vy += (cy - a.y) * g
    }
    // spring on links
    for (const l of simLinksRaw) {
      const dx = l.t.x - l.s.x
      const dy = l.t.y - l.s.y
      const dist = Math.hypot(dx, dy) || 0.01
      const target = 52
      const f = ((dist - target) / dist) * 0.5 * alpha
      const fx = dx * f
      const fy = dy * f
      l.s.vx += fx; l.s.vy += fy
      l.t.vx -= fx; l.t.vy -= fy
    }
    // integrate + friction
    for (const n of simNodesRaw) {
      if (n === dragNode.value) { n.vx = 0; n.vy = 0; continue }
      n.vx *= 0.82
      n.vy *= 0.82
      n.x += n.vx
      n.y += n.vy
    }
    alpha *= 0.99
  }

  const tick = () => {
    step()
    syncDOM()
    if (!didFit && alpha < 0.3) { didFit = true; fitView() }
    if (alpha > 0.01) {
      raf = requestAnimationFrame(tick)
    } else {
      fitView() // final fit once the constellation has settled
    }
  }

  // When the user prefers reduced motion the rAF loop is skipped entirely.
  // Instead the physics runs to completion synchronously and positions are
  // written to the DOM once, eliminating all per-frame animation.
  const prefersReducedMotion = typeof window !== 'undefined' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (prefersReducedMotion) {
    while (alpha > 0.01) step()
    syncDOM()
    fitView()
  } else {
    raf = requestAnimationFrame(tick)
  }
}
function reheat() {
  const w = wrapRef.value?.clientWidth || 960
  const h = wrapRef.value?.clientHeight || 720
  runSim(w, h)
}

// ---- view: fit / pan / zoom / drag ----
function fitView() {
  if (!simNodesRaw.length) return
  const vis = simNodesRaw.filter((n) => !hiddenTypes.has(n.type))
  const pts = vis.length ? vis : simNodesRaw
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  for (const n of pts) {
    minX = Math.min(minX, n.x); maxX = Math.max(maxX, n.x)
    minY = Math.min(minY, n.y); maxY = Math.max(maxY, n.y)
  }
  const w = wrapRef.value?.clientWidth || 960
  const h = wrapRef.value?.clientHeight || 720
  const pad = 64
  const spanX = Math.max(maxX - minX, 1)
  const spanY = Math.max(maxY - minY, 1)
  const k = Math.min((w - pad * 2) / spanX, (h - pad * 2) / spanY, 3)
  scale.value = Math.max(k, 0.2)
  panX.value = w / 2 - ((minX + maxX) / 2) * scale.value
  panY.value = h / 2 - ((minY + maxY) / 2) * scale.value
}

function screenToWorld(clientX, clientY) {
  const r = svgRef.value.getBoundingClientRect()
  return {
    x: (clientX - r.left - panX.value) / scale.value,
    y: (clientY - r.top - panY.value) / scale.value,
  }
}

function onWheel(e) {
  const r = svgRef.value.getBoundingClientRect()
  const sx = e.clientX - r.left
  const sy = e.clientY - r.top
  const wx = (sx - panX.value) / scale.value
  const wy = (sy - panY.value) / scale.value
  const next = Math.min(4, Math.max(0.2, scale.value * (e.deltaY < 0 ? 1.12 : 0.89)))
  scale.value = next
  panX.value = sx - wx * next
  panY.value = sy - wy * next
}

// panning the canvas
let panStart = null
function startPan(e) {
  panStart = { x: e.clientX, y: e.clientY, px: panX.value, py: panY.value }
  window.addEventListener('pointermove', onPan)
  window.addEventListener('pointerup', endPan)
}
function onPan(e) {
  if (!panStart) return
  panX.value = panStart.px + (e.clientX - panStart.x)
  panY.value = panStart.py + (e.clientY - panStart.y)
}
function endPan() {
  panStart = null
  window.removeEventListener('pointermove', onPan)
  window.removeEventListener('pointerup', endPan)
}

// dragging a node
const dragNode = ref(null)
function startDrag(n, e) {
  dragNode.value = n
  window.addEventListener('pointermove', onDrag)
  window.addEventListener('pointerup', endDrag)
  reheat()
  e.stopPropagation()
}
function onDrag(e) {
  if (!dragNode.value) return
  const p = screenToWorld(e.clientX, e.clientY)
  dragNode.value.x = p.x
  dragNode.value.y = p.y
  
  const el = nodeEls.get(dragNode.value.id)
  if (el) el.setAttribute('transform', `translate(${dragNode.value.x},${dragNode.value.y})`)
  
  for (const l of simLinksRaw) {
    if (l.s.id === dragNode.value.id || l.t.id === dragNode.value.id) {
      const elL = linkEls.get(l.id)
      if (elL) {
        elL.setAttribute('x1', l.s.x)
        elL.setAttribute('y1', l.s.y)
        elL.setAttribute('x2', l.t.x)
        elL.setAttribute('y2', l.t.y)
      }
    }
  }
}
function endDrag() {
  dragNode.value = null
  window.removeEventListener('pointermove', onDrag)
  window.removeEventListener('pointerup', endDrag)
}

function focusSearch() {
  const q = query.value.trim().toLowerCase()
  if (!q) return
  const hit = simNodesRaw.find((n) => (n.title || '').toLowerCase().includes(q))
  if (!hit) return
  selectedId.value = hit.id
  centerOn(hit)
}
function centerOn(n) {
  const w = wrapRef.value?.clientWidth || 960
  const h = wrapRef.value?.clientHeight || 720
  scale.value = Math.max(scale.value, 1.2)
  panX.value = w / 2 - n.x * scale.value
  panY.value = h / 2 - n.y * scale.value
}

const reload = () => load()

onMounted(load)
onUnmounted(() => {
  cancelAnimationFrame(raf)
  endPan(); endDrag()
})
</script>

<style scoped>
.graph-grid {
  background-image:
    linear-gradient(var(--color-border-hairline) 1px, transparent 1px),
    linear-gradient(90deg, var(--color-border-hairline) 1px, transparent 1px);
  background-size: 48px 48px;
  -webkit-mask-image: radial-gradient(circle at 50% 50%, var(--color-bg-base) 55%, transparent 100%);
  mask-image: radial-gradient(circle at 50% 50%, var(--color-bg-base) 55%, transparent 100%);
}
.ctrl-btn {
  display: flex; align-items: center; justify-content: center;
  width: 34px; height: 34px; border-radius: var(--radius-full);
  color: var(--color-text-muted);
  background: var(--color-surface);
  border: 1px solid var(--color-border-hairline);
  transition: color 150ms, background 150ms, border-color 150ms;
}
.ctrl-btn:hover { color: var(--color-text-primary); border-color: var(--color-border-default); background: var(--color-surface-hover); }
.ctrl-btn:focus-visible { outline: 1px solid var(--color-primary); outline-offset: 1px; }

.stat { display: flex; flex-direction: column; gap: 2px; padding: 8px 12px; border: 1px solid var(--color-border-hairline); border-radius: var(--radius-full); background: var(--color-surface-container-low); }
.stat-num { font-family: var(--font-mono-data); font-size: var(--text-display); line-height: 1; color: var(--color-text-primary); }
.stat-lbl { font-family: var(--font-mono-data); font-size: var(--text-mono-data); color: var(--color-text-faint); }

.conn-head { font-family: var(--font-label-caps); font-size: var(--text-label-caps); color: var(--color-text-faint); margin-bottom: 8px; }
.conn-row {
  display: flex; align-items: center; gap: 8px; width: 100%;
  padding: 7px 9px; border-radius: var(--radius-full);
  border: 1px solid transparent;
  background: var(--color-surface-container-low);
  text-align: left; transition: background 120ms, border-color 120ms;
}
.conn-row:hover { background: var(--color-surface-hover); border-color: var(--color-border-default); }
.conn-row:focus-visible { outline: 1px solid var(--color-primary); outline-offset: 1px; }

.legend-btn:focus-visible { outline: 1px solid var(--color-primary); outline-offset: 2px; }
.conn-label { font-family: var(--font-mono-data); font-size: var(--text-mono-data); color: var(--color-da-accent-text); white-space: nowrap; }
.conn-title { font-size: var(--text-body); color: var(--color-text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }

.panel-enter-active, .panel-leave-active { transition: transform 220ms cubic-bezier(0.22, 1, 0.36, 1), opacity 220ms; }
.panel-enter-from, .panel-leave-to { transform: translateX(24px); opacity: 0; }
</style>
