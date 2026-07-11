<template>
  <div class="setting-row" :class="{ 'setting-row--stacked': stacked }">
    <div class="setting-copy">
      <label class="row-title" :for="inputId">{{ label }}</label>
      <p class="row-description">{{ description }}</p>
      <p class="row-key">
        {{ settingKey }}
        <span v-if="pending" class="row-pending">restart to apply</span>
      </p>
    </div>
    <div class="setting-control">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  inputId: string
  label: string
  description: string
  settingKey: string
  pending?: boolean
  stacked?: boolean
}>()
</script>
