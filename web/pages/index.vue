<template>
  <div class="px-margin pt-margin">
    <!-- Header -->
    <header class="mb-break">
      <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">{{ greeting }}</h1>
    </header>

    <!-- Quick Capture — full-width input row -->
    <div class="mb-section bg-surface-container-low border border-border-hairline rounded h-12 px-base flex items-center gap-component focus-within:border-primary transition-colors">
      <input
        v-model="captureInput"
        @keyup.enter="submitCapture"
        type="text"
        placeholder="capture a thought…"
        class="w-full bg-transparent border-none p-0 focus:ring-0 font-body text-body text-text-primary placeholder:text-text-faint outline-none custom-caret"
      >
      <span
        class="flex items-center gap-1 font-mono-data text-mono-data text-text-faint transition-opacity duration-300 shrink-0"
        :class="captured ? 'opacity-100' : 'opacity-0'"
      >
        <span class="material-symbols-outlined !text-[14px]">check</span>captured
      </span>
    </div>

    <!-- Bento Grid Layout -->
    <div class="grid grid-cols-2 gap-section pb-margin">

      <!-- Pane 1: Today's Tasks -->
      <section class="bg-surface border border-border-hairline rounded-lg h-[340px] flex flex-col">
        <div class="px-component py-base border-b border-border-hairline flex items-center justify-between">
          <h2 class="font-label-caps text-label-caps text-text-muted">TODAY'S TASKS</h2>
          <span class="material-symbols-outlined text-text-faint text-sm">more_vert</span>
        </div>
        <div class="flex-grow overflow-y-auto p-base flex flex-col gap-tight">
          <div v-if="tasks.length === 0" class="flex-grow flex items-center justify-center text-text-faint font-body text-body">No open tasks</div>
          <div
            v-for="t in tasks"
            :key="t.id"
            class="flex items-center py-2 px-base gap-component transition-colors"
            :class="needsAttention(t) ? 'bg-surface-container-low border-l-2 border-status-gate' : 'hover:bg-surface-hover group'"
          >
            <span class="font-mono-data text-mono-data" :class="needsAttention(t) ? 'text-status-gate' : 'text-text-faint group-hover:text-text-muted'">{{ t.id }}</span>
            <span class="font-body text-body text-text-primary">{{ t.title }}</span>
          </div>
        </div>
      </section>

      <!-- Pane 2: Active Projects -->
      <section class="bg-surface border border-border-hairline rounded-lg h-[340px] flex flex-col">
        <div class="px-component py-base border-b border-border-hairline flex items-center justify-between">
          <h2 class="font-label-caps text-label-caps text-text-muted">ACTIVE PROJECTS</h2>
          <span class="material-symbols-outlined text-text-faint text-sm">filter_list</span>
        </div>
        <div class="flex-grow p-component flex flex-col gap-base overflow-y-auto">
          <div v-if="projects.length === 0" class="flex-grow flex items-center justify-center text-text-faint font-body text-body">No active projects</div>
          <div
            v-for="p in projects"
            :key="p.id"
            class="flex items-center justify-between py-base border-b border-border-hairline last:border-0"
          >
            <div class="flex items-center gap-component">
              <span class="w-1.5 h-1.5 rounded-full" :class="needsAttention(p) ? 'bg-status-gate animate-pulse' : 'bg-status-running'"></span>
              <span class="font-body text-body text-text-primary">{{ p.title }}</span>
            </div>
            <span class="font-label-caps text-[10px] py-0.5 px-2 rounded bg-surface-container-high" :class="statePill(p.state).classes">{{ statePill(p.state).label }}</span>
          </div>
        </div>
      </section>

      <!-- Pane 3: What Shipped -->
      <section class="col-span-2 bg-surface border border-border-hairline rounded-lg h-[300px] flex flex-col">
        <div class="px-component py-base border-b border-border-hairline">
          <h2 class="font-label-caps text-label-caps text-text-muted">WHAT SHIPPED</h2>
        </div>
        <div class="flex-grow p-component overflow-y-auto">
          <div v-if="shipped.length === 0" class="h-full flex items-center justify-center text-text-faint font-body text-body">Nothing shipped yet</div>
          <div v-else class="flex flex-col gap-base border-l border-border-hairline ml-base pl-base">
            <div v-for="s in shipped" :key="s.id" class="relative">
              <div class="absolute -left-[21px] top-1 w-2.5 h-2.5 bg-status-passed rounded-full border-2 border-surface"></div>
              <span class="font-mono-data text-mono-data text-text-muted block">{{ fmtTime(s.closedAt || s.updatedAt) }}</span>
              <span class="font-mono-data text-mono-data text-status-passed">[{{ s.id }}]</span>
              <span class="font-body text-body text-text-primary">{{ s.title }}</span>
            </div>
          </div>
        </div>
      </section>

    </div>
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
  if (/block/.test(v)) return { label: 'BLOCKED', classes: 'text-status-failed' }
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

// Quick Capture — writes a real Capture into the inbox (same as `kernl capture`).
const captureInput = ref('')
const captured = ref(false)
let captureTimer

const submitCapture = async () => {
  const body = captureInput.value.trim()
  if (!body) return
  captureInput.value = ''
  try {
    await $fetch('/api/inbox', { method: 'POST', body: { body } })
    captured.value = true
    clearTimeout(captureTimer)
    captureTimer = setTimeout(() => { captured.value = false }, 1500)
  } catch (e) {
    // swallow; keep the input quiet on failure
  }
}
</script>
