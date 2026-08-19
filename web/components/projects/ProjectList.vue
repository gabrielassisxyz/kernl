<template>
  <div class="flex-1 min-h-0 overflow-y-auto flex flex-col px-7 pt-1 pb-[72px] text-body">
    <section v-for="section in sections" :key="section.id" class="mt-[26px]">
      <button
        type="button"
        class="w-full flex items-baseline gap-2 pb-[7px] border-b border-border-hairline select-none cursor-pointer"
        :aria-expanded="!collapsed[section.id]"
        @click="$emit('toggle-section', section.id)"
      >
        <span
          class="material-symbols-outlined self-center section-chevron transition-transform duration-150"
          :class="collapsed[section.id] ? '-rotate-90' : 'rotate-0'"
          aria-hidden="true"
        >expand_more</span>
        <span class="font-label-caps text-label-caps uppercase text-text-label">{{ section.label }}</span>
        <span class="font-mono-data text-mono-data text-text-faint">{{ section.projects.length }}</span>
      </button>

      <template v-if="!collapsed[section.id] && view === 'list'">
        <ProjectRow
          v-for="p in section.projects"
          :key="p.id"
          :project="p"
          :compact="compact"
          :confirming="confirmId === p.id"
          @open="$emit('open', $event)"
          @toggle-pin="$emit('toggle-pin', $event)"
          @ask-delete="$emit('ask-delete', $event)"
          @confirm-delete="$emit('confirm-delete', $event)"
          @cancel-delete="$emit('cancel-delete')"
        />
      </template>

      <div v-else-if="!collapsed[section.id]" class="grid gap-2.5 mt-3.5" :style="CARD_GRID">
        <ProjectCard
          v-for="p in section.projects"
          :key="p.id"
          :project="p"
          :confirming="confirmId === p.id"
          @open="$emit('open', $event)"
          @toggle-pin="$emit('toggle-pin', $event)"
          @ask-delete="$emit('ask-delete', $event)"
          @confirm-delete="$emit('confirm-delete', $event)"
          @cancel-delete="$emit('cancel-delete')"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Project, ProjectStatus } from '~/composables/useProjects'
import type { ListSortDir, ListSortField } from '~/composables/useListPreferences'
import ProjectRow from '~/components/projects/ProjectRow.vue'
import ProjectCard from '~/components/projects/ProjectCard.vue'

const props = defineProps<{
  projects: Project[]
  view: 'list' | 'card'
  compact?: boolean
  confirmId?: string | null
  collapsed: Record<string, boolean>
  sortField: ListSortField
  sortDir: ListSortDir
}>()

defineEmits<{
  (e: 'open', project: Project): void
  (e: 'toggle-pin', project: Project): void
  (e: 'ask-delete', project: Project): void
  (e: 'confirm-delete', project: Project): void
  (e: 'cancel-delete'): void
  (e: 'toggle-section', id: string): void
}>()

const CARD_GRID = { gridTemplateColumns: 'repeat(auto-fill, minmax(224px, 1fr))' }

const LIFECYCLE: { id: ProjectStatus; label: string }[] = [
  { id: 'active', label: 'Active' },
  { id: 'stalled', label: 'Stalled' },
  { id: 'paused', label: 'Paused' },
  { id: 'not_started', label: 'Not started' },
  { id: 'done', label: 'Done' },
  { id: 'archived', label: 'Archived' },
]

// A pinned project appears in Pinned and nowhere else. Listing it twice would
// make the section counts add up to more than the project count, and pinning
// exists precisely so the project stops being wherever its status put it.
const sections = computed(() => {
  const pinned = props.projects.filter((p) => p.pinned)
  const rest = props.projects.filter((p) => !p.pinned)
  return [
    { id: 'pinned', label: 'Pinned', projects: sorted(pinned) },
    ...LIFECYCLE.map((s) => ({
      id: s.id,
      label: s.label,
      projects: sorted(rest.filter((p) => p.status === s.id)),
    })),
  ].filter((s) => s.projects.length > 0)
})

const sorted = (projects: Project[]): Project[] => {
  const direction = props.sortDir === 'asc' ? 1 : -1
  return [...projects].sort((a, b) => {
    if (props.sortField === 'name') return a.title.localeCompare(b.title) * direction
    const field = props.sortField === 'created' ? 'createdAt' : 'updatedAt'
    return (a[field] < b[field] ? -1 : a[field] > b[field] ? 1 : 0) * direction
  })
}
</script>

<style scoped>
.section-chevron { font-size: 12px; line-height: 1; color: var(--color-text-faint); }
</style>
