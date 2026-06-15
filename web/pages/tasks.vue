<template>
  <div class="flex flex-col h-full min-h-0 relative">
    <!-- Surface header -->
    <header class="px-section pt-margin pb-component border-b border-border-hairline flex items-end justify-between gap-section shrink-0">
      <div class="min-w-0">
        <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Tasks</h1>
        <p class="font-body text-body text-text-muted mt-tight">{{ summary }}</p>
      </div>

      <div class="flex items-center gap-base shrink-0">
        <!-- Project filter -->
        <div class="relative">
          <button
            class="flex items-center gap-tight px-component py-base border border-border-hairline rounded-lg font-body text-body text-text-muted hover:text-text-primary hover:bg-surface transition-colors cursor-pointer"
            @click="filterOpen = !filterOpen"
          >
            <span class="material-symbols-outlined !text-[16px]">filter_list</span>
            {{ projectFilter ? (projectTitles[projectFilter] || 'Project') : 'All projects' }}
          </button>
          <div
            v-if="filterOpen"
            class="absolute right-0 mt-tight w-[200px] max-h-[320px] overflow-y-auto py-tight bg-surface border border-border-hairline rounded-lg shadow-xl z-50"
            @click="filterOpen = false"
          >
            <button
              class="w-full text-left px-component py-base font-body text-body hover:bg-surface-hover transition-colors cursor-pointer"
              :class="!projectFilter ? 'text-text-primary' : 'text-text-muted'"
              @click="setProjectFilter('')"
            >
              All projects
            </button>
            <button
              v-for="p in projects"
              :key="p.id"
              class="w-full text-left px-component py-base font-body text-body hover:bg-surface-hover transition-colors truncate cursor-pointer"
              :class="projectFilter === p.id ? 'text-text-primary' : 'text-text-muted'"
              @click="setProjectFilter(p.id)"
            >
              {{ p.title }}
            </button>
          </div>
        </div>

        <!-- View toggle -->
        <div class="flex items-center border border-border-hairline rounded-lg overflow-hidden">
          <button
            v-for="opt in views"
            :key="opt.id"
            class="flex items-center gap-tight px-component py-base font-body text-body transition-colors duration-150 cursor-pointer"
            :class="view === opt.id ? 'bg-surface-hover text-text-primary' : 'text-text-muted hover:text-text-primary hover:bg-surface'"
            @click="view = opt.id"
          >
            <span class="material-symbols-outlined !text-[16px]">{{ opt.icon }}</span>
            {{ opt.label }}
          </button>
        </div>

        <!-- New task -->
        <button
          class="flex items-center gap-tight px-component py-base rounded-lg bg-primary text-white font-body text-body hover:bg-primary/90 transition-colors cursor-pointer"
          @click="showCreate = true"
        >
          <span class="material-symbols-outlined !text-[16px]">add</span>
          New task
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
    <TaskBoard v-else-if="view === 'kanban'" :tasks="tasks" :project-titles="projectTitles" @open="openDetail" />
    <TaskList v-else :tasks="tasks" :project-titles="projectTitles" @open="openDetail" />

    <!-- Detail drawer -->
    <TaskDetail
      :task="selected"
      :project-title="selected ? projectTitles[selected.projectId] : undefined"
      @close="selected = null"
      @set-status="changeStatus"
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
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useTasks, type Task, type TaskStatus, TASK_STATUSES } from '~/composables/useTasks'
import { useProjects } from '~/composables/useProjects'
import TaskBoard from '~/components/tasks/TaskBoard.vue'
import TaskList from '~/components/tasks/TaskList.vue'
import TaskDetail from '~/components/tasks/TaskDetail.vue'
import TaskCreateModal from '~/components/tasks/TaskCreateModal.vue'

const { tasks, loading, error, load, setStatus } = useTasks()
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

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && selected.value) selected.value = null
}

onMounted(() => {
  loadProjects()
  reload()
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>
