<template>
  <div class="bg-[#141821] p-4 border-b border-[#1B2029]">
    <h3 class="text-[11.5px] uppercase font-semibold tracking-[0.12em] text-[#666D7C] mb-3">Properties</h3>
    <div class="grid grid-cols-2 gap-x-6 gap-y-2 text-[13px]">
      <div v-for="(val, key) in displayData" :key="key" class="flex gap-4">
        <span class="text-[#9098A7] w-24 shrink-0 font-medium">{{ key }}</span>
        <span class="text-[#D6DBE3] truncate">{{ val }}</span>
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
