<template>
  <div class="setting-row">
    <div class="setting-copy">
      <p class="row-title">{{ item.label }}</p>
      <p class="row-description">{{ item.description }}</p>
      <p class="row-key">{{ item.key }}</p>
    </div>
    <div class="row-meta">
      <span class="setting-value" :title="item.value">{{ item.value }}</span>
      <span class="meta-chip" :class="sourceChipClass(item.source)">{{ item.source }}</span>
      <span v-if="item.restartRequired" class="meta-chip meta-chip--gate">restart required</span>
      <span v-if="item.sensitive" class="meta-chip meta-chip--danger">sensitive</span>
      <span v-if="!item.editable" class="meta-chip">{{ item.editPath }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
interface SettingItem {
  key: string
  label: string
  description: string
  value: string
  source: string
  editPath: string
  editable: boolean
  sensitive: boolean
  restartRequired: boolean
}

defineProps<{
  item: SettingItem
}>()

function sourceChipClass(source: string) {
  return {
    'meta-chip--graph': source === 'graph',
    'meta-chip--local': source === 'localStorage',
    'meta-chip--yaml': source === 'yaml',
  }
}
</script>
