<template>
  <div class="flex flex-col h-full bg-bg-base">
    <header class="h-rail-width w-full flex items-center px-section border-b border-border-hairline bg-surface shrink-0">
      <div class="flex items-center justify-between w-full">
        <div class="flex items-center gap-component">
          <h1 class="font-headline text-headline text-text-primary">Memory</h1>
          <span class="font-mono-data text-mono-data text-text-faint">{{ view === 'telos' ? 'Identity & goals' : 'Active claims' }}</span>
        </div>
      </div>
    </header>

    <div class="flex flex-1 overflow-hidden">
      <aside class="w-64 border-r border-border-hairline bg-surface flex flex-col shrink-0 overflow-hidden">
        <!-- Telos: the always-injected half of Memory, pinned above learned topics. -->
        <div class="p-section border-b border-border-hairline shrink-0">
          <button
            @click="selectTelos"
            class="w-full flex items-center gap-base px-tight py-1.5 rounded transition-colors text-left"
            :class="view === 'telos' ? 'bg-surface-hover text-primary' : 'text-text-muted hover:text-text-primary hover:bg-surface-hover'"
          >
            <span class="material-symbols-outlined !text-[18px]" aria-hidden="true">explore</span>
            <span class="font-body text-body">Telos</span>
            <span class="ml-auto font-mono-data text-mono-data text-text-faint">always on</span>
          </button>
        </div>

        <div class="px-section pt-section pb-base shrink-0">
          <h2 class="font-label-caps text-label-caps text-text-muted">Topics</h2>
        </div>
        <div class="flex-1 overflow-y-auto px-section pb-section flex flex-col gap-1">
          <div v-if="topicsPending" class="text-text-muted font-mono-data text-mono-data">Loading…</div>
          <div v-else-if="topicsError" class="font-mono-data text-mono-data text-status-failed-text">
            Failed to load topics.
            <button class="underline ml-1 hover:no-underline" @click="refreshTopics">Retry</button>
          </div>
          <button
            v-else
            v-for="topic in topics"
            :key="topic"
            @click="selectTopic(topic)"
            class="text-left px-tight py-1 rounded transition-colors font-body text-body truncate"
            :class="view === 'topics' && selectedTopic === topic ? 'bg-surface-hover text-primary' : 'text-text-muted hover:text-text-primary hover:bg-surface-hover'"
          >
            {{ topic }}
          </button>
          <div v-if="!topicsPending && !topicsError && topics.length === 0" class="text-text-faint font-mono-data text-mono-data">No topics</div>
        </div>
      </aside>

      <section class="flex-1 overflow-y-auto p-section relative">
        <div class="max-w-4xl mx-auto flex flex-col gap-component">
          <MemoryTelos v-if="view === 'telos'" />

          <template v-else>
          <div v-if="claimsPending" class="flex flex-col items-center justify-center py-break text-text-muted">
            <span class="material-symbols-outlined text-[32px] mb-component animate-pulse text-text-faint">memory</span>
            <p class="font-body text-body">Loading…</p>
          </div>

          <UiErrorState
            v-else-if="claimsError"
            fill
            title="Could not load claims."
            message="Check that the Kernl API is running, then retry."
            :detail="claimsError?.message ?? null"
            @retry="refreshClaims"
          />

          <div v-else-if="claims.length === 0" class="flex flex-col items-center justify-center py-break text-text-muted">
            <span class="material-symbols-outlined text-[32px] mb-component">memory</span>
            <p class="font-body text-body">No active claims for this topic</p>
          </div>
          
          <template v-else>
            <MemoryClaimCard
              v-for="claim in claims"
              :key="claim.ID || claim.id"
              :claim="claim"
              @refute="handleRefute"
            />
          </template>
          </template>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import MemoryClaimCard from '~/components/MemoryClaimCard.vue'
import MemoryTelos from '~/components/memory/MemoryTelos.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'

// Memory has two halves: Telos (always-injected identity) and learned Topics
// (relevance-retrieved claims). Identity leads — the page opens on Telos.
const view = ref<'telos' | 'topics'>('telos')

// Fetch topics — the API wraps the array as { topics: [...] }.
const { data: topicsData, pending: topicsPending, error: topicsError, refresh: refreshTopics } = useFetch<{ topics: string[] }>('/api/memory/topics', {
  server: false,
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
const { data: claimsData, pending: claimsPending, refresh: refreshClaims, error: claimsError } = useFetch<{ claims: any[] }>('/api/memory/claims', {
  server: false,
  query: computed(() => ({ topic: selectedTopic.value })),
  default: () => ({ claims: [] }),
  watch: [selectedTopic]
})

const claims = computed(() => claimsData.value?.claims || [])

const selectTopic = (topic: string) => {
  selectedTopic.value = topic
  view.value = 'topics'
}

const selectTelos = () => {
  view.value = 'telos'
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
