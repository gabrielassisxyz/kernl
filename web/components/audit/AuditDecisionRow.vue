<template>
  <div class="relative py-base">
    <div class="absolute -left-[21px] top-4 w-2.5 h-2.5 bg-status-passed rounded-full border-2 border-surface"></div>
    <div class="flex flex-col gap-1">
      <div class="flex items-center gap-component text-mono-data font-mono-data">
        <span class="text-text-muted">{{ formatTime(decision.created_at) }}</span>
        <span class="text-text-faint">ACTION:</span>
        <span class="text-text-primary">{{ decision.context }}</span>
        <span v-if="decision.related_ids && decision.related_ids.length" class="text-status-passed">
          [{{ decision.related_ids.join(', ') }}]
        </span>
      </div>
      <div class="font-body text-body text-text-primary mt-1">
        {{ decision.outcome }}
      </div>
      <div class="bg-surface-container-low border border-border-hairline rounded px-base py-tight mt-2 max-w-full overflow-x-auto">
        <pre class="font-mono-data text-mono-data text-text-muted whitespace-pre-wrap">{{ decision.body }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup>
const props = defineProps({
  decision: {
    type: Object,
    required: true
  }
})

const formatTime = (isoString) => {
  if (!isoString) return ''
  const date = new Date(isoString)
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
</script>
