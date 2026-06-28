<template>
  <input
    ref="inputEl"
    v-model="model"
    :type="type"
    :required="required"
    :disabled="disabled"
    :placeholder="placeholder"
    :class="classes"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'

const model = defineModel<string>({ default: '' })
const inputEl = ref<HTMLInputElement | null>(null)

withDefaults(defineProps<{
  type?: string
  placeholder?: string
  required?: boolean
  disabled?: boolean
  classes?: string
}>(), {
  type: 'text',
  placeholder: '',
  required: false,
  disabled: false,
  classes: 'h-9 w-full rounded border border-border-hairline bg-bg-base px-component font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50',
})

defineExpose({
  focus: () => inputEl.value?.focus(),
})
</script>

<style scoped>
@media (pointer: coarse) {
  input {
    min-height: 44px;
    font-size: 16px; /* Prevents auto-zoom on iOS Safari */
  }
}
</style>
