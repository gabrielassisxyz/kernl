<template>
  <div class="px-margin pt-margin pb-margin">
    <header class="mb-section flex items-end justify-between gap-section">
      <div class="min-w-0">
        <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Projects</h1>
        <p class="mt-tight font-body text-body text-text-muted">{{ summary }}</p>
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
            {{ statusFilter === 'all' ? 'All status' : statusLabel(statusFilter) }}
          </button>
          <div
            v-if="filterOpen"
            class="absolute right-0 mt-tight w-[160px] py-tight bg-surface border border-border-hairline rounded z-dropdown"
            @click="filterOpen = false"
          >
            <button
              v-for="opt in filterOptions"
              :key="opt.id"
              class="w-full text-left px-component py-base font-body text-body hover:bg-surface-hover transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
              :class="statusFilter === opt.id ? 'text-text-primary' : 'text-text-muted'"
              @click="statusFilter = opt.id"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <button
          class="flex items-center gap-tight px-component py-base rounded bg-primary text-on-primary font-body text-body hover:bg-primary-container transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
          @click="showCreate = true"
        >
          <span class="material-symbols-outlined !text-[16px]">add</span>
          New project
        </button>
      </div>
    </header>

    <div v-if="loading && projects.length === 0" class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-section">
      <UiSkeleton v-for="n in 6" :key="n" classes="h-[180px]" text="Loading projects..." />
    </div>

    <UiErrorState
      v-else-if="error"
      bordered
      title="Could not load projects."
      message="Check that the Kernl API is reachable, then retry."
      :detail="error"
      retry-label="Retry"
      @retry="load"
    />

    <UiEmptyState
      v-else-if="filtered.length === 0"
      bordered
      icon="folder_open"
      :title="projects.length === 0 ? 'No projects yet.' : 'No projects match this filter.'"
      :body="projects.length === 0 ? 'Create a project to organize tasks around active work.' : 'Adjust the status filter to see hidden projects.'"
      :action-label="projects.length === 0 ? 'New project' : ''"
      action-icon="add"
      @action="showCreate = true"
    />

    <div v-else class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-section">
      <!-- Wrapper hosts the hover actions: the card itself is a <button>, so
           edit/delete cannot legally nest inside it. -->
      <div v-for="p in filtered" :key="p.id" class="relative group/card">
        <ProjectCard :project="p" @open="open(p.id)" />
        <div
          class="absolute top-2 right-2 flex items-center gap-1 opacity-0 group-hover/card:opacity-100 focus-within:opacity-100 transition-opacity"
        >
          <button
            type="button"
            class="card-action"
            :aria-label="`Edit project ${p.title}`"
            title="Edit project"
            @click.stop="editing = p"
          >
            <span class="material-symbols-outlined !text-[16px]" aria-hidden="true">edit</span>
          </button>
          <button
            type="button"
            class="card-action card-action--danger"
            :aria-label="`Delete project ${p.title}`"
            title="Delete project"
            @click.stop="deleting = p"
          >
            <span class="material-symbols-outlined !text-[16px]" aria-hidden="true">delete</span>
          </button>
        </div>
      </div>
    </div>

    <ProjectCreateModal v-if="showCreate" @close="showCreate = false" @created="load" />

    <ProjectEditModal
      v-if="editing"
      ref="editModalRef"
      :project="editing"
      @close="editing = null"
      @save="saveEdit"
    />

    <UiModal :open="!!deleting" title="Delete project" size="sm" @close="deleting = null">
      <p class="font-body text-body text-text-muted">
        Delete <span class="text-text-primary">{{ deleting?.title }}</span>? Its companion note is
        removed too. Tasks are kept and become unassigned. This cannot be undone.
      </p>
      <p v-if="deleteError" class="mt-base font-mono-data text-mono-data text-status-failed-text">{{ deleteError }}</p>
      <template #footer>
        <div class="flex justify-end gap-base">
          <UiButton variant="ghost" @click="deleting = null">Cancel</UiButton>
          <UiButton variant="primary" :loading="removing" @click="confirmDelete">Delete project</UiButton>
        </div>
      </template>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useProjects, PROJECT_STATUSES, type Project, type ProjectStatus } from '~/composables/useProjects'
import ProjectCard from '~/components/projects/ProjectCard.vue'
import ProjectCreateModal from '~/components/projects/ProjectCreateModal.vue'
import ProjectEditModal from '~/components/projects/ProjectEditModal.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiModal from '~/components/ui/UiModal.vue'
import UiSkeleton from '~/components/ui/UiSkeleton.vue'

const { projects, loading, error, load, update, remove } = useProjects()

const showCreate = ref(false)
const editing = ref<Project | null>(null)
const deleting = ref<Project | null>(null)
const removing = ref(false)
const deleteError = ref<string | null>(null)
const editModalRef = ref<InstanceType<typeof ProjectEditModal> | null>(null)

async function saveEdit(patch: { title: string; description: string; status: ProjectStatus }) {
  if (!editing.value) return
  editModalRef.value?.setSaving(true)
  editModalRef.value?.setError(null)
  try {
    await update(editing.value.id, patch)
    editing.value = null
  } catch (e) {
    editModalRef.value?.setError(e instanceof Error ? e.message : String(e))
  } finally {
    editModalRef.value?.setSaving(false)
  }
}

async function confirmDelete() {
  if (!deleting.value || removing.value) return
  removing.value = true
  deleteError.value = null
  try {
    await remove(deleting.value.id)
    deleting.value = null
  } catch (e) {
    deleteError.value = e instanceof Error ? e.message : String(e)
  } finally {
    removing.value = false
  }
}
const filterOpen = ref(false)
const statusFilter = ref<'all' | ProjectStatus>('all')

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

const filterOptions = [{ id: 'all' as const, label: 'All status' }, ...PROJECT_STATUSES]
const statusLabel = (s: ProjectStatus) => PROJECT_STATUSES.find((o) => o.id === s)?.label ?? s

const filtered = computed(() =>
  statusFilter.value === 'all'
    ? projects.value
    : projects.value.filter((p) => p.status === statusFilter.value)
)

const summary = computed(() => {
  const n = projects.value.length
  if (loading.value && n === 0) return 'Loading…'
  if (n === 0) return 'No projects yet.'
  const active = projects.value.filter((p) => p.status === 'active').length
  return `${n} ${n === 1 ? 'project' : 'projects'}, ${active} active`
})

function open(id: string) {
  navigateTo('/tasks?project=' + encodeURIComponent(id))
}

onMounted(load)
onUnmounted(() => document.removeEventListener('click', onDocumentClick))
</script>

<style scoped>
.card-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-hairline);
  background-color: var(--color-surface-overlay);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease, border-color 120ms ease;
}

.card-action:hover {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

.card-action--danger:hover {
  color: var(--color-status-failed-text);
  border-color: color-mix(in srgb, var(--color-status-failed-text) 40%, transparent);
}

.card-action:focus-visible {
  outline: none;
  border-color: color-mix(in srgb, var(--color-primary) 70%, transparent);
}
</style>
