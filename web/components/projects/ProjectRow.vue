<template>
  <div
    class="project-row group grid items-center gap-3 px-2.5 py-[9px] rounded border-b border-border-row text-body cursor-pointer hover:bg-surface-hover transition-colors duration-150"
    :class="compact ? 'project-row--compact' : 'project-row--full'"
    @click="$emit('open', project)"
  >
    <span class="w-1.5 h-1.5 mt-0.5 shrink-0 rounded-full" :class="STATUS_DOT[project.status]"></span>

    <div class="min-w-0">
      <!-- The title keeps its own destination: the row opens the panel, the
           title goes to the project's tasks. Without the stop, the link would
           navigate and open a panel that is about to be unmounted. -->
      <NuxtLink
        :to="`/tasks?project=${encodeURIComponent(project.id)}`"
        class="row-title inline-block max-w-full truncate font-medium text-text-primary hover:underline outline-none focus-visible:ring-1 focus-visible:ring-primary/30 rounded"
        @click.stop
      >
        {{ project.title }}
      </NuxtLink>
      <p v-if="project.description" class="row-desc mt-0.5 truncate text-text-muted">{{ project.description }}</p>
    </div>

    <!-- The cell stays even when a project has no tag, so the columns after it
         keep lining up down the page. -->
    <div v-if="!compact" class="flex items-center justify-end gap-[7px] min-w-0 justify-self-end">
      <span
        v-if="project.tags?.length"
        class="min-w-0 truncate font-mono-data text-mono-data text-text-muted bg-surface-chip rounded-sm px-[5px] py-px"
      >
        {{ project.tags[0] }}
      </span>
    </div>

    <div v-if="!compact" class="flex items-center gap-[9px] justify-self-end">
      <span class="font-mono-data text-mono-data text-text-label tabular-nums">
        {{ project.doneCount }}/{{ project.taskCount }}
      </span>
      <div
        class="w-[88px] h-[3px] rounded-xs bg-surface-control-hover overflow-hidden"
        role="progressbar"
        :aria-valuenow="percent"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-label="`${project.title} progress`"
      >
        <div class="h-full rounded-xs bg-primary transition-[width] duration-300" :style="{ width: `${percent}%` }"></div>
      </div>
    </div>

    <span v-if="!compact" class="justify-self-end whitespace-nowrap font-mono-data text-mono-data text-text-muted">
      {{ formatRelativeTime(project.updatedAt) }}
    </span>

    <div class="justify-self-end" @click.stop>
      <div v-if="confirming" class="flex items-center gap-2 whitespace-nowrap">
        <span class="text-text-secondary">Delete?</span>
        <button
          type="button"
          class="font-medium text-status-failed-text hover:underline outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
          @click="$emit('confirm-delete', project)"
        >
          Yes
        </button>
        <button
          type="button"
          class="text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
          @click="$emit('cancel-delete')"
        >
          No
        </button>
      </div>
      <div v-else class="flex items-center gap-0.5">
        <!-- The pin is both the state marker and the control: on a pinned row
             it stays lit instead of waiting for a hover, and clicking it
             unpins. Edit and delete stay hover-only, pinned or not. -->
        <button
          type="button"
          class="row-action transition-opacity duration-150"
          :class="project.pinned
            ? 'text-primary opacity-100'
            : 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100'"
          :title="project.pinned ? 'Unpin' : 'Pin'"
          :aria-label="`${project.pinned ? 'Unpin' : 'Pin'} ${project.title}`"
          :aria-pressed="project.pinned"
          @click="$emit('toggle-pin', project)"
        >
          <span class="material-symbols-outlined row-action-icon" aria-hidden="true">keep</span>
        </button>
        <button
          type="button"
          class="row-action opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-150"
          title="Edit"
          :aria-label="`Edit ${project.title}`"
          @click="$emit('open', project)"
        >
          <span class="material-symbols-outlined row-action-icon" aria-hidden="true">edit</span>
        </button>
        <button
          type="button"
          class="row-action row-action--danger opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-150"
          title="Delete"
          :aria-label="`Delete ${project.title}`"
          @click="$emit('ask-delete', project)"
        >
          <span class="material-symbols-outlined row-action-icon" aria-hidden="true">delete</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Project, ProjectStatus } from '~/composables/useProjects'
import { formatRelativeTime } from '~/utils/time'

const props = defineProps<{
  project: Project
  /** The side panel is open, so the row gives up its metadata columns. */
  compact?: boolean
  confirming?: boolean
}>()

defineEmits<{
  (e: 'open', project: Project): void
  (e: 'toggle-pin', project: Project): void
  (e: 'ask-delete', project: Project): void
  (e: 'confirm-delete', project: Project): void
  (e: 'cancel-delete'): void
}>()

const STATUS_DOT: Record<ProjectStatus, string> = {
  active: 'bg-status-active',
  paused: 'bg-status-gate',
  done: 'bg-status-done',
  archived: 'bg-status-archived',
}

// A project with no tasks has no progress to report; 0/0 is not 0%.
const percent = computed(() =>
  props.project.taskCount > 0
    ? Math.round((props.project.doneCount / props.project.taskCount) * 100)
    : 0,
)
</script>

<style scoped>
/* Two stacked lines inside a 9px-padded row: the body step's 20px leading turns
   a 55px row into a 61px one, and the density is what keeps a long project list
   scannable. */
.row-title {
  line-height: normal;
}
.row-desc {
  font-size: 12.5px;
  line-height: normal;
}

.project-row--full {
  grid-template-columns: 10px minmax(0, 1fr) 132px 150px 76px 96px;
}
.project-row--compact {
  grid-template-columns: 10px minmax(0, 1fr) 96px;
}

.row-action {
  display: flex;
  padding: 4px;
  border-radius: var(--radius);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease-out, color 120ms ease-out;
}
.row-action:hover {
  background-color: var(--color-surface-control-hover);
  color: var(--color-text-primary);
}
.row-action--danger:hover {
  color: var(--color-status-failed-text);
}
.row-action-icon {
  font-size: 14px;
  line-height: 1;
}
</style>
