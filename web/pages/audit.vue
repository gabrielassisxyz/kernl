<template>
  <div class="px-margin pt-margin h-screen flex flex-col">
    <header class="mb-section flex-none">
      <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Autonomous Audit Log</h1>
      <p class="font-body text-body text-text-muted mt-tight">Decisions made by autonomous agents without human intervention.</p>
    </header>

    <div class="flex-grow flex flex-col overflow-hidden pb-margin">
      <section class="bg-surface border border-border-hairline rounded-lg flex flex-col h-full">
        <div class="px-component py-base border-b border-border-hairline flex items-center">
          <h2 class="font-label-caps text-label-caps text-text-muted flex-grow">AUTONOMOUS DECISIONS</h2>
        </div>
        
        <div class="flex-grow overflow-y-auto p-component">
          <div v-if="loading" class="text-text-muted font-body text-body flex items-center justify-center h-32">
            <span class="w-1.5 h-1.5 rounded-full bg-status-running animate-pulse mr-2"></span>
            Loading decisions...
          </div>
          <div v-else-if="decisions.length === 0" class="text-text-muted font-body text-body">
            No autonomous decisions found.
          </div>
          <div v-else class="flex flex-col border-l border-border-hairline ml-base pl-base">
            <AuditDecisionRow 
              v-for="decision in decisions" 
              :key="decision.id" 
              :decision="decision" 
            />
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import AuditDecisionRow from '~/components/audit/AuditDecisionRow.vue'

const decisions = ref([])
const loading = ref(true)

const fetchDecisions = async () => {
  try {
    const response = await fetch('/api/audit/decisions')
    if (response.ok) {
      decisions.value = await response.json()
    }
  } catch (err) {
    console.error('Failed to fetch decisions', err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchDecisions()
})
</script>
