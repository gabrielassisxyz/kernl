<template>
  <button
    type="button"
    class="group relative w-full text-left rounded bg-transparent px-component py-base flex flex-col gap-base transition-colors duration-150 cursor-pointer overflow-hidden outline-none focus-visible:bg-surface-hover hover:bg-surface-hover"
    :class="bead.requiresHumanAction ? 'bg-status-gate/10 hover:bg-status-gate/20' : ''"
    @click="$emit('open', bead)"
  >
    <h3 class="font-headline text-body font-medium text-text-primary leading-snug pr-base">
      {{ bead.title }}
    </h3>

    <div class="flex items-center gap-tight font-mono-data text-mono-data">
      <span
        class="w-1.5 h-1.5 rounded-full shrink-0"
        :class="[dotClass, isRunning ? 'animate-pulse' : '']"
      ></span>
      <span class="text-text-faint">{{ bead.id }}</span>
      <span class="text-text-dim">·</span>
      <span class="text-text-faint truncate">{{ prettyState(bead.state) }}</span>
    </div>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Bead } from '~/composables/useBeads'
import { statusTone, statusDotClass, prettyState } from '~/utils/workflow'

const props = defineProps<{ bead: Bead }>()
defineEmits<{ (e: 'open', bead: Bead): void }>()

const tone = computed(() => statusTone(props.bead))
const dotClass = computed(() => statusDotClass(tone.value))
const isRunning = computed(() => tone.value === 'running')
</script>
