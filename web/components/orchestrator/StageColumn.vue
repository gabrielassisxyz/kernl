<template>
  <section class="flex flex-col min-w-[240px] flex-1 rounded-lg border border-border-hairline bg-surface-container-low/40">
    <!-- column header -->
    <header class="flex items-center justify-between px-component py-base border-b border-border-hairline bg-surface-container-low/40">
      <h2 class="font-label-caps text-mono-data tracking-widest text-text-muted uppercase">{{ column.label }}</h2>
      <span class="font-mono-data text-mono-data text-text-faint">{{ beads.length }}</span>
    </header>

    <!-- cards -->
    <TransitionGroup
      tag="div"
      name="card"
      class="flex flex-col gap-base p-base overflow-y-auto flex-1"
    >
      <BeadCard
        v-for="bead in beads"
        :key="bead.id"
        :bead="bead"
        @open="$emit('open', bead)"
      />
      <div
        v-if="beads.length === 0"
        key="__empty"
        class="flex items-center justify-center py-section text-text-faint font-mono-data text-mono-data"
      >
        empty
      </div>
    </TransitionGroup>
  </section>
</template>

<script setup lang="ts">
import type { Bead } from '~/composables/useBeads'
import type { OrchestratorColumn } from '~/utils/workflow'
import BeadCard from '~/components/orchestrator/BeadCard.vue'

defineProps<{
  column: OrchestratorColumn
  beads: Bead[]
}>()
defineEmits<{ (e: 'open', bead: Bead): void }>()
</script>

<style scoped>
/* Live card movement: position shifts + a quiet fade. No lift/scale/shadow. */
.card-move,
.card-enter-active,
.card-leave-active {
  transition: opacity 180ms ease, transform 180ms ease;
}
.card-enter-from,
.card-leave-to {
  opacity: 0;
}
.card-leave-active {
  position: absolute;
}
</style>
