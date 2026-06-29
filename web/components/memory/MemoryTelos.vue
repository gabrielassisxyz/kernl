<template>
  <div class="flex flex-col gap-section">
    <!-- Loading -->
    <div v-if="pending" class="flex flex-col gap-component">
      <UiSkeleton class="h-5 w-64" />
      <UiSkeleton class="h-28 w-full" />
      <UiSkeleton class="h-28 w-full" />
    </div>

    <!-- Error -->
    <UiErrorState
      v-else-if="error"
      fill
      title="Could not load Telos."
      message="Check that the Kernl API is running, then retry."
      :detail="error?.message ?? null"
      @retry="refresh"
    />

    <!-- Empty: teach what Telos is, then offer the first step. -->
    <UiEmptyState
      v-else-if="notes.length === 0"
      icon="explore"
      title="Your Telos is empty"
      body="Telos is what your DA always knows about you — your identity, values, and long-term goals. It's injected into every conversation, not retrieved when relevant. Write one note and the DA carries it everywhere."
      action-label="New Telos note"
      @action="newTelos"
    />

    <!-- Loaded -->
    <template v-else>
      <!-- The mechanism, made visible: what the DA always sees, and its footprint. -->
      <div class="flex flex-wrap items-center justify-between gap-component">
        <div class="flex items-center gap-base text-text-muted">
          <span class="telos-live-dot" aria-hidden="true"></span>
          <span class="font-body text-body">Always in context — injected into every DA turn.</span>
          <span class="font-mono-data text-mono-data text-text-faint">{{ footprint }}</span>
        </div>
        <UiButton variant="secondary" size="sm" icon="add" @click="newTelos">New Telos note</UiButton>
      </div>

      <!-- Over-cap warning: the DA is only seeing part of this. -->
      <div
        v-if="injection.truncated"
        class="flex items-start gap-base rounded border border-status-gate/40 bg-status-gate/10 px-component py-base"
      >
        <span class="material-symbols-outlined !text-[18px] text-status-gate mt-px" aria-hidden="true">warning</span>
        <p class="font-body text-body text-text-primary">
          Telos exceeds the {{ capKb }} KB injection cap — only the first part reaches the DA.
          Trim or split your Telos notes so nothing important is cut.
        </p>
      </div>

      <ul class="flex flex-col gap-component">
        <li
          v-for="note in notes"
          :key="note.id"
          class="group flex flex-col gap-base rounded border border-border-hairline bg-surface p-component transition-colors duration-150 hover:border-border-default"
        >
          <div class="flex items-start justify-between gap-component">
            <h3 class="font-headline text-headline text-text-primary">{{ note.title || 'Untitled' }}</h3>
            <UiButton
              v-if="note.path"
              variant="ghost"
              size="xs"
              icon="open_in_new"
              class="shrink-0 opacity-0 transition-opacity duration-150 group-hover:opacity-100 focus-visible:opacity-100"
              @click="edit(note)"
            >
              Edit
            </UiButton>
          </div>
          <p class="max-w-[68ch] whitespace-pre-wrap font-body text-body leading-relaxed text-text-muted">{{ bodyPreview(note.body) }}</p>
        </li>
      </ul>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiSkeleton from '~/components/ui/UiSkeleton.vue'

interface TelosNote {
  id: string
  title: string
  body: string
  path: string
  updatedAt: string
}
interface Injection {
  bytes: number
  capBytes: number
  truncated: boolean
}

const { data, pending, error, refresh } = useFetch<{ notes: TelosNote[]; injection: Injection }>(
  '/api/memory/telos',
  { server: false, default: () => ({ notes: [], injection: { bytes: 0, capBytes: 0, truncated: false } }) },
)

const notes = computed(() => data.value?.notes ?? [])
const injection = computed<Injection>(() => data.value?.injection ?? { bytes: 0, capBytes: 0, truncated: false })

const kb = (bytes: number) => (bytes / 1024).toFixed(1)
const capKb = computed(() => kb(injection.value.capBytes || 4000))
const footprint = computed(() => `≈${kb(injection.value.bytes)} of ${capKb.value} KB`)

// The body cache can be long; the editor is the place to read it in full. Here
// we show enough to recognize the note.
const bodyPreview = (body: string) => {
  const trimmed = (body || '').trim()
  return trimmed.length > 320 ? trimmed.slice(0, 320).trimEnd() + '…' : trimmed
}

const edit = (note: TelosNote) => navigateTo(`/notes?path=${encodeURIComponent(note.path)}`)
const newTelos = () => navigateTo('/notes?new=telos')
</script>

<style scoped>
/* Live indicator for the always-on injection — the DESIGN "Status Glow" vocab,
   reserved for live/connected dots, not decoration. */
.telos-live-dot {
  width: 7px;
  height: 7px;
  border-radius: var(--radius-full);
  background-color: var(--color-status-active);
  box-shadow: 0 0 8px color-mix(in srgb, var(--color-status-active) 70%, transparent);
  flex-shrink: 0;
  animation: telos-pulse 2.4s ease-in-out infinite;
}

@keyframes telos-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.45; }
}

@media (prefers-reduced-motion: reduce) {
  .telos-live-dot { animation: none; }
}
</style>
