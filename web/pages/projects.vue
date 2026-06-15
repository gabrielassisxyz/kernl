<template>
  <div class="px-margin pt-margin pb-margin">
    <!-- Surface header -->
    <header class="mb-section">
      <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">
        Projects
      </h1>
      <p class="mt-tight font-body text-body text-text-muted">
        {{ summary }}
      </p>
    </header>

    <!-- Loading -->
    <div
      v-if="loading && cards.length === 0"
      class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-section"
    >
      <div
        v-for="n in 6"
        :key="n"
        class="h-[180px] rounded-lg border border-border-hairline bg-surface animate-pulse"
      ></div>
    </div>

    <!-- Empty -->
    <div
      v-else-if="cards.length === 0"
      class="flex flex-col items-center justify-center py-break text-text-muted border border-border-hairline rounded-lg bg-surface"
    >
      <span class="material-symbols-outlined text-[32px] mb-component text-text-faint">deployed_code</span>
      <p class="font-body text-body">{{ error ? 'Could not load projects' : 'No projects yet' }}</p>
      <p v-if="!error" class="mt-tight font-body text-body text-text-faint">
        Epics from the bead graph appear here.
      </p>
    </div>

    <!-- Card grid -->
    <div v-else class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-section">
      <ProjectCard
        v-for="c in cards"
        :key="c.epic.id"
        :epic="c.epic"
        :done="c.done"
        :total="c.total"
        @open="open(c.epic.id)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useBeads } from '~/composables/useBeads'
import ProjectCard from '~/components/projects/ProjectCard.vue'

const { loading, error, load, epics, childrenOf } = useBeads()

const DONE_CHILD_STATES = new Set(['shipped', 'closed', 'done', 'abandoned'])

const cards = computed(() =>
  epics.value.map((epic) => {
    const children = childrenOf(epic.id)
    const done = children.filter((c) => DONE_CHILD_STATES.has(c.state)).length
    return { epic, done, total: children.length }
  })
)

// "N projects, M in flight" — in flight = not yet terminal.
const summary = computed(() => {
  const n = cards.value.length
  if (loading.value && n === 0) return 'Loading…'
  if (n === 0) return 'No projects yet.'
  const inFlight = cards.value.filter(
    (c) => !['shipped', 'closed', 'done', 'abandoned', 'deferred'].includes(c.epic.state)
  ).length
  const projWord = n === 1 ? 'project' : 'projects'
  return `${n} ${projWord}, ${inFlight} in flight`
})

function open(id: string) {
  navigateTo('/orchestrator?epic=' + encodeURIComponent(id))
}

onMounted(load)
</script>
