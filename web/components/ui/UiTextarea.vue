<template>
  <textarea
    ref="textareaEl"
    v-model="model"
    :rows="rows"
    :required="required"
    :disabled="disabled"
    :placeholder="placeholder"
    :class="classes"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue'

const model = defineModel<string>({ default: '' })
const textareaEl = ref<HTMLTextAreaElement | null>(null)

withDefaults(defineProps<{
  rows?: number | string
  placeholder?: string
  required?: boolean
  disabled?: boolean
  classes?: string
}>(), {
  rows: 3,
  placeholder: '',
  required: false,
  disabled: false,
  classes: 'w-full rounded border border-border-hairline bg-bg-base px-component py-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50 resize-none',
})

defineExpose({
  focus: () => textareaEl.value?.focus(),
})
</script>

<style scoped>
@media (pointer: coarse) {
  textarea {
    min-height: 44px;
    font-size: 16px; /* Prevents auto-zoom on iOS Safari */
  }
}
</style>
