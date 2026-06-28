<template>
  <div class="px-margin pt-margin h-full flex flex-col">
    <header class="mb-section flex-none">
      <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Audit log</h1>
      <p class="font-body text-body text-text-muted mt-tight">Autonomous decisions, ordered by recency.</p>
    </header>

    <section class="flex-grow overflow-y-auto pb-margin">
      <div v-if="loading" class="text-text-muted font-body text-body flex items-center h-32">
        <span class="w-1.5 h-1.5 rounded-full bg-status-running animate-pulse mr-2"></span>
        Loading…
      </div>
      <div v-else-if="decisions.length === 0" class="text-text-muted font-body text-body">
        No autonomous decisions.
      </div>
      <div v-else class="flex flex-col border-t border-border-hairline">
        <div v-for="decision in decisions" :key="decision.id" class="border-b border-border-hairline py-base">
          <AuditDecisionRow :decision="decision" />
        </div>
      </div>
    </section>
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
