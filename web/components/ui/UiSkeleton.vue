<template>
  <div
    aria-hidden="true"
    class="flex items-center justify-center animate-pulse rounded border border-border-hairline bg-surface-container-low"
    :class="classes"
  >
    <span v-if="text" class="font-mono-data text-mono-data text-text-faint px-component text-center">{{ text }}</span>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  classes?: string
  text?: string
}>(), {
  classes: 'h-[180px]',
  text: '',
})
</script>
