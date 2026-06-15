<template>
  <div class="flex flex-col h-full min-h-0">
    <!-- Surface header -->
    <header class="shrink-0 px-margin pt-margin pb-section">
      <div class="flex items-center justify-between gap-section flex-wrap">
        <div class="flex items-center gap-section min-w-0">
          <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Orchestrator</h1>

          <!-- epic selector -->
          <div v-if="epics.length" class="relative max-w-[420px]">
            <select
              v-model="selectedEpicId"
              class="appearance-none w-full truncate bg-surface-container-low border border-border-hairline rounded h-9 pl-component pr-8 font-mono-data text-mono-data text-text-primary hover:border-border-default focus:border-primary outline-none cursor-pointer transition-colors"
            >
              <option v-for="e in epics" :key="e.id" :value="e.id">{{ e.id }} · {{ e.title }}</option>
            </select>
            <span class="material-symbols-outlined absolute right-1.5 top-1/2 -translate-y-1/2 text-text-faint !text-[18px] pointer-events-none">expand_more</span>
          </div>
        </div>

        <!-- live indicator -->
        <div v-if="selectedEpicId" class="flex items-center gap-tight font-mono-data text-[11px]" :class="live ? 'text-status-passed' : 'text-text-dim'">
          <span class="w-1.5 h-1.5 rounded-full" :class="live ? 'bg-status-passed animate-pulse' : 'bg-text-dim'"></span>
          {{ live ? 'live' : 'offline' }}
        </div>
      </div>

      <p class="mt-base font-body text-body text-text-muted">{{ summary }}</p>
    </header>

    <!-- Loading -->
    <div v-if="loading && beads.length === 0" class="flex-1 px-margin pb-margin grid grid-cols-5 gap-section min-h-0">
      <div v-for="n in 5" :key="n" class="rounded-lg border border-border-hairline bg-surface animate-pulse"></div>
    </div>

    <!-- Empty -->
    <div v-else-if="epics.length === 0" class="flex-1 flex flex-col items-center justify-center text-text-muted">
      <span class="material-symbols-outlined text-[32px] mb-component text-text-faint">account_tree</span>
      <p class="font-body text-body">{{ error ? 'Could not load the pipeline' : 'No epics to orchestrate' }}</p>
      <p v-if="!error" class="mt-tight font-body text-body text-text-faint">Epics from the bead graph appear here.</p>
    </div>

    <!-- Kanban -->
    <div v-else class="flex-1 min-h-0 px-margin pb-margin flex gap-section overflow-x-auto">
      <StageColumn
        v-for="col in ORCHESTRATOR_COLUMNS"
        :key="col.id"
        :column="col"
        :beads="byColumn[col.id]"
        @open="openBead"
      />
    </div>

    <!-- Detail modal -->
    <BeadDetailModal
      :bead="selectedBead"
      :epic-id="selectedEpicId"
      @close="selectedBead = null"
      @mutated="scheduleReload"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { useBeads, type Bead } from '~/composables/useBeads'
import { ORCHESTRATOR_COLUMNS, stageBucket, type StageBucket } from '~/utils/workflow'
import StageColumn from '~/components/orchestrator/StageColumn.vue'
import BeadDetailModal from '~/components/orchestrator/BeadDetailModal.vue'

const route = useRoute()
const { beads, loading, error, load, epics, childrenOf } = useBeads()

const selectedEpicId = ref('')
const selectedBead = ref<Bead | null>(null)
const live = ref(false)

// Place children into their macro-stage column.
const byColumn = computed(() => {
  const groups: Record<StageBucket, Bead[]> = {
    planning: [], implementation: [], integration: [], shipment: [], done: [],
  }
  if (selectedEpicId.value) {
    for (const b of childrenOf(selectedEpicId.value)) groups[stageBucket(b.state)].push(b)
  }
  return groups
})

const inFlight = computed(() =>
  childrenOf(selectedEpicId.value).filter(
    (b) => !['shipped', 'closed', 'done', 'abandoned', 'deferred'].includes(b.state)
  ).length
)

const summary = computed(() => {
  if (loading.value && beads.value.length === 0) return 'Loading…'
  if (!selectedEpicId.value) return 'Select an epic to orchestrate.'
  const total = childrenOf(selectedEpicId.value).length
  const beadWord = inFlight.value === 1 ? 'bead' : 'beads'
  return `Epic ${selectedEpicId.value} · ${inFlight.value} ${beadWord} in flight · ${total} total`
})

function openBead(b: Bead) {
  selectedBead.value = b
}

// Keep the open modal's bead reference fresh after a reload (state may have moved).
watch(beads, (list) => {
  if (selectedBead.value) {
    const next = list.find((b) => b.id === selectedBead.value!.id)
    selectedBead.value = next ?? null
  }
})

// Default-select after the first load: honor ?epic=, else first epic.
function ensureSelection() {
  if (!epics.value.length) return
  if (selectedEpicId.value && epics.value.some((e) => e.id === selectedEpicId.value)) return
  const wanted = String(route.query.epic || '')
  selectedEpicId.value = epics.value.find((e) => e.id === wanted)?.id ?? epics.value[0].id
}
watch(epics, ensureSelection)

// ── Live epic SSE: debounced reload so cards move between columns ─────────────
let es: EventSource | null = null
let reloadTimer: ReturnType<typeof setTimeout> | null = null

function scheduleReload() {
  if (reloadTimer) clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => { load() }, 400)
}

function closeStream() {
  if (es) { es.close(); es = null }
  live.value = false
}

function openStream(epicId: string) {
  closeStream()
  if (!epicId) return
  es = new EventSource(`/api/epics/${encodeURIComponent(epicId)}/events`)
  es.onopen = () => { live.value = true }
  es.onmessage = () => { live.value = true; scheduleReload() }
  es.onerror = () => { live.value = false /* EventSource auto-retries */ }
}

watch(selectedEpicId, (id) => {
  selectedBead.value = null
  openStream(id)
})

onMounted(async () => {
  await load()
  ensureSelection()
})

onBeforeUnmount(() => {
  closeStream()
  if (reloadTimer) clearTimeout(reloadTimer)
})
</script>
