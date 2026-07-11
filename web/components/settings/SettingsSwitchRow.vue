<template>
  <div class="setting-row">
    <div class="setting-copy">
      <p class="row-title">{{ label }}</p>
      <p class="row-description">{{ description }}</p>
      <p class="row-key">{{ settingKey }}</p>
    </div>
    <button
      type="button"
      class="switch-control"
      :class="{ 'switch-control--on': checked }"
      role="switch"
      :aria-checked="checked"
      @click="$emit('toggle')"
    >
      <span class="switch-thumb"></span>
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  description: string
  settingKey: string
  checked: boolean
}>()

defineEmits<{
  (e: 'toggle'): void
}>()
</script>
