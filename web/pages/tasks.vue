<template>
  <div class="flex flex-col h-full min-h-0 relative">
    <!-- Surface header -->
    <header class="px-section pt-margin pb-component border-b border-border-hairline flex items-end justify-between gap-section shrink-0">
      <div class="min-w-0">
        <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Tasks</h1>
        <p class="font-body text-body text-text-muted mt-tight">{{ summary }}</p>
      </div>

      <!-- View toggle (segmented control) -->
      <div class="flex items-center border border-border-hairline rounded-lg overflow-hidden shrink-0">
        <button
          v-for="opt in views"
          :key="opt.id"
          class="flex items-center gap-tight px-component py-base font-body text-body transition-colors duration-150"
          :class="view === opt.id
            ? 'bg-surface-hover text-text-primary'
            : 'text-text-muted hover:text-text-primary hover:bg-surface'"
          @click="view = opt.id"
        >
          <span class="material-symbols-outlined !text-[16px]">{{ opt.icon }}</span>
          {{ opt.label }}
        </button>
      </div>
    </header>

    <!-- Loading -->
    <div v-if="loading" class="flex-1 flex items-center justify-center text-text-muted">
      <span class="material-symbols-outlined animate-spin !text-[24px]">progress_activity</span>
    </div>

    <!-- Error -->
    <div v-else-if="error" class="flex-1 flex flex-col items-center justify-center gap-base text-text-muted">
      <span class="material-symbols-outlined !text-[32px] text-status-failed">error</span>
      <p class="font-body text-body">Couldn't load tasks.</p>
      <p class="font-mono-data text-mono-data text-text-faint">{{ error }}</p>
    </div>

    <!-- Empty -->
    <div v-else-if="tasks.length === 0" class="flex-1 flex flex-col items-center justify-center gap-base text-text-muted">
      <span class="material-symbols-outlined !text-[32px]">task_alt</span>
      <p class="font-body text-body">No tasks yet.</p>
    </div>

    <!-- Content -->
    <TaskBoard v-else-if="view === 'kanban'" :tasks="tasks" @open="openDetail" />
    <TaskList v-else :tasks="tasks" @open="openDetail" />

    <!-- Read-only detail drawer -->
    <TaskDetail :bead="selected" @close="selected = null" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useBeads, type Bead } from '~/composables/useBeads'
import { normalizeTaskStatus } from '~/utils/workflow'
import TaskBoard from '~/components/tasks/TaskBoard.vue'
import TaskList from '~/components/tasks/TaskList.vue'
import TaskDetail from '~/components/tasks/TaskDetail.vue'

const { tasks, loading, error, load } = useBeads()

type View = 'kanban' | 'list'
const view = ref<View>('kanban')
const views: { id: View; label: string; icon: string }[] = [
  { id: 'kanban', label: 'Kanban', icon: 'view_kanban' },
  { id: 'list', label: 'List', icon: 'view_list' },
]

const selected = ref<Bead | null>(null)
function openDetail(bead: Bead) {
  selected.value = bead
}

// One-line summary: "N open, M in progress" (omits empty buckets gracefully).
const summary = computed(() => {
  const counts = { open: 0, in_progress: 0, blocked: 0, done: 0 }
  for (const t of tasks.value) counts[normalizeTaskStatus(t.state)]++
  const parts: string[] = []
  if (counts.open) parts.push(`${counts.open} open`)
  if (counts.in_progress) parts.push(`${counts.in_progress} in progress`)
  if (counts.blocked) parts.push(`${counts.blocked} blocked`)
  if (counts.done) parts.push(`${counts.done} done`)
  return parts.length ? parts.join(', ') : 'Nothing here yet.'
})

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && selected.value) selected.value = null
}

onMounted(() => {
  load()
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>
