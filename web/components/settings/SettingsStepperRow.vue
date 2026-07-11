<template>
  <div class="setting-row">
    <div class="setting-copy">
      <p class="row-title">{{ label }}</p>
      <p class="row-description">{{ description }}</p>
      <p class="row-key">{{ settingKey }}</p>
    </div>
    <div class="stepper">
      <button
        type="button"
        class="step-button"
        :disabled="value <= min"
        :aria-label="`Decrease ${label}`"
        @click="$emit('decrease')"
      >
        -
      </button>
      <span class="step-value">{{ value }}{{ suffix }}</span>
      <button
        type="button"
        class="step-button"
        :disabled="value >= max"
        :aria-label="`Increase ${label}`"
        @click="$emit('increase')"
      >
        +
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  description: string
  settingKey: string
  value: number
  suffix: string
  min: number
  max: number
}>()

defineEmits<{
  (e: 'decrease'): void
  (e: 'increase'): void
}>()
</script>
