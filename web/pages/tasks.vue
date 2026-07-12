<template>
  <div class="flex flex-col h-full min-h-0 relative">
    <header class="px-section pt-margin pb-component border-b border-border-hairline flex items-end justify-between gap-section shrink-0">
      <div class="min-w-0">
        <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Tasks</h1>
        <p class="font-body text-body text-text-muted mt-tight">{{ summary }}</p>
      </div>

      <div class="flex items-center gap-base shrink-0">
        <div class="relative" ref="filterContainerRef" @keydown.esc.stop="closeFilter">
          <button
            ref="filterTriggerRef"
            class="flex items-center gap-tight px-component py-base border border-border-hairline rounded font-body text-body text-text-muted hover:text-text-primary hover:bg-surface transition-colors cursor-pointer outline-none focus-visible:border-primary/70 focus-visible:ring-1 focus-visible:ring-primary/30"
            aria-haspopup="listbox"
            :aria-expanded="filterOpen"
            @click="filterOpen = !filterOpen"
          >
            <span class="material-symbols-outlined !text-[16px]">filter_list</span>
            {{ projectFilter ? (projectTitles[projectFilter] || 'Project') : 'All projects' }}
          </button>
          <div
            v-if="filterOpen"
            class="absolute right-0 mt-tight w-[200px] max-h-[320px] overflow-y-auto py-tight bg-surface border border-border-hairline rounded z-dropdown"
            @click="filterOpen = false"
          >
            <button
              class="w-full text-left px-component py-base font-body text-body hover:bg-surface-hover transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
              :class="!projectFilter ? 'text-text-primary' : 'text-text-muted'"
              @click="setProjectFilter('')"
            >
              All projects
            </button>
            <button
              v-for="p in projects"
              :key="p.id"
              class="w-full text-left px-component py-base font-body text-body hover:bg-surface-hover transition-colors truncate cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
              :class="projectFilter === p.id ? 'text-text-primary' : 'text-text-muted'"
              @click="setProjectFilter(p.id)"
            >
              {{ p.title }}
            </button>
          </div>
        </div>

        <div class="flex items-center border border-border-hairline rounded overflow-hidden">
          <button
            v-for="opt in views"
            :key="opt.id"
            class="flex items-center gap-tight px-component py-base font-body text-body transition-colors duration-150 cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-primary/30"
            :class="view === opt.id ? 'bg-surface-hover text-text-primary' : 'text-text-muted hover:text-text-primary hover:bg-surface'"
            @click="view = opt.id"
          >
            <span class="material-symbols-outlined !text-[16px]">{{ opt.icon }}</span>
            {{ opt.label }}
          </button>
        </div>

        <button
          class="flex items-center gap-tight px-component py-base rounded bg-primary text-on-primary font-body text-body hover:bg-primary-container transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
          @click="showCreate = true"
        >
          <span class="material-symbols-outlined !text-[16px]">add</span>
          New task
        </button>
      </div>
    </header>

    <div v-if="loading" class="flex-1 flex items-center justify-center px-margin">
      <UiSkeleton classes="h-[220px] w-full max-w-[720px]" text="Loading tasks..." />
    </div>

    <UiErrorState
      v-else-if="error"
      fill
      title="Could not load tasks."
      message="Check that the Kernl API is running, then retry."
      :detail="error"
      retry-label="Retry"
      @retry="reload"
    />

    <UiEmptyState
      v-else-if="tasks.length === 0"
      fill
      icon="task_alt"
      title="No tasks yet."
      body="Create a task directly, or process captures from Inbox into tasks."
      action-label="New task"
      action-icon="add"
      @action="showCreate = true"
    />

    <TaskBoard v-else-if="view === 'kanban'" :tasks="tasks" :project-titles="projectTitles" @open="openDetail" />
    <TaskList v-else :tasks="tasks" :project-titles="projectTitles" @open="openDetail" />

    <TaskDetail
      :task="selected"
      :project-title="selected ? projectTitles[selected.projectId] : undefined"
      @close="selected = null"
      @set-status="changeStatus"
      @set-tags="changeTags"
    />

    <TaskCreateModal
      v-if="showCreate"
      :default-project-id="projectFilter"
      @close="showCreate = false"
      @created="reload"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useTasks, type Task, type TaskStatus, TASK_STATUSES } from '~/composables/useTasks'
