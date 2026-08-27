<template>
  <div class="flex flex-col h-full min-h-0 text-body">
    <header class="shrink-0 px-7 pt-[22px] pb-4 border-b border-border-hairline flex items-end justify-between gap-6">
      <div class="min-w-0">
        <h1 class="font-display text-display text-text-heading">Projects</h1>
        <p class="mt-[5px] text-text-muted">{{ summary }}</p>
      </div>

      <div class="flex items-center gap-2.5 shrink-0">
        <div class="flex items-center gap-[7px] h-[30px] w-[250px] px-[9px] rounded-lg bg-bg-elevated border border-border-default focus-within:border-primary/70 transition-colors">
          <span class="material-symbols-outlined control-icon text-text-muted" aria-hidden="true">search</span>
          <input
            v-model="query"
            type="search"
            placeholder="Search projects"
            aria-label="Search projects"
            class="flex-1 min-w-0 bg-transparent border-0 outline-none text-text-primary placeholder:text-text-faint"
          />
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

        <UiButton size="sm" variant="primary" icon="add" :icon-size="13" @click="openCreate">New</UiButton>
      </div>
    </header>

    <div v-if="loading && projects.length === 0" class="flex-1 flex items-center justify-center px-margin">
      <UiSkeleton classes="h-[220px] w-full max-w-[720px]" text="Loading projects..." />
    </div>

    <UiErrorState
      v-else-if="error"
      fill
      title="Could not load projects."
      message="Check that the Kernl API is reachable, then retry."
      :detail="error"
      retry-label="Retry"
      @retry="load"
    />

    <UiEmptyState
      v-else-if="projects.length === 0"
      fill
      icon="folder_open"
      title="No projects yet."
      body="Create a project to organize tasks around active work."
      action-label="New project"
      action-icon="add"
      @action="openCreate"
    />

    <div v-else class="flex-1 min-h-0 flex">
      <div v-if="visible.length === 0" class="flex-1 px-7 py-16 text-text-muted">
        No projects match “{{ query }}”.
      </div>

      <ProjectList
        v-else
        :projects="visible"
        :view="view"
        :compact="panelOpen"
        :confirm-id="confirmId"
        :collapsed="collapsed"
        :sort-field="sortField"
        :sort-dir="sortDir"
        @open="openEdit"
        @toggle-section="toggleSection"
        @toggle-pin="togglePin"
        @ask-delete="confirmId = $event.id"
        @confirm-delete="removeProject"
        @cancel-delete="confirmId = null"
      />

      <ProjectPanel
        v-if="panelOpen"
        ref="panelRef"
        :key="editing?.id ?? 'create'"
        :project="editing"
        @close="closePanel"
        @patch="applyPatch"
        @create="createProject"
        @delete="removeProject"
        @toggle-task="toggleTask"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useProjects, type NewProject, type Project, type ProjectStatus } from '~/composables/useProjects'
import { isFinished, useTasks, type Task } from '~/composables/useTasks'
import ProjectList from '~/components/projects/ProjectList.vue'
import ProjectPanel from '~/components/projects/ProjectPanel.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiSkeleton from '~/components/ui/UiSkeleton.vue'
import UiSortControl from '~/components/ui/UiSortControl.vue'
import { useListPreferences } from '~/composables/useListPreferences'

const { projects, loading, error, load, create, update, remove } = useProjects()
// Only used to write a nested task's status; the panel owns its own reading.
const { update: updateTask } = useTasks()

type View = 'list' | 'card'
const VIEWS: { id: View; icon: string; title: string }[] = [
  { id: 'list', icon: 'view_list', title: 'List view' },
  { id: 'card', icon: 'view_kanban', title: 'Card view' },
]
const view = ref<View>('list')
const { collapsed, sortField, sortDir, sortLabel, toggleSection, setSortField, toggleSortDir } =
  useListPreferences('kernl:projects-list-preferences', { paused: true, done: true, archived: true })

const query = ref('')
const confirmId = ref<string | null>(null)
const editing = ref<Project | null>(null)
const panelOpen = ref(false)
const panelRef = ref<InstanceType<typeof ProjectPanel> | null>(null)

type ProjectPatch = { title?: string; description?: string; status?: ProjectStatus; pinned?: boolean; tags?: string[] }

// Search is client-side: the whole list is already loaded, and a round trip per
// keystroke would be slower than filtering what is in memory.
const visible = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return projects.value
  return projects.value.filter((p) =>
    `${p.title} ${p.description} ${(p.tags ?? []).join(' ')}`.toLowerCase().includes(q),
  )
})

const summary = computed(() => {
  const n = projects.value.length
  if (loading.value && n === 0) return 'Loading…'
  if (n === 0) return 'No projects yet.'
  const parts = [`${n} ${n === 1 ? 'project' : 'projects'}`]
  const active = projects.value.filter((p) => p.status === 'active').length
  if (active) parts.push(`${active} active`)
  const pinned = projects.value.filter((p) => p.pinned).length
  if (pinned) parts.push(`${pinned} pinned`)
  return parts.join(' · ')
})

function openCreate() {
  editing.value = null
  confirmId.value = null
  panelOpen.value = true
}

function openEdit(project: Project) {
  editing.value = project
  confirmId.value = null
  panelOpen.value = true
}

function closePanel() {
  panelOpen.value = false
  editing.value = null
}

// useProjects.update reloads the list itself, so the panel's copy is refreshed
// from the reloaded array rather than patched by hand.
async function applyPatch(id: string, patch: ProjectPatch) {
  await update(id, patch)
  if (editing.value?.id === id) {
    editing.value = projects.value.find((p) => p.id === id) ?? null
  }
}

async function createProject(p: NewProject) {
  await create(p)
  closePanel()
}

const togglePin = (p: Project) => applyPatch(p.id, { pinned: !p.pinned })

async function removeProject(project: Project) {
  confirmId.value = null
  await remove(project.id)
  if (editing.value?.id === project.id) closePanel()
}

// A nested task's status changes the project's done count, so both the panel's
// list and the project rows behind it have to be refetched.
async function toggleTask(task: Task) {
  await updateTask(task.id, { status: isFinished(task.status) ? 'todo' : 'done' })
  panelRef.value?.reloadTasks()
  await load()
  if (editing.value) editing.value = projects.value.find((p) => p.id === editing.value!.id) ?? null
}

function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  if (confirmId.value) confirmId.value = null
  else if (panelOpen.value) closePanel()
}

onMounted(() => {
  load()
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
