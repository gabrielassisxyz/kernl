<template>
  <div 
    class="group flex items-center px-section py-component border-b border-border-hairline hover:bg-surface-hover transition-colors duration-150 cursor-pointer relative"
    :class="{ 'bg-surface-container-low': isSelected }"
    @click="$emit('select')"
  >
    <div v-if="item.flagged" class="absolute left-0 top-0 bottom-0 w-[2px] bg-status-gate"></div>
    
    <div class="flex flex-col flex-1 min-w-0 pr-break">
      <div class="flex items-center gap-base mb-tight">
        <span 
          class="font-mono-data text-[10px] tracking-widest px-tight"
          :class="item.flagged ? 'text-status-gate bg-status-gate/10 border border-status-gate/20' : 'text-text-faint border border-border-hairline'"
        >
          {{ item.type || 'ITEM' }}
        </span>
        <h3 class="font-headline text-text-primary truncate">{{ item.title }}</h3>
      </div>
      
      <div v-if="item.type === 'VOICE'" class="flex items-center gap-tight">
        <span class="material-symbols-outlined text-[14px] text-primary">equalizer</span>
        <p class="font-body text-text-muted truncate">{{ item.subtitle }}</p>
      </div>
      <p v-else-if="item.type === 'SNIPPET'" class="font-body text-text-muted truncate font-mono-data text-[12px]">{{ item.subtitle }}</p>
      <p v-else class="font-body text-text-muted truncate">{{ item.subtitle }}</p>
    </div>
    
    <div class="opacity-0 group-hover:opacity-100 flex items-center gap-section transition-opacity duration-200 bg-surface-hover pl-section">
      <button @click.stop="$emit('action', 'keep')" class="font-mono-data text-text-muted hover:text-primary transition-colors">Keep</button>
      <button @click.stop="$emit('action', 'convert')" class="font-mono-data text-text-muted hover:text-primary transition-colors">Convert</button>
      <button @click.stop="$emit('action', 'discard')" class="font-mono-data text-text-muted hover:text-status-failed transition-colors">Discard</button>
    </div>
  </div>
</template>

<script setup lang="ts">
export interface InboxItemData {
  id: string
  type: string
  title: string
  subtitle: string
  flagged?: boolean
}

defineProps<{
  item: InboxItemData
  isSelected?: boolean
}>()

defineEmits<{
  (e: 'action', action: 'keep' | 'convert' | 'discard'): void
  (e: 'select'): void
}>()
</script>
