<template>
  <div class="flex flex-col h-full bg-bg-base">
    <!-- Header -->
    <header class="h-rail-width w-full flex items-center px-section border-b border-border-hairline bg-surface shrink-0">
      <div class="flex items-center justify-between w-full">
        <div class="flex items-center gap-component">
          <h1 class="font-headline text-headline text-text-primary">Bookmarks</h1>
          <span class="font-mono-data text-text-faint pt-1 ml-2">{{ bookmarks.length }} items</span>
        </div>
        <div class="flex items-center gap-base">
          <span class="font-mono-data text-text-dim px-base py-1 border border-border-hairline rounded bg-surface-container-low text-[11px]">⌘ K Search</span>
          <button class="material-symbols-outlined text-text-muted hover:text-text-primary transition-colors text-[20px]">filter_list</button>
        </div>
      </div>
    </header>

    <!-- Main Split View -->
    <div class="flex flex-1 overflow-hidden">
      <!-- Left: List View -->
      <section class="w-[340px] flex-shrink-0 border-r border-border-hairline bg-bg-base overflow-y-auto hide-scrollbar flex flex-col">
        <div v-if="pending" class="p-section text-text-muted font-mono-data text-sm text-center py-break">Loading...</div>
        <div v-else-if="bookmarks.length === 0" class="flex flex-col items-center justify-center py-break text-text-muted mt-10">
          <span class="material-symbols-outlined text-[32px] mb-component opacity-50">bookmark</span>
          <p class="font-body text-[13px]">No bookmarks found</p>
        </div>
        <template v-else>
          <BookmarkItem
            v-for="(bookmark, index) in bookmarks"
            :key="bookmark.ID"
            :item="bookmark"
            :isSelected="selectedIndex === index"
            @select="selectIndex(index)"
          />
        </template>
      </section>

      <!-- Right: Reader Pane -->
      <section class="flex-1 bg-surface-container-low overflow-y-auto hide-scrollbar relative">
        <BookmarkReader 
          v-if="selectedBookmark" 
          :bookmark="selectedBookmark" 
          @highlight="handleHighlight" 
        />
        <div v-else class="flex h-full items-center justify-center text-text-muted font-mono-data text-sm">
          Select a bookmark to read
        </div>
        
        <!-- Toast for 501 / errors -->
        <div v-if="toastMessage" class="absolute bottom-section left-1/2 transform -translate-x-1/2 bg-surface-hover border border-border-hairline px-section py-component shadow-sm rounded-sm">
          <span class="font-mono-data text-text-primary">{{ toastMessage }}</span>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import BookmarkItem from '~/components/bookmarks/BookmarkItem.vue'
import BookmarkReader from '~/components/bookmarks/BookmarkReader.vue'
import type { BookmarkItemData } from '~/components/bookmarks/BookmarkItem.vue'

const { data, pending, refresh } = useFetch<BookmarkItemData[]>('/api/bookmarks', {
  server: false,
  default: () => []
})

const bookmarks = computed(() => data.value || [])
const selectedIndex = ref(0)
const selectedBookmark = computed(() => bookmarks.value[selectedIndex.value] || null)

const toastMessage = ref('')
let toastTimer: any = null

const selectIndex = (index: number) => {
  selectedIndex.value = index
}

const handleHighlight = async (highlightData: { text: string, note?: string }) => {
  if (!selectedBookmark.value) return
  
  try {
    const res = await fetch(`/api/bookmarks/${selectedBookmark.value.ID}/highlights`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(highlightData)
    })
    
    if (res.status === 501) {
      showToast('Highlighting is not implemented yet (501)')
      return
    }
    
    if (!res.ok) {
      throw new Error(`Failed with status ${res.status}`)
    }

    await refresh()
    showToast('Highlight saved successfully')
  } catch (err: any) {
    showToast(err.message || 'Error saving highlight')
  }
}

const showToast = (msg: string) => {
  toastMessage.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    toastMessage.value = ''
  }, 3000)
}

const handleKeydown = (e: KeyboardEvent) => {
  if (bookmarks.value.length === 0) return
  
  // Only handle navigation if not interacting with inputs
  if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
  
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    selectedIndex.value = (selectedIndex.value + 1) % bookmarks.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    selectedIndex.value = (selectedIndex.value - 1 + bookmarks.value.length) % bookmarks.value.length
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>
