<template>
  <div class="border-t border-da-accent/30 bg-da-accent/[0.04] px-component py-base">
    <div class="flex items-center gap-tight mb-tight">
      <span class="font-mono-data text-mono-data text-da-accent-text">DA briefing</span>
      <button
        class="ml-auto font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
        @click="$emit('close')"
      >close</button>
    </div>
    <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ body || '…' }}</p>
  </div>
</template>

<script setup lang="ts">
defineProps<{ body: string }>()
defineEmits<{ (e: 'close'): void }>()
</script>
