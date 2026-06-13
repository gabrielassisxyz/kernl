<template>
  <div class="px-margin pt-margin">
    <!-- Header -->
    <header class="mb-break">
      <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">{{ greeting }}</h1>
    </header>

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
            <span class="font-label-caps text-[10px] py-0.5 px-2 rounded bg-surface-container" :class="needsAttention(p) ? 'text-status-gate' : 'text-text-muted'">{{ stateLabel(p.state) }}</span>
          </div>
        </div>
      </section>

      <!-- Pane 3: What Shipped -->
      <section class="bg-surface border border-border-hairline rounded-lg h-[340px] flex flex-col">
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

      <!-- Pane 4: Quick Capture -->
      <section class="bg-surface border border-border-hairline rounded-lg h-[340px] flex flex-col">
        <div class="px-component py-base border-b border-border-hairline">
          <h2 class="font-label-caps text-label-caps text-text-muted">QUICK CAPTURE</h2>
        </div>
        <div class="flex-grow p-component flex flex-col justify-end gap-component">
          <div class="flex-grow flex items-center justify-center text-center opacity-30">
            <span class="material-symbols-outlined text-[48px]">lightbulb</span>
          </div>
          <div class="w-full">
            <label class="block font-mono-data text-[10px] text-text-faint mb-tight uppercase tracking-widest">Entry Stream</label>
            <div class="bg-surface-container-low border border-border-hairline rounded px-base py-component min-h-[44px] flex items-center focus-within:border-primary transition-colors">
              <input v-model="captureInput" @keyup.enter="submitCapture" class="w-full bg-transparent border-none p-0 focus:ring-0 font-body text-body text-text-primary placeholder:text-text-faint outline-none custom-caret" :placeholder="capturePlaceholder" type="text">
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
const stateLabel = (s) => (s || '').replace(/_/g, ' ').toUpperCase()

const tasks = computed(() => beads.value.filter(b => b.type !== 'epic' && !isClosed(b)).slice(0, 6))
const projects = computed(() => beads.value.filter(b => b.type === 'epic' && !isClosed(b)).slice(0, 6))
const shipped = computed(() =>
  beads.value
    .filter(isClosed)
    .sort((a, b) => new Date(b.closedAt || b.updatedAt || 0) - new Date(a.closedAt || a.updatedAt || 0))
    .slice(0, 6)
)

const greeting = computed(() => {
  const n = shipped.value.length
  if (n === 0) return 'Welcome back.'
  return `Welcome back. ${n} ${n === 1 ? 'thing' : 'things'} shipped recently.`
})

const fmtTime = (s) => {
  if (!s) return ''
  const d = new Date(s)
  return isNaN(d.getTime()) ? '' : d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

// Quick Capture — writes a real Capture into the inbox (same as `kernl capture`).
const captureInput = ref('')
const capturePlaceholder = ref('capture a thought...')

const submitCapture = async () => {
  const body = captureInput.value.trim()
  if (!body) return
  captureInput.value = ''
  try {
    await $fetch('/api/inbox', { method: 'POST', body: { body } })
    capturePlaceholder.value = 'Captured → Inbox'
  } catch (e) {
    capturePlaceholder.value = 'Capture failed'
  }
  setTimeout(() => { capturePlaceholder.value = 'capture a thought...' }, 2000)
}
</script>
