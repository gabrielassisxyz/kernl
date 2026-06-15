<template>
  <div class="px-margin pt-margin pb-margin">
    <!-- Surface header -->
    <header class="mb-section flex items-end justify-between gap-section">
      <div class="min-w-0">
        <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Projects</h1>
        <p class="mt-tight font-body text-body text-text-muted">{{ summary }}</p>
      </div>

      <div class="flex items-center gap-base shrink-0">
        <!-- Status filter -->
        <div class="relative">
          <button
            class="flex items-center gap-tight px-component py-base border border-border-hairline rounded-lg font-body text-body text-text-muted hover:text-text-primary hover:bg-surface transition-colors cursor-pointer"
            @click="filterOpen = !filterOpen"
          >
            <span class="material-symbols-outlined !text-[16px]">filter_list</span>
            {{ statusFilter === 'all' ? 'All status' : statusLabel(statusFilter) }}
          </button>
          <div
            v-if="filterOpen"
            class="absolute right-0 mt-tight w-[160px] py-tight bg-surface border border-border-hairline rounded-lg shadow-xl z-50"
            @click="filterOpen = false"
          >
            <button
              v-for="opt in filterOptions"
              :key="opt.id"
              class="w-full text-left px-component py-base font-body text-body hover:bg-surface-hover transition-colors cursor-pointer"
              :class="statusFilter === opt.id ? 'text-text-primary' : 'text-text-muted'"
              @click="statusFilter = opt.id"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <!-- New project -->
        <button
          class="flex items-center gap-tight px-component py-base rounded-lg bg-primary text-white font-body text-body hover:bg-primary/90 transition-colors cursor-pointer"
          @click="showCreate = true"
        >
          <span class="material-symbols-outlined !text-[16px]">add</span>
          New project
        </button>
      </div>
    </header>

    <!-- Loading -->
    <div v-if="loading && projects.length === 0" class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-section">
      <div v-for="n in 6" :key="n" class="h-[180px] rounded-lg border border-border-hairline bg-surface animate-pulse"></div>
    </div>

    <!-- Empty -->
    <div
      v-else-if="filtered.length === 0"
      class="flex flex-col items-center justify-center py-break text-text-muted border border-border-hairline rounded-lg bg-surface"
    >
      <span class="material-symbols-outlined text-[32px] mb-component text-text-faint">folder_open</span>
      <p class="font-body text-body">{{ error ? 'Could not load projects' : projects.length === 0 ? 'No projects yet' : 'No projects match this filter' }}</p>
      <p v-if="!error && projects.length === 0" class="mt-tight font-body text-body text-text-faint">
        Create a project to organize your tasks.
      </p>
    </div>

    <!-- Card grid -->
    <div v-else class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-section">
      <ProjectCard v-for="p in filtered" :key="p.id" :project="p" @open="open(p.id)" />
    </div>

    <ProjectCreateModal v-if="showCreate" @close="showCreate = false" @created="load" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useProjects, PROJECT_STATUSES, type ProjectStatus } from '~/composables/useProjects'
import ProjectCard from '~/components/projects/ProjectCard.vue'
import ProjectCreateModal from '~/components/projects/ProjectCreateModal.vue'

const { projects, loading, error, load } = useProjects()

const showCreate = ref(false)
const filterOpen = ref(false)
const statusFilter = ref<'all' | ProjectStatus>('all')

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
</script>
