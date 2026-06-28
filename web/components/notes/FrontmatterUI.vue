<template>
  <div class="bg-bg-elevated p-component border-b border-border-hairline">
    <h3 class="font-label-caps text-label-caps text-text-muted mb-base">Properties</h3>
    <div class="grid grid-cols-2 gap-x-6 gap-y-2 text-body">
      <div v-for="(val, key) in displayData" :key="key" class="flex gap-4">
        <span class="text-text-muted w-24 shrink-0 font-medium">{{ key }}</span>
        <span class="text-text-primary truncate">{{ val }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  data: Object
})

const displayData = computed(() => {
  const filtered = { ...props.data }
  delete filtered.uuid // hide UUID
  return filtered
})
</script>
