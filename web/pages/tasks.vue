<template>
  <div class="flex flex-col h-full min-h-0 text-body">
    <header class="shrink-0 px-7 pt-[22px] pb-4 border-b border-border-hairline flex items-end justify-between gap-6">
      <div class="min-w-0">
        <h1 class="font-display text-display text-text-heading">Tasks</h1>
        <p class="mt-[5px] text-text-muted">{{ summary }}</p>
      </div>

      <div class="flex items-center gap-2.5 shrink-0">
        <div class="flex items-center gap-[7px] h-[30px] w-[230px] px-[9px] rounded-lg bg-bg-elevated border border-border-default focus-within:border-primary/70 transition-colors">
          <span class="material-symbols-outlined control-icon text-text-muted" aria-hidden="true">search</span>
          <input
            v-model="query"
            type="search"
            placeholder="Search tasks"
            aria-label="Search tasks"
            class="flex-1 min-w-0 bg-transparent border-0 outline-none text-text-primary placeholder:text-text-faint"
          />
        </div>

        <div class="flex items-center gap-1.5 h-[30px] pl-2 pr-[9px] rounded-lg bg-bg-elevated border border-border-default">
          <span class="material-symbols-outlined control-icon text-text-muted" aria-hidden="true">filter_list</span>
          <select
            :value="projectFilter"
            aria-label="Filter by project"
            class="w-[120px] bg-transparent border-0 outline-none cursor-pointer font-mono-data text-mono-data text-text-secondary"
            @change="setProjectFilter(($event.target as HTMLSelectElement).value)"
          >
            <option value="">all projects</option>
            <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
          </select>
        </div>

        <div class="flex h-[30px] rounded-lg border border-border-default overflow-hidden">
          <button
            v-for="opt in VIEWS"
            :key="opt.id"
            type="button"
            class="w-8 flex items-center justify-center cursor-pointer transition-colors outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-primary/30"
            :class="view === opt.id ? 'bg-accent-tint text-primary' : 'text-text-muted hover:text-text-primary'"
            :title="opt.title"
            :aria-label="opt.title"
            :aria-pressed="view === opt.id"
            @click="view = opt.id"
          >
            <span class="material-symbols-outlined control-icon" aria-hidden="true">{{ opt.icon }}</span>
          </button>
        </div>

        <UiSortControl
          :sort-field="sortField"
          :sort-dir="sortDir"
          :sort-label="sortLabel()"
          @update:sort-field="setSortField"
          @toggle-direction="toggleSortDir"
        />

        <UiButton size="sm" variant="primary" icon="add" :icon-size="13" @click="openCreate">New task</UiButton>
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
      @action="openCreate"
    />

    <div v-else class="flex-1 min-h-0 flex">
      <div v-if="visible.length === 0" class="flex-1 px-7 py-16 text-text-muted">
        No tasks match “{{ query }}”.
      </div>

      <TaskBoard
        v-else-if="view === 'board'"
        :tasks="visible"
        :project-titles="projectTitles"
        @open="openEdit"
        @advance="advance"
      />

      <TaskList
        v-else
        :tasks="visible"
        :project-titles="projectTitles"
        :compact="panelOpen"
        :confirm-id="confirmId"
        :collapsed="collapsed"
        :sort-field="sortField"
        :sort-dir="sortDir"
        @open="openEdit"
        @toggle-section="toggleSection"
        @toggle-done="toggleDone"
        @advance="advance"
        @ask-delete="confirmId = $event.id"
        @confirm-delete="removeTask"
        @cancel-delete="confirmId = null"
      />

      <TaskPanel
        v-if="panelOpen"
        :key="editing?.id ?? 'create'"
        :task="editing"
        :projects="projects"
        :briefing="briefing"
        @close="closePanel"
        @patch="applyPatch"
        @create="createTask"
        @delete="removeTask"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { isFinished, useTasks, type NewTask, type Task, type TaskPatch, type TaskStatus } from '~/composables/useTasks'
import { useProjects } from '~/composables/useProjects'
import TaskBoard from '~/components/tasks/TaskBoard.vue'
import TaskList from '~/components/tasks/TaskList.vue'
import TaskPanel from '~/components/tasks/TaskPanel.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiSkeleton from '~/components/ui/UiSkeleton.vue'
import UiSortControl from '~/components/ui/UiSortControl.vue'
import { useListPreferences } from '~/composables/useListPreferences'

const { tasks, loading, error, load, create, update, remove } = useTasks()
const { projects, load: loadProjects } = useProjects()

const route = useRoute()
const projectFilter = ref<string>(typeof route.query.project === 'string' ? route.query.project : '')

