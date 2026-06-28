<template>
  <div class="px-margin pt-margin">
    <header class="mb-break">
      <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">{{ greeting }}</h1>
    </header>

    <CaptureThought />

    <div class="grid grid-cols-1 md:grid-cols-2 gap-section md:gap-break pb-section md:pb-break mt-section md:mt-break">
      <section class="flex flex-col">
        <div class="pb-base mb-base border-b border-border-hairline flex items-center justify-between">
          <h2 class="font-label-caps text-label-caps text-text-primary">Today's tasks</h2>
          <span class="font-body text-body text-text-muted">{{ tasks.length }} open</span>
        </div>
        <div class="flex flex-col">
          <div v-if="tasks.length === 0" class="py-section font-body text-body">
            <p class="text-text-faint">No open tasks.</p>
            <p class="text-text-muted mt-1 leading-relaxed">Create beads to track actionable work and assign them to agents or yourself.</p>
          </div>
          <div
            v-for="t in tasks"
            :key="t.id"
            class="flex items-center py-2 gap-component transition-colors group cursor-pointer"
          >
            <span class="w-[6px] h-[6px] rounded-full flex-shrink-0" :class="needsAttention(t) ? 'bg-status-gate' : 'bg-status-running'"></span>
            <span class="font-mono-data text-mono-data w-12" :class="needsAttention(t) ? 'text-status-gate' : 'text-text-faint group-hover:text-text-muted'">{{ t.id }}</span>
            <span class="font-body text-body text-text-primary truncate">{{ t.title }}</span>
          </div>
        </div>
      </section>

      <section class="flex flex-col">
        <div class="pb-base mb-base border-b border-border-hairline flex items-center justify-between">
          <h2 class="font-label-caps text-label-caps text-text-primary">Active projects</h2>
          <span class="font-body text-body text-text-muted">{{ projects.length }} active</span>
        </div>
        <div class="flex flex-col">
          <div v-if="projects.length === 0" class="py-section font-body text-body">
            <p class="text-text-faint">No active projects.</p>
            <p class="text-text-muted mt-1 leading-relaxed">Group epic beads to coordinate multi-step goals and complex graph updates.</p>
          </div>
          <div
            v-for="p in projects"
            :key="p.id"
            class="flex items-center justify-between py-2 pl-[3px] gap-component group cursor-pointer"
          >
            <div class="flex items-center gap-component min-w-0">
              <span class="w-[6px] h-[6px] rounded-full flex-shrink-0" :class="needsAttention(p) ? 'bg-status-gate animate-pulse' : 'bg-status-running'"></span>
              <span class="font-body text-body text-text-primary truncate">{{ p.title }}</span>
            </div>
            <span class="font-label-caps text-label-caps transition-colors whitespace-nowrap" :class="statePill(p.state).classes">{{ statePill(p.state).label }}</span>
          </div>
        </div>
      </section>

    </div>

    <section class="pb-margin">
      <div class="pb-base mb-base border-b border-border-hairline">
        <h2 class="font-label-caps text-label-caps text-text-primary">What shipped</h2>
      </div>
      <div class="flex flex-col">
        <div v-if="shipped.length === 0" class="py-section text-text-faint font-body text-body">Nothing shipped yet</div>
        <div v-else class="flex flex-col">
          <div v-for="s in shipped" :key="s.id" class="flex items-center py-1.5 gap-component group cursor-pointer hover:bg-surface-hover px-2 -mx-2 rounded transition-colors">
            <span class="font-mono-data text-mono-data text-text-muted w-[100px] flex-shrink-0">{{ fmtTime(s.closedAt || s.updatedAt) }}</span>
            <span class="font-mono-data text-mono-data text-status-passed w-12 flex-shrink-0">{{ s.id }}</span>
            <span class="font-body text-body text-text-primary truncate">{{ s.title }}</span>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'

// Single source for the three read panes: the orchestrator bead graph.
const { data: beadsData } = useFetch('/api/beads', { server: false, default: () => [] })
const beads = computed(() => beadsData.value || [])

const isClosed = (b) => (b.state || '').toLowerCase() === 'closed'
const needsAttention = (b) => /review|gate|await|block/i.test(b.state || '')

// Short, colored status pill for Active Projects.
const statePill = (s) => {
  const v = (s || '').toLowerCase()
  if (/block/.test(v)) return { label: 'BLOCKED', classes: 'text-status-failed-text' }
  if (/review|await/.test(v)) return { label: 'REVIEW', classes: 'text-status-gate' }
  if (/flight|progress|running|active/.test(v)) return { label: 'IN-FLIGHT', classes: 'text-status-active' }
  if (/ready/.test(v)) return { label: 'READY', classes: 'text-status-active' }
  if (/queue|open|todo/.test(v)) return { label: 'QUEUED', classes: 'text-text-muted' }
  const first = (s || '').replace(/_/g, ' ').trim().split(' ')[0] || ''
  return { label: first.toUpperCase(), classes: 'text-text-muted' }
}

const tasks = computed(() => beads.value.filter(b => b.type !== 'epic' && !isClosed(b)).slice(0, 6))
const projects = computed(() => beads.value.filter(b => b.type === 'epic' && !isClosed(b)).slice(0, 6))
const shipped = computed(() =>
  beads.value
    .filter(isClosed)
    .sort((a, b) => new Date(b.closedAt || b.updatedAt || 0) - new Date(a.closedAt || a.updatedAt || 0))
    .slice(0, 6)
)

const timeGreeting = () => {
  const h = new Date().getHours()
  if (h >= 5 && h <= 11) return 'Good morning.'
  if (h >= 12 && h <= 17) return 'Good afternoon.'
  return 'Good evening.'
}

const greeting = computed(() => {
  const n = shipped.value.length
  const prefix = timeGreeting()
  if (n === 0) return prefix
  return `${prefix} ${n} ${n === 1 ? 'thing' : 'things'} shipped recently.`
})

const fmtTime = (s) => {
  if (!s) return ''
  const d = new Date(s)
  return isNaN(d.getTime()) ? '' : d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}
</script>
