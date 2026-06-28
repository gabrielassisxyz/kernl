<template>
  <button
    type="button"
    :aria-label="`Open project ${project.title}`"
    class="group flex flex-col text-left w-full min-h-[180px] p-component rounded-lg border border-border-hairline bg-surface hover:bg-surface-hover hover:border-border-default transition-colors duration-150 cursor-pointer focus:outline-none focus-visible:border-primary/50"
    @click="$emit('open')"
  >
    <!-- Top row: status dot + title -->
    <div class="flex items-start gap-base">
      <span
        class="mt-[9px] w-1.5 h-1.5 rounded-full shrink-0"
        :class="dotClass"
        :title="statusLabel"
      ></span>
      <h3 class="flex-1 min-w-0 font-headline text-headline text-text-primary font-medium leading-snug line-clamp-2">
        {{ project.title }}
      </h3>
    </div>

    <!-- Description -->
    <p
      v-if="project.description"
      class="mt-base font-body text-body text-text-muted line-clamp-2"
    >
      {{ project.description }}
    </p>
    <p v-else class="mt-base font-body text-body text-text-faint italic">
      No description
    </p>

    <div class="flex-1"></div>

    <!-- Task progress meter -->
    <div class="mt-component">
      <div class="flex items-center justify-between mb-tight">
        <span class="font-mono-data text-mono-data text-text-faint tracking-tight">
          {{ project.doneCount }}/{{ project.taskCount }} tasks
        </span>
        <span class="font-mono-data text-mono-data tracking-tight" :class="percentTone">
          {{ project.taskCount > 0 ? pct + '%' : '—' }}
        </span>
      </div>
      <div class="h-[3px] w-full rounded-full bg-surface-container-high overflow-hidden">
        <div
          class="h-full rounded-full transition-all duration-300"
          :class="barTone"
          :style="{ width: (project.taskCount > 0 ? pct : 0) + '%' }"
        ></div>
      </div>
    </div>

    <!-- Footer: status + updatedAt -->
    <div class="mt-component flex items-center justify-between border-t border-border-hairline pt-base">
      <span class="font-mono-data text-mono-data uppercase tracking-widest text-text-faint">
        {{ statusLabel }}
      </span>
      <span class="font-mono-data text-mono-data text-text-faint">
        {{ updated }}
      </span>
    </div>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Project, ProjectStatus } from '~/composables/useProjects'
import { PROJECT_STATUSES } from '~/composables/useProjects'
import { formatRelativeTime } from '~/utils/time'

const props = defineProps<{ project: Project }>()
defineEmits<{ (e: 'open'): void }>()

const STATUS_DOT: Record<ProjectStatus, string> = {
  active: 'bg-status-active',
  paused: 'bg-status-gate',
  done: 'bg-status-passed',
  archived: 'bg-text-faint',
}

const dotClass = computed(() => STATUS_DOT[props.project.status] ?? 'bg-text-dim')
const statusLabel = computed(
  () => PROJECT_STATUSES.find((s) => s.id === props.project.status)?.label ?? props.project.status
)

const pct = computed(() =>
  props.project.taskCount > 0
    ? Math.round((props.project.doneCount / props.project.taskCount) * 100)
    : 0
)
const barTone = computed(() => {
  if (props.project.taskCount === 0) return 'bg-text-dim'
  if (props.project.doneCount >= props.project.taskCount) return 'bg-status-passed'
  return 'bg-status-running'
})
const percentTone = computed(() =>
  props.project.taskCount > 0 && props.project.doneCount >= props.project.taskCount
    ? 'text-status-passed'
    : 'text-text-faint'
)
const updated = computed(() => formatRelativeTime(props.project.updatedAt))
</script>
