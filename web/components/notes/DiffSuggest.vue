<template>
  <div class="absolute top-4 right-4 w-80 bg-surface-overlay border border-border-default rounded-md shadow-xl z-40 overflow-hidden flex flex-col max-h-[70vh]">
    <div class="bg-bg-elevated border-b border-border-hairline px-4 py-2 text-[11.5px] uppercase font-semibold tracking-[0.12em] text-text-muted flex justify-between items-center">
      <span>DA Suggestion</span>
      <span class="bg-surface-hover text-da-accent font-mono px-1.5 py-0.5 rounded-sm">{{ hunks.length }}</span>
    </div>
    <div class="overflow-y-auto">
      <div v-for="hunk in hunks" :key="hunk.id" class="p-4 border-b border-border-hairline last:border-0 relative">
        <div class="absolute -left-1 top-4 bottom-4 w-1 bg-da-accent rounded-r-sm"></div>
        <div class="text-[13px] text-text-primary bg-surface-hover p-3 rounded font-mono mb-3 overflow-x-auto whitespace-pre">
          <span class="text-status-passed">+ {{ hunk.content }}</span>
        </div>
        <div class="flex gap-2">
          <button @click="$emit('accept', hunk)" class="flex-1 bg-surface-hover border border-border-default text-text-primary hover:bg-status-passed hover:text-on-surface hover:border-transparent px-3 py-1.5 rounded-[4px] text-sm transition-colors">Accept</button>
          <button @click="$emit('reject', hunk)" class="flex-1 bg-transparent border border-border-default text-text-muted hover:bg-surface-hover px-3 py-1.5 rounded-[4px] text-sm transition-colors">Reject</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  hunks: Array
})
</script>
