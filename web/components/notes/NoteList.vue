<template>
  <div class="note-list p-4 border-t border-[#242935] bg-[#0F1217] text-[13px]">
    <h3 class="text-[11.5px] uppercase font-semibold tracking-[0.12em] text-[#666D7C] mb-4">All Notes</h3>
    <div v-if="loading" class="text-[#9098A7]">Loading...</div>
    <div v-else class="flex flex-col gap-1">
      <div v-for="file in files" :key="file" class="py-1 px-2 text-[#9098A7] hover:text-[#D6DBE3] hover:bg-[#141821] rounded-[4px] cursor-pointer truncate font-mono" @click="$emit('select', file)">
        {{ file }}
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const files = ref([])
const loading = ref(true)

defineEmits(['select'])

const refresh = async () => {
  loading.value = true
  try {
    // Flat disk-truth list so brand-new untagged notes have somewhere to appear.
    const res = await fetch('/api/vault/list')
    if (res.ok) {
      const data = await res.json()
      files.value = data.files || []
    }
  } catch (e) {
    console.error('Error fetching vault list', e)
  } finally {
    loading.value = false
  }
}

onMounted(refresh)

defineExpose({ refresh })
</script>
