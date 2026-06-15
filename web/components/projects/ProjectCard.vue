<template>
  <button
    type="button"
    :aria-label="`Open project ${epic.title}`"
    class="group flex flex-col text-left w-full min-h-[180px] p-component rounded-lg border border-border-hairline bg-surface hover:bg-surface-hover hover:border-border-default transition-colors duration-150 cursor-pointer focus:outline-none focus-visible:border-primary/50"
    :class="{ 'border-status-gate/40 bg-status-gate/[0.03] hover:border-status-gate/50': isGate }"
    @click="$emit('open')"
  >
    <!-- Top row: status dot + title, id pinned right -->
    <div class="flex items-start gap-base">
      <span
        class="mt-[9px] w-1.5 h-1.5 rounded-full shrink-0"
        :class="dotClass"
        :title="prettyState(epic.state)"
      ></span>
      <h3 class="flex-1 min-w-0 font-headline text-headline text-text-primary font-medium leading-snug line-clamp-2">
        {{ epic.title }}
      </h3>
      <span class="shrink-0 font-mono-data text-[10px] text-text-dim pt-1 tracking-tight">
        {{ epic.id }}
      </span>
    </div>

    <!-- Description -->
    <p
      v-if="description"
      class="mt-base font-body text-body text-text-muted line-clamp-2"
    >
      {{ description }}
    </p>
    <p v-else class="mt-base font-body text-body text-text-faint italic">
      No description
    </p>

    <!-- Spacer pushes the meter to the card floor for a clean baseline grid -->
    <div class="flex-1"></div>

    <!-- Progress meter -->
    <div class="mt-component">
      <div class="flex items-center justify-between mb-tight">
        <span class="font-mono-data text-[11px] text-text-faint tracking-tight">
          {{ done }}/{{ total }}
        </span>
        <span class="font-mono-data text-[11px] tracking-tight" :class="percentTone">
          {{ total > 0 ? pct + '%' : '—' }}
        </span>
      </div>
      <div class="h-[3px] w-full rounded-full bg-surface-container-high overflow-hidden">
        <div
          class="h-full rounded-full transition-all duration-300"
          :class="barTone"
          :style="{ width: (total > 0 ? pct : 0) + '%' }"
        ></div>
      </div>
    </div>

    <!-- Footer: state literal + updatedAt -->
    <div class="mt-component flex items-center justify-between border-t border-border-hairline pt-base">
      <span class="font-mono-data text-[10px] uppercase tracking-widest text-text-faint">
        {{ epic.state }}
      </span>
      <span class="font-mono-data text-[10px] text-text-dim">
        {{ updated }}
      </span>
    </div>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Bead } from '~/composables/useBeads'
import { statusTone, statusDotClass, prettyState } from '~/utils/workflow'
import { formatRelativeTime } from '~/utils/time'

const props = defineProps<{
  epic: Bead
  done: number
  total: number
}>()

defineEmits<{ (e: 'open'): void }>()

const tone = computed(() => statusTone(props.epic))
const dotClass = computed(() => statusDotClass(tone.value))
const isGate = computed(() => tone.value === 'gate')

const description = computed(() => (props.epic.description || '').trim())

const pct = computed(() =>
  props.total > 0 ? Math.round((props.done / props.total) * 100) : 0
)

// Calm by default; only the meter carries a tone, and only for completion truth.
const barTone = computed(() => {
  if (props.total === 0) return 'bg-text-dim'
  if (props.done >= props.total) return 'bg-status-passed'
  return 'bg-status-running'
})
const percentTone = computed(() =>
  props.total > 0 && props.done >= props.total ? 'text-status-passed' : 'text-text-faint'
)

const updated = computed(() => formatRelativeTime(props.epic.updatedAt))
</script>
