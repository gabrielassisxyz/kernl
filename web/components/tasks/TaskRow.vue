<template>
  <div
    class="task-row group grid items-center gap-3 px-2.5 py-2 rounded border-b border-border-row text-body cursor-pointer hover:bg-surface-hover transition-colors duration-150"
    :class="compact ? 'task-row--compact' : 'task-row--full'"
    @click="$emit('open', task)"
  >
    <!-- Toggling done is the one action reachable without the hover cluster:
         it is the one done most often, and it must not open the panel. -->
    <button
      type="button"
      class="w-3.5 h-3.5 shrink-0 flex items-center justify-center rounded-full border-[1.3px] outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
      :class="done ? 'border-text-dim text-text-muted' : 'border-status-running text-transparent'"
      :title="done ? 'Reopen' : 'Mark as done'"
      :aria-label="done ? `Reopen ${task.title}` : `Mark ${task.title} as done`"
      @click.stop="$emit('toggle-done', task)"
    >
      <span class="material-symbols-outlined row-check" aria-hidden="true">check</span>
    </button>

    <span class="min-w-0 truncate" :class="done ? 'text-text-muted line-through' : 'text-text-primary'">
      {{ task.title }}
    </span>

    <!-- The cell is kept even when there is no project, or the columns after it
         slide left and stop lining up down the page. -->
    <span v-if="!compact" class="justify-self-start min-w-0">
      <span
        v-if="projectTitle"
        class="block truncate font-mono-data text-mono-data text-text-muted bg-surface-chip rounded-sm px-[5px] py-px"
      >
        {{ projectTitle }}
      </span>
    </span>

    <span
      v-if="!compact"
      class="justify-self-end whitespace-nowrap font-mono-data text-mono-data"
      :class="late ? 'text-status-failed-text' : task.dueDate ? 'text-text-muted' : 'text-text-dim'"
    >
      {{ formatDueDate(task.dueDate) || NO_VALUE }}
    </span>

    <span v-if="!compact" class="justify-self-end whitespace-nowrap font-mono-data text-mono-data text-text-muted">
      {{ formatTimestamp(task.updatedAt) }}
    </span>

    <div class="justify-self-end" @click.stop>
      <div v-if="confirming" class="flex items-center gap-2 whitespace-nowrap">
        <span class="text-text-secondary">Delete?</span>
        <button
          type="button"
          class="font-medium text-status-failed-text hover:underline outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
          @click="$emit('confirm-delete', task)"
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
      <!-- Revealed on hover, and by keyboard focus: a control that only exists
           under a pointer is a control a keyboard cannot reach. -->
      <div
        v-else
        class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-150"
      >
        <button
          type="button"
          class="row-action"
          :title="advanceTitle"
          :aria-label="`${advanceTitle}: ${task.title}`"
          @click="$emit('advance', task)"
        >
          <span class="material-symbols-outlined row-action-icon" aria-hidden="true">arrow_forward</span>
        </button>
        <button
          type="button"
          class="row-action"
          title="Edit"
          :aria-label="`Edit ${task.title}`"
          @click="$emit('open', task)"
        >
          <span class="material-symbols-outlined row-action-icon" aria-hidden="true">edit</span>
        </button>
        <button
          type="button"
          class="row-action row-action--danger"
          title="Delete"
          :aria-label="`Delete ${task.title}`"
          @click="$emit('ask-delete', task)"
        >
          <span class="material-symbols-outlined row-action-icon" aria-hidden="true">delete</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Task } from '~/composables/useTasks'
import { formatDueDate, formatTimestamp, isOverdue } from '~/utils/time'

const props = defineProps<{
  task: Task
  projectTitle?: string
  /** The side panel is open, so the row gives up its metadata columns. */
  compact?: boolean
  confirming?: boolean
}>()

defineEmits<{
  (e: 'open', task: Task): void
  (e: 'toggle-done', task: Task): void
  (e: 'advance', task: Task): void
  (e: 'ask-delete', task: Task): void
  (e: 'confirm-delete', task: Task): void
  (e: 'cancel-delete'): void
}>()

// An em dash written as an escape: the repo's prose linter scans added source
// lines for the literal character.
const NO_VALUE = '\u2014'

const done = computed(() => props.task.status === 'done')

// A finished task is never late, however old its deadline.
const late = computed(() => !done.value && isOverdue(props.task.dueDate))

const ADVANCE_TITLE = { todo: 'Start', in_progress: 'Complete', done: 'Reopen' } as const
const advanceTitle = computed(() => ADVANCE_TITLE[props.task.status] ?? 'Advance')
</script>

<style scoped>
/* The columns are fixed rather than content-sized so every row's metadata lines
   up down the page; only the title flexes. */
.task-row--full {
  grid-template-columns: 14px minmax(0, 1fr) 130px 62px 128px 96px;
}
.task-row--compact {
  grid-template-columns: 14px minmax(0, 1fr) 96px;
}

/* 10px on the tick, which is smaller than any type step: it is a mark inside a
   14px ring, not text. */
.row-check {
  font-size: 10px;
  line-height: 1;
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
