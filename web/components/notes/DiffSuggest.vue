<template>
  <div class="absolute top-component right-component z-modal flex max-h-[70vh] w-80 flex-col overflow-hidden rounded border border-border-default bg-surface-overlay">
    <div class="bg-bg-elevated border-b border-border-hairline px-4 py-2 text-label-caps uppercase font-semibold tracking-[0.12em] text-text-muted flex justify-between items-center">
      <span>DA Suggestion</span>
      <span class="bg-surface-hover text-da-accent-text font-mono px-1.5 py-0.5 rounded-sm">{{ hunks.length }}</span>
    </div>
    <div class="overflow-y-auto">
      <div v-for="hunk in hunks" :key="hunk.id" class="p-component border-b border-border-hairline last:border-0 relative">
        <div class="text-body text-text-primary bg-surface-hover p-3 rounded font-mono mb-3 overflow-x-auto whitespace-pre">
          <span class="text-status-passed">+ {{ hunk.content }}</span>
        </div>
        <div class="flex gap-2">
          <button @click="$emit('accept', hunk)" class="flex-1 bg-status-passed/10 border border-status-passed/40 text-status-passed hover:bg-status-passed/15 px-3 py-1.5 rounded text-sm transition-colors">Apply edit</button>
          <button @click="$emit('reject', hunk)" class="flex-1 bg-transparent border border-border-default text-text-muted hover:bg-surface-hover px-3 py-1.5 rounded text-sm transition-colors">Discard edit</button>
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
