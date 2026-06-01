<template>
  <div class="group flex flex-col p-component border border-border-hairline rounded bg-surface hover:bg-surface-hover hover:border-primary transition-colors duration-150 relative">
    <div class="flex items-start justify-between gap-break">
      <!-- Content -->
      <div class="flex-1 min-w-0">
        <p class="font-body text-text-primary text-[13px] leading-relaxed">{{ claim.Statement || claim.statement || claim.Title || claim.title }}</p>
      </div>
      
      <!-- Actions -->
      <div class="opacity-0 group-hover:opacity-100 flex items-center transition-opacity duration-200 shrink-0">
        <button @click="isRefuting = true" v-if="!isRefuting" class="font-mono-data text-[12px] text-text-muted hover:text-status-failed transition-colors px-2 py-1 rounded bg-transparent hover:bg-surface-container-low border border-transparent hover:border-border-hairline">
          Refute
        </button>
      </div>
    </div>

    <!-- Provenance (Hover/Expand) -->
    <div class="mt-component pt-component border-t border-border-hairline opacity-0 h-0 overflow-hidden group-hover:opacity-100 group-hover:h-auto transition-all duration-200">
      <div class="flex items-center gap-component text-[11px] font-mono-data text-text-faint">
        <span class="flex items-center gap-1"><span class="material-symbols-outlined text-[14px]">source</span> {{ claim.Source || claim.source || 'Unknown' }}</span>
        <span v-if="claim.Confidence || claim.confidence != null" class="flex items-center gap-1"><span class="material-symbols-outlined text-[14px]">analytics</span> {{(claim.Confidence || claim.confidence) * 100}}%</span>
        <span>{{ claim.ID || claim.id }}</span>
        <span v-if="claim.CreatedAt || claim.createdAt">{{ formatDate(claim.CreatedAt || claim.createdAt) }}</span>
      </div>
    </div>

    <!-- Refute Inline Form -->
    <div v-if="isRefuting" class="mt-component pt-component border-t border-border-hairline flex flex-col gap-tight">
      <input 
        v-model="refuteReason" 
        type="text" 
        placeholder="Reason for refutation..." 
        class="w-full bg-surface-container-low border border-border-hairline rounded px-3 py-2 text-[13px] font-body text-text-primary focus:outline-none focus:border-primary transition-colors custom-caret"
        @keyup.enter="submitRefute"
        @keyup.escape="cancelRefute"
        autofocus
      />
      <div class="flex items-center justify-end gap-2 mt-2">
        <button @click="cancelRefute" class="font-mono-data text-[11px] text-text-muted hover:text-text-primary px-2 py-1 transition-colors">Cancel</button>
        <button @click="submitRefute" :disabled="!refuteReason.trim()" class="font-mono-data text-[11px] bg-status-failed text-[#F2F2EF] px-3 py-1 rounded hover:opacity-90 disabled:opacity-50 transition-colors">Submit</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  claim: any
}>()

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
