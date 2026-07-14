<template>
  <label class="flex flex-col gap-tight">
    <span v-if="label" class="font-label-caps text-label-caps text-text-muted">{{ label }}</span>
    <slot />
    <span v-if="error" class="font-mono-data text-mono-data text-status-failed-text">{{ error }}</span>
    <!-- A hint is help you read once, not a value: it sits a rung below the label
         in emphasis. text-faint is the floor here — the next rung down (text-dim)
         measures 1.93:1 against this surface and is unreadable as body text. -->
    <span v-else-if="hint" class="font-mono-data text-mono-data text-text-faint">{{ hint }}</span>
  </label>
</template>

<script setup lang="ts">
defineProps<{
  label?: string
  hint?: string
  error?: string | null
}>()
</script>
