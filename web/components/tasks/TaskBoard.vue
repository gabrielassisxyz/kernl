<template>
  <div class="flex-1 overflow-x-auto overflow-y-hidden">
    <div class="flex gap-section h-full min-w-min px-section py-base">
      <section
        v-for="col in TASK_COLUMNS"
        :key="col.id"
        class="flex flex-col w-[300px] shrink-0 h-full"
      >
        <!-- Column header -->
        <div class="flex items-center justify-between pb-base mb-base border-b border-border-hairline">
          <h2 class="font-label-caps text-label-caps text-text-muted uppercase">{{ col.label }}</h2>
          <span class="font-mono-data text-mono-data text-text-faint">{{ grouped[col.id].length }}</span>
        </div>

        <!-- Cards -->
        <div class="flex-1 overflow-y-auto flex flex-col gap-base pb-base pr-tight">
          <TaskCard
            v-for="bead in grouped[col.id]"
            :key="bead.id"
            :bead="bead"
            @open="$emit('open', bead)"
          />
          <div
            v-if="grouped[col.id].length === 0"
            class="flex items-center justify-center py-section text-text-dim font-body text-body select-none"
          >
            —
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Bead } from '~/composables/useBeads'
import { TASK_COLUMNS, normalizeTaskStatus, type TaskStatus } from '~/utils/workflow'
import TaskCard from '~/components/tasks/TaskCard.vue'

const props = defineProps<{ tasks: Bead[] }>()

defineEmits<{ (e: 'open', bead: Bead): void }>()

// Bucket each task into its normalized column. Done column sinks closed work to
// the bottom of attention; the others keep input order from the API.
const grouped = computed<Record<TaskStatus, Bead[]>>(() => {
  const buckets: Record<TaskStatus, Bead[]> = {
    open: [],
    in_progress: [],
    blocked: [],
    done: [],
  }
  for (const bead of props.tasks) {
    buckets[normalizeTaskStatus(bead.state)].push(bead)
  }
  return buckets
})
</script>
