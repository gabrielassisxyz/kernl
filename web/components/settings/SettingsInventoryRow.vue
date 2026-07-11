<template>
  <div class="setting-row">
    <div class="setting-copy">
      <p class="row-title">{{ row.label }}</p>
      <p class="row-description">{{ row.description }}</p>
      <p class="row-key">{{ row.key }}</p>
    </div>
    <div class="row-meta">
      <span class="setting-value" :title="row.value">{{ row.value }}</span>
      <span class="meta-chip" :title="row.reason">read-only</span>
    </div>
  </div>
</template>

<script setup lang="ts">
interface ReadOnlySettingRow {
  key: string
  label: string
  description: string
  value: string
  source: string
  reason: string
}

defineProps<{
  row: ReadOnlySettingRow
}>()
</script>
