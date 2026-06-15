<template>
  <button
    type="button"
    class="group relative w-full text-left rounded-lg border bg-surface px-component py-base flex flex-col gap-base transition-colors duration-150 cursor-pointer overflow-hidden outline-none focus-visible:border-primary"
    :class="bead.requiresHumanAction
      ? 'border-status-gate/40 hover:border-status-gate/60 hover:bg-surface-hover'
      : 'border-border-hairline hover:border-border-default hover:bg-surface-hover'"
    @click="$emit('open', bead)"
  >
    <!-- amber gate left-edge mark -->
    <span
      v-if="bead.requiresHumanAction"
      class="absolute left-0 top-0 bottom-0 w-[2px] bg-status-gate"
    ></span>

    <h3 class="font-headline text-body font-medium text-text-primary leading-snug pr-base"
        :class="{ 'pl-tight': bead.requiresHumanAction }">
      {{ bead.title }}
    </h3>

    <div class="flex items-center gap-tight font-mono-data text-[11px]"
         :class="{ 'pl-tight': bead.requiresHumanAction }">
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
