<template>
  <div 
    class="group flex items-center px-section py-component border-b border-border-hairline hover:bg-surface-hover transition-colors duration-150 cursor-pointer relative"
    :class="{ 'bg-surface-container-low': isSelected }"
    @click="$emit('select')"
  >
    <div class="flex flex-col flex-1 min-w-0 pr-break">
      <div class="flex items-center gap-base mb-tight">
        <span class="font-mono-data text-[10px] tracking-widest px-tight text-text-faint border border-border-hairline uppercase">
          {{ item.Action || 'REVIEW' }}
        </span>
        <h3 class="font-headline text-text-primary truncate">{{ item.Title || 'Untitled Ingest Review' }}</h3>
      </div>
      
      <p class="font-body text-text-muted truncate font-mono-data text-[12px]">{{ item.SourceNodeID }}</p>
    </div>
    
    <div class="opacity-0 group-hover:opacity-100 flex items-center gap-section transition-opacity duration-200 bg-surface-hover pl-section shrink-0">
      <button @click.stop="$emit('action', 'Create Page')" class="font-mono-data text-[11px] text-text-muted hover:text-primary transition-colors">Create Page</button>
      <button @click.stop="$emit('action', 'Deep Research')" class="font-mono-data text-[11px] text-text-muted hover:text-primary transition-colors">Deep Research</button>
      <button @click.stop="$emit('action', 'Skip')" class="font-mono-data text-[11px] text-text-muted hover:text-status-failed transition-colors">Skip</button>
      <button @click.stop="$emit('action', 'Update')" class="font-mono-data text-[11px] text-text-muted hover:text-primary transition-colors">Update</button>
      <button @click.stop="$emit('action', 'Add Contradiction Callout')" class="font-mono-data text-[11px] text-text-muted hover:text-status-gate transition-colors">Add Contradiction Callout</button>
    </div>
  </div>
</template>

<script setup lang="ts">
export interface IngestReviewData {
  ID: string
  CreatedAt: string
  UpdatedAt: string
  Title: string
  SourceNodeID: string
  Action: string
  Payload: string
  ContentHash: string
  Tags: string[]
}

defineProps<{
  item: IngestReviewData
  isSelected?: boolean
}>()

defineEmits<{
  (e: 'action', action: string): void
  (e: 'select'): void
}>()
</script>