import { useProjects } from '~/composables/useProjects'
import TaskBoard from '~/components/tasks/TaskBoard.vue'
import TaskList from '~/components/tasks/TaskList.vue'
import TaskDetail from '~/components/tasks/TaskDetail.vue'
import TaskCreateModal from '~/components/tasks/TaskCreateModal.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiSkeleton from '~/components/ui/UiSkeleton.vue'

const { tasks, loading, error, load, update, setStatus } = useTasks()
const { projects, load: loadProjects } = useProjects()

const route = useRoute()
const projectFilter = ref<string>(typeof route.query.project === 'string' ? route.query.project : '')

type View = 'kanban' | 'list'
const view = ref<View>('kanban')
const views: { id: View; label: string; icon: string }[] = [
  { id: 'kanban', label: 'Kanban', icon: 'view_kanban' },
  { id: 'list', label: 'List', icon: 'view_list' },
]

const showCreate = ref(false)
const filterOpen = ref(false)
const selected = ref<Task | null>(null)

// --- Filter dropdown: refs for outside-click + focus restore ---
const filterTriggerRef = ref<HTMLButtonElement | null>(null)
const filterContainerRef = ref<HTMLElement | null>(null)

function onDocumentClick(e: MouseEvent) {
  if (!filterContainerRef.value?.contains(e.target as Node)) {
    filterOpen.value = false
  }
}

// Escape handler called from @keydown.esc.stop on the container div.
function closeFilter() {
  filterOpen.value = false
  filterTriggerRef.value?.focus()
}

// Add / remove the outside-click listener in sync with open state.
// nextTick defers the add so the triggering click doesn't immediately re-close.
watch(filterOpen, (open) => {
  if (open) {
    nextTick(() => document.addEventListener('click', onDocumentClick))
  } else {
    document.removeEventListener('click', onDocumentClick)
  }
})

const projectTitles = computed<Record<string, string>>(() =>
  Object.fromEntries(projects.value.map((p) => [p.id, p.title]))
)

const summary = computed(() => {
  const counts: Record<TaskStatus, number> = { todo: 0, in_progress: 0, done: 0 }
  for (const t of tasks.value) counts[t.status]++
  const parts: string[] = []
  if (counts.todo) parts.push(`${counts.todo} to do`)
  if (counts.in_progress) parts.push(`${counts.in_progress} in progress`)
  if (counts.done) parts.push(`${counts.done} done`)
  return parts.length ? parts.join(', ') : 'Nothing here yet.'
})

function reload() {
  load(projectFilter.value || undefined)
}

function setProjectFilter(id: string) {
  projectFilter.value = id
  // Keep the URL shareable / consistent with the Projects → Tasks drill-in.
  navigateTo({ path: '/tasks', query: id ? { project: id } : {} })
  reload()
}

function openDetail(task: Task) {
  selected.value = task
}

async function changeStatus(id: string, status: string) {
  await setStatus(id, status as TaskStatus)
  if (selected.value?.id === id) selected.value = { ...selected.value, status: status as TaskStatus }
  reload()
}

async function changeTags(id: string, tags: string[]) {
  await update(id, { tags })
  if (selected.value?.id === id) selected.value = { ...selected.value, tags }
  reload()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && selected.value) selected.value = null
}

onMounted(async () => {
  loadProjects()
  await load(projectFilter.value || undefined)
  // Deep link from the tags page: ?task=<id> opens that task's drawer.
  const id = typeof route.query.task === 'string' ? route.query.task : ''
  if (id) selected.value = tasks.value.find((t) => t.id === id) ?? null
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
  document.removeEventListener('click', onDocumentClick)
})
</script>