type View = 'list' | 'board'
const VIEWS: { id: View; icon: string; title: string }[] = [
  { id: 'list', icon: 'view_list', title: 'List view' },
  { id: 'board', icon: 'view_kanban', title: 'Board view' },
]
const view = ref<View>('list')
const { collapsed, sortField, sortDir, sortLabel, toggleSection, setSortField, toggleSortDir } =
  useListPreferences('kernl:tasks-list-preferences', { done: true, closed: true })

const query = ref('')
const confirmId = ref<string | null>(null)
// `editing === null` while the panel is open means create mode; a task means
// edit. The panel's :key uses that difference to rebuild its draft.
const editing = ref<Task | null>(null)
const panelOpen = ref(false)
const briefing = ref<{ body: string } | null>(null)

const projectTitles = computed<Record<string, string>>(() =>
  Object.fromEntries(projects.value.map((p) => [p.id, p.title])),
)

// Search is client-side: the whole list is already loaded, and a round trip per
// keystroke would be slower than filtering what is in memory.
const visible = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return tasks.value
  return tasks.value.filter((t) =>
    `${t.title} ${t.description} ${projectTitles.value[t.projectId] ?? ''}`.toLowerCase().includes(q),
  )
})

const summary = computed(() => {
  const counts: Record<TaskStatus, number> = { todo: 0, in_progress: 0, done: 0, closed: 0 }
  for (const t of tasks.value) counts[t.status]++
  const parts: string[] = []
  if (counts.todo) parts.push(`${counts.todo} to do`)
  if (counts.in_progress) parts.push(`${counts.in_progress} in progress`)
  if (counts.done) parts.push(`${counts.done} done`)
  if (counts.closed) parts.push(`${counts.closed} closed`)
  return parts.length ? parts.join(' · ') : 'Nothing here yet.'
})

function reload() {
  load(projectFilter.value || undefined)
}

// The URL's ?project= is the single source of truth for the filter. On a
// statically-generated page the router hydrates with the server route (empty
// query) and only resolves the real location a tick after onMounted, so a
// setup-time init misses the filter on a hard reload of /tasks/?project=X.
watch(
  () => route.query.project,
  (id) => {
    projectFilter.value = typeof id === 'string' ? id : ''
    reload()
  },
)

function setProjectFilter(id: string) {
  navigateTo({ path: '/tasks', query: id ? { project: id } : {} })
}

function openCreate() {
  editing.value = null
  briefing.value = null
  confirmId.value = null
  panelOpen.value = true
}

async function openEdit(task: Task) {
  editing.value = task
  confirmId.value = null
  panelOpen.value = true
  briefing.value = null
  try {
    briefing.value = await $fetch<{ body: string }>(`/api/nodes/${task.id}/briefing`)
  } catch {
    briefing.value = null // 404 = this task did not come from a capture
  }
}

function closePanel() {
  panelOpen.value = false
  editing.value = null
  briefing.value = null
}

// Every write reloads: the list carries fields only the server recomputes -
// updatedAt, and which section the task now belongs to.
async function applyPatch(id: string, patch: TaskPatch) {
  await update(id, patch)
  if (editing.value?.id === id) {
    editing.value = { ...editing.value, ...patch }
  }
  reload()
}

async function createTask(t: NewTask) {
  await create(t)
  closePanel()
  reload()
}

// Closed is not on the cycle: work is called off deliberately, from the panel,
// never by pressing the same button one more time. Advancing out of it reopens
// it, which is the only move that makes sense from a task nobody is doing.
const NEXT_STATUS: Record<TaskStatus, TaskStatus> = {
  todo: 'in_progress',
  in_progress: 'done',
  done: 'todo',
  closed: 'todo',
}

const advance = (t: Task) => applyPatch(t.id, { status: NEXT_STATUS[t.status] })
// The bullet finishes an open task and reopens a finished or called-off one.
// Sending a closed task to done would quietly rewrite why it ended.
const toggleDone = (t: Task) => applyPatch(t.id, { status: isFinished(t.status) ? 'todo' : 'done' })

async function removeTask(task: Task) {
  confirmId.value = null
  await remove(task.id)
  if (editing.value?.id === task.id) closePanel()
  reload()
}

// Escape from anywhere on the page. The panel stops the event when focus is
// inside it, so this only fires when focus is not.
function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  if (confirmId.value) confirmId.value = null
  else if (panelOpen.value) closePanel()
}

onMounted(() => {
  loadProjects()
  reload()
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<style scoped>
.control-icon {
  font-size: 14px;
  line-height: 1;
}
</style>
