<template>
  <div class="group flex flex-col px-section py-component border-b border-border-hairline hover:bg-surface-hover transition-colors duration-150 cursor-pointer relative"
       :class="{ 'bg-surface-container-low': isSelected }"
       @click="$emit('select')">
    <div v-if="isSelected" class="absolute left-0 top-0 bottom-0 w-[2px] bg-primary"></div>
    
    <div class="flex items-center gap-base mb-tight">
      <h3 class="font-headline text-text-primary truncate font-medium">{{ item.Title || 'Untitled' }}</h3>
    </div>
    
    <div class="flex flex-col gap-tight">
      <p class="font-body text-text-muted truncate text-body">{{ item.Description || item.Excerpt || item.URL }}</p>
      <div class="flex gap-tight mt-1 items-center">
        <span class="font-mono-data text-mono-data text-text-faint truncate max-w-[200px]">{{ domain(item.URL) }}</span>
        <div v-if="item.Tags && item.Tags.length > 0" class="flex gap-tight">
          <span class="font-mono-data text-mono-data text-text-faint bg-surface border border-border-hairline px-tight py-[1px]" v-for="tag in item.Tags.slice(0, 2)" :key="tag">
            {{ tag }}
          </span>
          <span class="font-mono-data text-mono-data text-text-faint" v-if="item.Tags.length > 2">
            +{{ item.Tags.length - 2 }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
export interface BookmarkHighlight {
  text: string
  note?: string
  created_at: string
}

export interface BookmarkItemData {
  ID: string
  CreatedAt: string
  Title: string
  URL: string
  Description: string
  Excerpt: string
  Tags: string[]
  Highlights?: BookmarkHighlight[]
}

defineProps<{
  item: BookmarkItemData
  isSelected?: boolean
}>()

defineEmits<{
  (e: 'select'): void
}>()

const domain = (urlStr: string) => {
  if (!urlStr) return ''
  try {
    const url = new URL(urlStr)
    return url.hostname.replace(/^www\./, '')
  } catch (e) {
    return urlStr
  }
}
</script>
