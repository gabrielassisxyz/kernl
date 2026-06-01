<template>
  <!-- Header Strip -->
  <InboxHeader :totalCount="items.length" :flaggedCount="flaggedCount" />

  <!-- Inbox Queue -->
  <section class="flex-1 overflow-y-auto hide-scrollbar relative">
    <div class="flex flex-col">
      <InboxItem 
        v-for="(item, index) in items" 
        :key="item.id" 
        :item="item" 
        :isSelected="selectedIndex === index"
        @select="selectedIndex = index"
        @action="(action) => handleAction(item.id, action)"
      />
    </div>
    
    <!-- Empty State -->
    <div v-if="!pending && items.length === 0" class="flex flex-col items-center justify-center py-break text-text-muted">
      <span class="material-symbols-outlined text-[32px] mb-component">inbox</span>
      <p class="font-body">Inbox is empty</p>
    </div>
  </section>

  <!-- Command Bar Hint -->
  <InboxHint v-if="items.length > 0" />
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import InboxHeader from '~/components/inbox/InboxHeader.vue'
import InboxItem from '~/components/inbox/InboxItem.vue'
import InboxHint from '~/components/inbox/InboxHint.vue'
import type { InboxItemData } from '~/components/inbox/InboxItem.vue'

const { data, pending, refresh } = useFetch<InboxItemData[]>('/api/inbox/pending', {
  default: () => []
})

const items = computed(() => data.value || [])
const flaggedCount = computed(() => items.value.filter(i => i.flagged).length)
const selectedIndex = ref(0)

const handleAction = async (id: string, action: 'keep' | 'convert' | 'discard') => {
  try {
    await $fetch(`/api/inbox/${id}/convert`, {
      method: 'POST',
      body: { action }
    })
    // Optimistically remove the item
    if (data.value) {
      data.value = data.value.filter(i => i.id !== id)
    }
    // Adjust selection if needed
    if (selectedIndex.value >= items.value.length) {
      selectedIndex.value = Math.max(0, items.value.length - 1)
    }
  } catch (error) {
    console.error('Failed to process item', error)
  }
}

const handleKeydown = (e: KeyboardEvent) => {
  if (items.value.length === 0) return
  
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    selectedIndex.value = (selectedIndex.value + 1) % items.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    selectedIndex.value = (selectedIndex.value - 1 + items.value.length) % items.value.length
  } else if (e.key === 'k' || e.key === 'K') {
    handleAction(items.value[selectedIndex.value].id, 'keep')
  } else if (e.key === 'c' || e.key === 'C') {
    handleAction(items.value[selectedIndex.value].id, 'convert')
  } else if (e.key === 'd' || e.key === 'D') {
    handleAction(items.value[selectedIndex.value].id, 'discard')
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>
