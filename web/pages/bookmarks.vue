<template>
  <div class="flex flex-col h-full bg-bg-base">
    <header class="h-rail-width w-full flex items-center px-section border-b border-border-hairline bg-surface shrink-0">
      <div class="flex items-center justify-between w-full">
        <div class="flex items-center gap-component">
          <h1 class="font-headline text-headline text-text-primary">Bookmarks</h1>
          <span class="font-mono-data text-mono-data text-text-faint">{{ bookmarks.length }} items</span>
        </div>
        <div class="flex items-center gap-1 font-mono-data text-mono-data text-text-muted">
          <span class="material-symbols-outlined text-[16px]">swap_vert</span>
          <span>Use ↑/↓ to navigate</span>
        </div>
      </div>
    </header>

    <div class="flex flex-1 overflow-hidden">
      <section class="w-full max-w-[340px] flex-shrink-0 border-r border-border-hairline bg-bg-base overflow-y-auto flex flex-col">
        <div v-if="pending" class="p-section">
          <UiSkeleton classes="h-[160px]" text="Loading bookmarks..." />
        </div>
        <UiErrorState
          v-else-if="error"
          title="Could not load bookmarks."
          message="Check that the Kernl API is running, then retry."
          :detail="error?.message ?? null"
          @retry="refresh"
        />
        <UiEmptyState
          v-else-if="bookmarks.length === 0"
          icon="bookmark"
          title="No bookmarks yet."
          body="Saved bookmarks appear here for reading and highlighting."
        />
        <template v-else>
          <BookmarkItem
            v-for="(bookmark, index) in bookmarks"
            :key="bookmark.id"
            :item="bookmark"
            :isSelected="selectedIndex === index"
            @select="selectIndex(index)"
          />
        </template>
      </section>

      <section class="flex-1 bg-surface-container-low overflow-y-auto relative">
        <BookmarkReader 
          v-if="selectedBookmark" 
          :bookmark="selectedBookmark" 
          @highlight="handleHighlight" 
        />
        <div v-else class="flex h-full items-center justify-center text-text-muted font-mono-data text-sm">
          Select a bookmark to read
        </div>

        <UiToast :message="toastMessage" position="bottom-center" />
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import BookmarkItem from '~/components/bookmarks/BookmarkItem.vue'
import BookmarkReader from '~/components/bookmarks/BookmarkReader.vue'
import type { BookmarkItemData } from '~/components/bookmarks/BookmarkItem.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiSkeleton from '~/components/ui/UiSkeleton.vue'
import UiToast from '~/components/ui/UiToast.vue'

const { data, pending, refresh, error } = useFetch<BookmarkItemData[]>('/api/bookmarks', {
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
    const res = await fetch(`/api/bookmarks/${selectedBookmark.value.id}/highlights`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(highlightData)
    })
    
    if (res.status === 501) {
      showToast('Highlighting is not available yet')
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
