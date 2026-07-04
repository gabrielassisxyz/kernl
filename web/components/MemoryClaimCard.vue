<template>
  <div class="group flex flex-col p-component border border-border-hairline rounded bg-surface hover:bg-surface-hover hover:border-primary transition-colors duration-150 relative">
    <div class="flex items-start justify-between gap-break">
      <!-- Content -->
      <div class="flex-1 min-w-0">
        <p class="font-body text-text-primary text-body leading-relaxed">{{ claim.Statement || claim.statement || claim.Title || claim.title }}</p>
      </div>
      
      <!-- Actions -->
      <div class="flex items-center shrink-0">
        <button @click="isRefuting = true" v-if="!isRefuting" class="font-mono-data text-mono-data text-text-muted hover:text-status-failed-text transition-colors px-2 py-1 rounded bg-transparent hover:bg-surface-container-low border border-transparent hover:border-border-hairline outline-none focus-visible:ring-1 focus-visible:ring-primary/30 focus-visible:border-primary/70">
          Refute
        </button>
      </div>
    </div>

    <!-- Provenance -->
    <div class="mt-component pt-component border-t border-border-hairline">
      <div class="flex items-center gap-component text-mono-data font-mono-data text-text-faint">
        <span class="flex items-center gap-1"><span class="material-symbols-outlined text-body">source</span> {{ provenanceLabel }}</span>
        <span v-if="claim.Confidence || claim.confidence != null" class="flex items-center gap-1"><span class="material-symbols-outlined text-body">analytics</span> {{(claim.Confidence || claim.confidence) * 100}}%</span>
        <span>{{ claim.ID || claim.id }}</span>
        <span v-if="claim.CreatedAt || claim.createdAt">{{ formatDate(claim.CreatedAt || claim.createdAt) }}</span>
      </div>
    </div>

    <!-- Refute Inline Form -->
    <div v-if="isRefuting" class="mt-component pt-component border-t border-border-hairline flex flex-col gap-tight">
      <UiInput
        v-model="refuteReason" 
        placeholder="Reason for refutation..." 
        classes="h-9 w-full rounded border border-border-hairline bg-surface-container-low px-component font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50 custom-caret"
        @keyup.enter="submitRefute"
        @keyup.escape="cancelRefute"
        autofocus
      />
      <div class="flex items-center justify-end gap-2 mt-2">
        <UiButton variant="ghost" size="xs" @click="cancelRefute">Cancel</UiButton>
        <UiButton variant="danger" size="xs" :disabled="!refuteReason.trim()" @click="submitRefute">Submit</UiButton>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiInput from '~/components/ui/UiInput.vue'

const props = defineProps<{
  claim: any
}>()

// Map the raw source attr to human-readable provenance.
const provenanceLabel = computed(() => {
  const src = (props.claim.Source || props.claim.source || '').toLowerCase()
  if (src === 'user') return 'You'
  if (src === 'da') return 'DA suggestion'
  return src || 'Unknown'
})

const emit = defineEmits<{
  (e: 'refute', id: string, reason: string): void
}>()

const isRefuting = ref(false)
const refuteReason = ref('')

const submitRefute = () => {
  if (!refuteReason.value.trim()) return
  const id = props.claim.ID || props.claim.id
  emit('refute', id, refuteReason.value.trim())
  isRefuting.value = false
  refuteReason.value = ''
}

const cancelRefute = () => {
  isRefuting.value = false
  refuteReason.value = ''
}

const formatDate = (dateStr: string) => {
  if (!dateStr) return ''
  try {
    const d = new Date(dateStr)
    return d.toISOString().slice(0, 19).replace('T', ' ')
  } catch {
    return dateStr
  }
}
</script>
