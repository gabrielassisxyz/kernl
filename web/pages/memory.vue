<template>
  <div class="flex flex-col h-full bg-bg-base">
    <!-- Header Strip -->
    <header class="h-rail-width w-full flex items-center px-section border-b border-border-hairline bg-surface shrink-0">
      <div class="flex items-center justify-between w-full">
        <div class="flex items-center gap-component">
          <h1 class="font-headline text-headline text-text-primary">Memory</h1>
          <span class="font-mono-data text-text-faint pt-1">Active claims</span>
        </div>
        <div class="flex items-center gap-base">
          <span class="font-mono-data text-text-dim px-base border border-border-hairline rounded bg-surface-container-low">⌘ F Search</span>
        </div>
      </div>
    </header>

    <!-- Main Content -->
    <div class="flex flex-1 overflow-hidden">
      <!-- Topics Sidebar (Region 2 of inner layout) -->
      <aside class="w-64 border-r border-border-hairline bg-surface flex flex-col shrink-0 overflow-hidden">
        <div class="p-section border-b border-border-hairline shrink-0">
          <h2 class="font-headline text-[11.5px] font-semibold tracking-[0.12em] uppercase text-text-muted">Topics</h2>
        </div>
        <div class="flex-1 overflow-y-auto p-section flex flex-col gap-1 hide-scrollbar">
          <div v-if="topicsPending" class="text-text-muted font-mono-data text-[11px]">Loading topics...</div>
          <button 
            v-else
            v-for="topic in topics" 
            :key="topic"
            @click="selectTopic(topic)"
            class="text-left px-tight py-1 rounded transition-colors font-body text-[13px] truncate"
            :class="selectedTopic === topic ? 'bg-surface-hover text-primary' : 'text-text-muted hover:text-text-primary hover:bg-surface-hover'"
          >
            {{ topic }}
          </button>
          <div v-if="!topicsPending && topics.length === 0" class="text-text-dim font-mono-data text-[11px]">No topics found.</div>
        </div>
      </aside>

      <!-- Claims Queue -->
      <section class="flex-1 overflow-y-auto hide-scrollbar p-section relative">
        <div class="max-w-4xl mx-auto flex flex-col gap-component">
          <div v-if="claimsPending" class="flex flex-col items-center justify-center py-break text-text-muted">
            <span class="material-symbols-outlined text-[32px] mb-component animate-pulse text-text-faint">memory</span>
            <p class="font-body text-[13px]">Loading claims...</p>
          </div>
          
          <div v-else-if="claims.length === 0" class="flex flex-col items-center justify-center py-break text-text-muted">
            <span class="material-symbols-outlined text-[32px] mb-component">memory</span>
            <p class="font-body text-[13px]">No active claims for this topic</p>
          </div>
          
          <template v-else>
            <MemoryClaimCard 
              v-for="claim in claims" 
              :key="claim.ID || claim.id" 
              :claim="claim"
              @refute="handleRefute"
            />
          </template>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import MemoryClaimCard from '~/components/MemoryClaimCard.vue'

// Fetch topics — the API wraps the array as { topics: [...] }.
const { data: topicsData, pending: topicsPending } = useFetch<{ topics: string[] }>('/api/memory/topics', {
  default: () => ({ topics: [] })
})

const topics = computed(() => topicsData.value?.topics || [])
const selectedTopic = ref<string>('')

// Auto-select first topic when loaded
watch(topics, (newTopics) => {
  if (newTopics.length > 0 && !selectedTopic.value) {
    selectedTopic.value = newTopics[0]
  }
}, { immediate: true })

// Fetch claims based on selected topic — the API wraps as { claims: [...] }.
const { data: claimsData, pending: claimsPending, refresh: refreshClaims } = useFetch<{ claims: any[] }>('/api/memory/claims', {
  query: computed(() => ({ topic: selectedTopic.value })),
  default: () => ({ claims: [] }),
  watch: [selectedTopic]
})

const claims = computed(() => claimsData.value?.claims || [])

const selectTopic = (topic: string) => {
  selectedTopic.value = topic
}

const handleRefute = async (id: string, reason: string) => {
  try {
    await $fetch(`/api/memory/claims/${id}/refute`, {
      method: 'POST',
      body: { reason }
    })

    // Refetch the topic's active claims (the refuted one drops out server-side
    // via SynthesizeTopic's 'refutes' filter).
    await refreshClaims()
  } catch (err) {
    console.error('Failed to refute claim:', err)
    // Could add toast notification here in a full app
  }
}
</script>
