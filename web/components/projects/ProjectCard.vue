<template>
  <!-- A div, not a button. The card carries its own pin, edit and delete
       controls, and a button cannot legally contain them. -->
  <div
    class="group flex flex-col min-h-[108px] p-3 rounded-xl border bg-surface text-body cursor-pointer transition-colors duration-150"
    :class="hovered ? 'border-border-default bg-surface-hover' : 'border-border-hairline'"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
    @click="$emit('open', project)"
  >
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-1.5 min-w-0">
        <span class="w-1.5 h-1.5 shrink-0 rounded-full" :class="STATUS_DOT[project.status]"></span>
        <span v-if="project.tags?.length" class="min-w-0 truncate font-mono-data text-mono-data text-text-muted">
          {{ project.tags[0] }}
        </span>
      </div>
      <div class="flex gap-px" @click.stop>
        <!-- The pin is the exception: on a pinned card it stays lit rather than
             waiting for a hover, because a card has no column for a state
             marker and the lit pin is the only way to tell. Edit and delete
             are ordinary hover actions, pinned or not. -->
        <button
          type="button"
          class="card-action transition-opacity duration-150"
          :class="[
            project.pinned ? 'text-primary opacity-100' : hovered ? 'opacity-100' : 'opacity-0',
          ]"
          :title="project.pinned ? 'Unpin' : 'Pin'"
          :aria-label="`${project.pinned ? 'Unpin' : 'Pin'} ${project.title}`"
          :aria-pressed="project.pinned"
          @click="$emit('toggle-pin', project)"
        >
          <span class="material-symbols-outlined card-action-icon" aria-hidden="true">keep</span>
        </button>
        <button
          type="button"
          class="card-action transition-opacity duration-150"
          :class="hovered ? 'opacity-100' : 'opacity-0'"
          title="Edit"
          :aria-label="`Edit ${project.title}`"
          @click="$emit('open', project)"
        >
          <span class="material-symbols-outlined card-action-icon" aria-hidden="true">edit</span>
        </button>
        <button
          type="button"
          class="card-action card-action--danger transition-opacity duration-150"
          :class="hovered ? 'opacity-100' : 'opacity-0'"
          title="Delete"
          :aria-label="`Delete ${project.title}`"
          @click="$emit('ask-delete', project)"
        >
          <span class="material-symbols-outlined card-action-icon" aria-hidden="true">delete</span>
        </button>
      </div>
    </div>

    <!-- The title keeps its own destination: the card opens the panel, the
         title goes to the project's tasks. Without the stop, the link would
         navigate and open a panel that is about to be unmounted. -->
    <NuxtLink
      :to="`/tasks?project=${encodeURIComponent(project.id)}`"
      class="block mt-2.5 font-medium leading-snug text-text-primary hover:underline outline-none focus-visible:ring-1 focus-visible:ring-primary/30 rounded line-clamp-2"
      @click.stop
    >
      {{ project.title }}
    </NuxtLink>

    <div class="flex-1 min-h-2.5"></div>

    <div v-if="confirming" class="flex items-center gap-2 mt-2" @click.stop>
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
    <template v-else>
      <div class="flex items-center justify-between gap-2 mt-2.5 font-mono-data text-mono-data text-text-muted tabular-nums">
        <span>{{ project.doneCount }}/{{ project.taskCount }}</span>
        <span>{{ formatRelativeTime(project.updatedAt) }}</span>
      </div>
      <div
        class="mt-[7px] h-0.5 rounded-xs bg-surface-control-hover overflow-hidden"
        role="progressbar"
        :aria-valuenow="percent"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-label="`${project.title} progress`"
      >
        <div class="h-full rounded-xs bg-primary transition-[width] duration-300" :style="{ width: `${percent}%` }"></div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Project, ProjectStatus } from '~/composables/useProjects'
import { formatRelativeTime } from '~/utils/time'

const props = defineProps<{ project: Project; confirming?: boolean }>()

defineEmits<{
  (e: 'open', project: Project): void
  (e: 'toggle-pin', project: Project): void
  (e: 'ask-delete', project: Project): void
  (e: 'confirm-delete', project: Project): void
  (e: 'cancel-delete'): void
}>()

const hovered = ref(false)

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
.card-action {
  display: flex;
  padding: 3px;
  border-radius: var(--radius);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease-out, color 120ms ease-out;
}
.card-action:hover {
  background-color: var(--color-surface-control-hover);
  color: var(--color-text-primary);
}
.card-action--danger:hover {
  color: var(--color-status-failed-text);
}
.card-action-icon {
  font-size: 13px;
  line-height: 1;
}
</style>
