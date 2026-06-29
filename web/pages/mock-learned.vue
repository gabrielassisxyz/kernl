<template>
  <!-- U9 design-validation mock. Not part of the app shell; renders the
       DA · learned Keep/Edit/Discard card in faithful DA-panel chrome with a
       live action log, so the design can be reviewed without an LLM or backend. -->
  <div class="bg-bg-base text-text-primary h-screen flex overflow-hidden font-body">
    <!-- Faux DA panel — mirrors DaChatSurface chrome. -->
    <aside class="w-[400px] max-w-[calc(100vw-60px)] flex flex-col h-full shrink-0 border-r border-border-hairline bg-surface">
      <div class="h-12 border-b border-border-hairline flex items-center justify-between px-base flex-shrink-0">
        <div class="flex items-center gap-2">
          <span class="font-headline text-text-primary font-semibold">DA</span>
          <span class="font-mono-data text-mono-data text-text-muted bg-surface-container px-1.5 py-0.5 border border-border-hairline">scope · global</span>
        </div>
        <span class="font-mono-data text-mono-data text-text-faint">mock</span>
      </div>

      <div class="flex-1 overflow-y-auto p-section flex flex-col gap-section">
        <div class="mb-2">
          <p class="font-headline text-headline text-text-primary">Afternoon, Gabriel.</p>
          <p class="font-body text-text-faint text-body mt-1">Ask me about anything in your graph.</p>
        </div>

        <!-- A short faux exchange so the card lands in real conversational context. -->
        <div class="flex flex-col gap-base rounded border border-da-accent/30 bg-da-accent/10 px-base py-base">
          <span class="font-label-caps text-label-caps text-da-accent-text">You</span>
          <p class="font-body text-body text-text-primary">{{ scenario.user }}</p>
        </div>
        <div class="flex flex-col gap-base">
          <div class="flex items-center gap-2">
            <span class="font-label-caps text-label-caps text-primary">Kernl DA</span>
          </div>
          <p class="font-body text-body text-text-primary">{{ scenario.assistant }}</p>
        </div>

        <!-- The card under review. -->
        <DaLearnedCard
          v-if="candidate"
          :key="cardKey"
          :subject="candidate.subject"
          :statement="candidate.statement"
          @keep="onKeep"
          @discard="onDiscard"
        />
        <p v-else class="font-mono-data text-mono-data text-text-faint">No candidate — a transactional turn proposes nothing.</p>
      </div>

      <div class="p-base border-t border-border-hairline bg-background">
        <div class="relative flex items-end bg-surface-overlay border border-border-hairline rounded p-2 gap-2 opacity-60">
          <textarea disabled rows="2" class="w-full bg-transparent border-none text-body text-text-primary resize-none placeholder:text-text-faint outline-none" placeholder="ask, instruct, or paste a directive…"></textarea>
        </div>
      </div>
    </aside>

    <!-- Control + log panel. -->
    <section class="flex-1 overflow-y-auto p-section">
      <div class="max-w-2xl mx-auto flex flex-col gap-component">
        <header class="flex flex-col gap-1">
          <h1 class="font-headline text-headline text-text-primary">DA · learned — design mock</h1>
          <p class="font-body text-body text-text-faint">
            Pick a scenario to populate the card on the left, then exercise Keep / Edit / Discard.
            Each action shows the exact request the real flow would POST to
            <code class="font-mono-data text-mono-data text-text-muted">/api/chat/sessions/&lt;id&gt;/learned</code>.
          </p>
        </header>

        <div class="flex flex-col gap-tight">
          <h2 class="font-label-caps text-label-caps text-text-muted">Scenarios</h2>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="(s, i) in scenarios"
              :key="i"
              @click="selectScenario(i)"
              class="px-component py-1.5 rounded border font-body text-body transition-colors"
              :class="i === activeScenario
                ? 'border-primary/60 bg-surface-hover text-text-primary'
                : 'border-border-hairline bg-surface text-text-muted hover:text-text-primary hover:bg-surface-hover'"
            >
              {{ s.label }}
            </button>
          </div>
        </div>

        <div class="flex flex-col gap-tight">
          <div class="flex items-center justify-between">
            <h2 class="font-label-caps text-label-caps text-text-muted">Action log</h2>
            <button v-if="log.length" @click="log = []" class="font-mono-data text-mono-data text-text-muted hover:text-text-primary">clear</button>
          </div>
          <div v-if="!log.length" class="font-mono-data text-mono-data text-text-faint border border-border-hairline rounded p-component bg-surface">
            No actions yet.
          </div>
          <div v-else class="flex flex-col gap-2">
            <pre
              v-for="(entry, i) in log"
              :key="i"
              class="font-mono-data text-mono-data text-text-primary border border-border-hairline rounded p-component bg-surface whitespace-pre-wrap"
            >{{ entry }}</pre>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import DaLearnedCard from '~/components/DaLearnedCard.vue'

definePageMeta({ layout: false })

interface Scenario {
  label: string
  user: string
  assistant: string
  candidate: { subject: string; statement: string } | null
}

const scenarios: Scenario[] = [
  {
    label: 'Durable preference',
    user: 'Honestly, stop suggesting Zoom — I only ever take calls on Google Meet.',
    assistant: "Got it. I'll default to Google Meet for any call you schedule.",
    candidate: { subject: 'tools', statement: 'Prefers Google Meet over Zoom for video calls.' },
  },
  {
    label: 'Standing goal',
    user: 'My whole focus this quarter is shipping the v0.1.0 magic loop, nothing else.',
    assistant: "Understood — I'll frame planning around the v0.1.0 magic loop as the priority.",
    candidate: { subject: 'goals', statement: 'This quarter the priority is shipping the v0.1.0 magic loop.' },
  },
  {
    label: 'Transactional turn (no card)',
    user: "What's the title of the note I opened a second ago?",
    assistant: 'That note is titled "Wave 2 execution state."',
    candidate: null,
  },
]

const activeScenario = ref(0)
const scenario = computed(() => scenarios[activeScenario.value])
const candidate = ref(scenarios[0].candidate)
const cardKey = ref(0)
const log = ref<string[]>([])

const selectScenario = (i: number) => {
  activeScenario.value = i
  candidate.value = scenarios[i].candidate ? { ...scenarios[i].candidate! } : null
  cardKey.value++ // remount the card so any in-progress edit resets
}

const onKeep = (statement: string) => {
  const edited = statement !== scenario.value.candidate?.statement
  log.value.unshift(
    `${edited ? 'KEEP (edited)' : 'KEEP'} → POST /learned\n` +
      JSON.stringify({ action: 'keep', subject: candidate.value?.subject, statement }, null, 2)
  )
  candidate.value = null
}

const onDiscard = () => {
  log.value.unshift(
    `DISCARD → POST /learned\n` +
      JSON.stringify({ action: 'discard', statement: candidate.value?.statement }, null, 2)
  )
  candidate.value = null
}
</script>
