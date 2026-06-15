<template>
  <div
    class="group flex flex-col gap-base p-component rounded-lg border border-border-hairline bg-surface hover:bg-surface-hover hover:border-border-default transition-colors duration-150 cursor-pointer"
    @click="$emit('open', bead)"
  >
    <div class="flex items-start gap-base">
      <span
        class="mt-[6px] w-1.5 h-1.5 rounded-full shrink-0"
        :class="[dotClass, tone === 'running' ? 'animate-pulse' : '']"
      ></span>
      <h3 class="font-headline text-text-primary leading-snug">{{ bead.title }}</h3>
    </div>

    <div class="flex items-center justify-between gap-base pl-component">
      <span class="font-mono-data text-mono-data text-text-faint">{{ bead.id }}</span>
      <span class="font-mono-data text-mono-data text-text-dim truncate">{{ prettyState(bead.state) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Bead } from '~/composables/useBeads'
import { statusTone, statusDotClass, prettyState } from '~/utils/workflow'

const props = defineProps<{ bead: Bead }>()

defineEmits<{ (e: 'open', bead: Bead): void }>()

const tone = computed(() => statusTone(props.bead))
const dotClass = computed(() => statusDotClass(tone.value))
</script>
