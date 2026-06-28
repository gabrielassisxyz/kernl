<template>
  <label class="flex flex-col gap-tight">
    <span v-if="label" class="font-label-caps text-label-caps text-text-muted">{{ label }}</span>
    <slot />
    <span v-if="error" class="font-mono-data text-mono-data text-status-failed-text">{{ error }}</span>
    <span v-else-if="hint" class="font-mono-data text-mono-data text-text-muted">{{ hint }}</span>
  </label>
</template>

<script setup lang="ts">
defineProps<{
  label?: string
  hint?: string
  error?: string | null
}>()
</script>
