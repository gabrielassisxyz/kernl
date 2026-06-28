<template>
  <select
    ref="selectEl"
    v-model="model"
    :disabled="disabled"
    :class="classes"
  >
    <slot />
  </select>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const model = defineModel<string>({ default: '' })
const selectEl = ref<HTMLSelectElement | null>(null)

withDefaults(defineProps<{
  disabled?: boolean
  classes?: string
}>(), {
  disabled: false,
  classes: 'h-9 w-full rounded border border-border-hairline bg-bg-base px-component font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50',
})

defineExpose({
  focus: () => selectEl.value?.focus(),
})
</script>

<style scoped>
@media (pointer: coarse) {
  select {
    min-height: 44px;
    font-size: 16px; /* Prevents auto-zoom on iOS Safari */
  }
}
</style>
