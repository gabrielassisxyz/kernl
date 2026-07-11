<template>
  <div class="setting-row">
    <div class="setting-copy">
      <p class="row-title">{{ label }}</p>
      <p class="row-description">{{ description }}</p>
      <p class="row-key">
        {{ settingKey }}
        <span v-if="pending" class="row-pending">restart to apply</span>
      </p>
    </div>
    <div class="setting-control">
      <button
        type="button"
        class="switch-control"
        :class="{ 'switch-control--on': checked }"
        role="switch"
        :aria-checked="checked"
        :aria-label="label"
        @click="$emit('toggle')"
      >
        <span class="switch-thumb"></span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  description: string
  settingKey: string
  checked: boolean
  pending?: boolean
}>()

defineEmits<{
  (e: 'toggle'): void
}>()
</script>
