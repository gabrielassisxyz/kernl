<template>
  <aside 
    class="z-modal flex flex-col overflow-hidden border-border-hairline bg-surface transition-all duration-200 ease-out flex-shrink-0 fixed lg:static top-0 right-0 bottom-[26px] lg:bottom-auto lg:h-full max-w-[calc(100vw-60px)]"
    :class="isOpen
      ? 'translate-x-0 opacity-100 w-[400px] border-l'
      : 'pointer-events-none translate-x-full opacity-0 lg:translate-x-0 lg:opacity-100 w-[400px] lg:w-0 border-l lg:border-l-0'"
  >
    <div class="w-[400px] max-w-[calc(100vw-60px)] flex flex-col h-full shrink-0">
      <div class="h-12 border-b border-border-hairline flex items-center justify-between px-base flex-shrink-0">
      <div class="flex items-center gap-2">
        <span class="font-headline text-text-primary font-semibold">DA</span>
        <span class="font-mono-data text-mono-data text-text-muted bg-surface-container px-1.5 py-0.5 border border-border-hairline">scope · global</span>
      </div>
      <UiIconButton icon="close" label="Close DA" @click="$emit('close')" />
    </div>

    <div class="flex-1 overflow-y-auto p-section flex flex-col gap-section">
      <div class="mb-4">
        <p class="font-headline text-headline text-text-primary">{{ daGreeting }}</p>
        <p class="font-body text-text-faint text-body mt-1">Ask me about anything in your graph.</p>
      </div>
      
      <div class="flex flex-col gap-section w-full">
        <div v-for="(msg, idx) in messages" :key="idx" class="flex flex-col gap-base">
          <template v-if="msg.role === 'assistant'">
            <div class="flex flex-col gap-base">
              <div class="flex items-center gap-2">
                <span class="font-label-caps text-label-caps text-primary">Kernl DA</span>
              </div>
              <div class="font-body text-body text-text-primary space-y-4">
                <p>{{ msg.content }}</p>
              </div>
            </div>
          </template>
          
          <template v-else>
            <div class="flex flex-col gap-base rounded border border-da-accent/30 bg-da-accent/10 px-base py-base">
              <span class="font-label-caps text-label-caps text-da-accent-text">You</span>
              <p class="font-body text-body text-text-primary">{{ msg.content }}</p>
            </div>
          </template>
        </div>
      </div>
      
      <div v-if="isStreaming" class="flex flex-col gap-base">
        <div class="flex items-center gap-2">
          <span class="font-label-caps text-label-caps text-primary">Kernl DA</span>
        </div>
        <div class="font-body text-body leading-relaxed text-text-primary">
          <span class="inline-block w-[2px] h-[1.1em] bg-da-accent align-middle ml-[2px] animate-blink"></span>
        </div>
      </div>

      <DaLearnedCard
        v-if="learnedCandidate"
        :subject="learnedCandidate.subject"
        :statement="learnedCandidate.statement"
        @keep="keepCandidate"
        @discard="discardCandidate"
      />

      <!-- DA-proposed note edit: the DA never writes; the user accepts or
           rejects each hunk here and only then does it touch the note. -->
      <div v-if="diffSuggestion" class="border border-da-accent/40 rounded bg-da-accent/[0.04] p-base flex flex-col gap-base">
        <div class="flex items-center gap-2">
          <span class="material-symbols-outlined !text-[16px] text-da-accent-text">edit_note</span>
          <span class="font-mono-data text-mono-data text-da-accent-text">Proposed edit · {{ diffNoteName }}</span>
        </div>
        <div
          v-for="hunk in pendingHunks"
          :key="hunk.id"
          class="rounded border border-border-hairline bg-surface p-2 font-mono-data text-mono-data text-status-passed whitespace-pre-wrap overflow-x-auto"
        >+ {{ hunk.content }}</div>
        <div class="flex justify-end gap-base">
          <UiButton variant="ghost" size="xs" @click="rejectDiff">Reject</UiButton>
          <UiButton variant="primary" size="xs" @click="acceptDiff">Apply edit</UiButton>
        </div>
      </div>

      <UiErrorState
        v-if="error"
        bordered
        title="DA stream failed."
        message="The current chat stream stopped before completing."
        :detail="error"
      />
    </div>
    
    <div class="p-base border-t border-border-hairline bg-background">
      <div class="relative flex items-end bg-surface-overlay border border-border-hairline rounded focus-within:border-primary transition-colors p-2 gap-2">
        <textarea
          v-model="daInput" 
          @keydown.enter.prevent="handleSend"
          :disabled="isStreaming"
          class="w-full bg-transparent border-none focus:ring-0 text-body text-text-primary resize-none placeholder:text-text-faint custom-caret outline-none" 
          placeholder="ask, instruct, or paste a directive…" 
          rows="3"
        ></textarea>
        <UiIconButton icon="arrow_upward" label="Send to DA" variant="primary" @click="handleSend" />
      </div>
      <div class="mt-2 flex justify-between items-center px-1">
        <span class="font-mono-data text-mono-data text-text-muted">Enter to send</span>
        <span class="font-mono-data text-mono-data text-text-muted">Context: All files</span>
      </div>
    </div>
    </div>
  </aside>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useChatSession } from '~/composables/useChatSession'
import DaLearnedCard from '~/components/DaLearnedCard.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiIconButton from '~/components/ui/UiIconButton.vue'

defineProps({
  isOpen: Boolean
})

const daGreeting = computed(() => {
  const h = new Date().getHours()
  const tod = h < 12 ? 'Morning' : h < 18 ? 'Afternoon' : 'Evening'
  return `${tod}, Gabriel.`
})
defineEmits(['close'])

const daInput = ref('')
const {
  messages, error, isStreaming, learnedCandidate, diffSuggestion,
  sendMessage, keepCandidate, discardCandidate, applyDiff, dismissDiff,
} = useChatSession()

const handleSend = () => {
  if (daInput.value.trim() && !isStreaming.value) {
    sendMessage(daInput.value)
    daInput.value = ''
  }
}

const pendingHunks = computed(() => diffSuggestion.value?.hunks || [])
const diffNoteName = computed(() => {
  const p = diffSuggestion.value?.notePath || ''
  return (p.split('/').pop() || p).replace(/\.md$/, '')
})

// Apply every proposed hunk; reject drops the suggestion untouched. (The DA
// currently proposes a single whole-body hunk, so this is accept-all / reject.)
const acceptDiff = () => applyDiff(pendingHunks.value)
const rejectDiff = () => dismissDiff()
</script>

<style scoped>
@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}
.animate-blink {
  animation: blink 1s step-end infinite;
}
</style>
