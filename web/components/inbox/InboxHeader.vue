<template>
  <header class="h-rail-width w-full flex items-center px-section border-b border-border-hairline bg-surface shrink-0">
    <div class="flex items-center justify-between w-full">
      <div class="flex items-center gap-component">
        <h1 class="font-headline text-headline text-text-primary">Inbox</h1>
        <span class="font-mono-data text-text-faint pt-1">{{ totalCount }} items · {{ flaggedCount }} flagged by DA</span>
      </div>
      <div class="flex items-center gap-base">
        <span class="font-mono-data text-text-faint px-base border border-border-hairline rounded bg-surface-container-low">⌘ K Search</span>
        <button class="material-symbols-outlined text-text-muted hover:text-text-primary transition-colors" aria-label="Filter list">filter_list</button>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
defineProps<{
  totalCount: number
  flaggedCount: number
}>()
</script>
