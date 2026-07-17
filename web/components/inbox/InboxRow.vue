<template>
  <div
    class="rounded-lg border bg-surface transition-colors"
    :class="[
      selected ? 'border-primary/40 ring-1 ring-primary/30' : 'border-border-hairline',
      expanded ? 'border-border-default bg-surface-hover' : 'hover:border-border-default hover:bg-surface-hover',
      isCursor && !expanded ? 'bg-surface-hover' : '',
    ]"
  >
    <!-- The whole head is the disclosure: clicking the capture opens it. -->
    <div
      class="group flex flex-col gap-tight p-component cursor-pointer"
      role="button"
      tabindex="0"
      :aria-expanded="expanded"
      @click="$emit('toggle')"
      @keydown.enter.prevent="$emit('toggle')"
      @keydown.space.prevent="$emit('toggle')"
    >
      <!-- provenance + row actions -->
      <div class="flex items-center gap-base">
        <button
          class="shrink-0 w-4 h-4 rounded-sm border flex items-center justify-center transition-all cursor-pointer"
          :class="[
            selected ? 'bg-primary border-primary opacity-100' : 'border-border-default opacity-0 group-hover:opacity-100 focus-visible:opacity-100',
            isCursor ? 'opacity-100' : '',
          ]"
          :aria-label="selected ? 'Deselect capture' : 'Select capture'"
          @click.stop="$emit('toggleSelect')"
        >
          <span v-if="selected" class="material-symbols-outlined !text-mono-data text-on-primary">check</span>
        </button>

        <span class="font-mono-data text-mono-data tracking-widest px-tight text-text-faint border border-border-hairline">{{ item.type || 'ITEM' }}</span>
        <span v-if="item.batchId" class="font-mono-data text-mono-data text-text-faint">#{{ batchNumber }}</span>
        <span v-if="batchTime" class="font-mono-data text-mono-data text-text-faint">{{ batchTime }}</span>

        <div class="flex-1"></div>

        <div class="flex items-center gap-base font-mono-data text-mono-data">
          <!-- DA briefing: peek when present, generate on demand otherwise -->
          <button v-if="item.hasPrep" class="px-base py-0.5 rounded border border-da-accent/40 text-da-accent-text hover:bg-da-accent/10 transition-colors cursor-pointer" @click.stop="$emit('peek')">◆ Brief</button>
          <button v-else class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-da-accent-text hover:border-da-accent/40 transition-colors disabled:opacity-50 cursor-pointer" :disabled="prepping" @click.stop="$emit('prep')">{{ prepping ? '…' : 'Prep' }}</button>
          <div class="w-[1px] h-3 bg-border-hairline"></div>
          <button
            v-if="proposals && proposals.length > 0"
            class="px-base py-0.5 rounded border border-status-passed/40 text-status-passed hover:bg-status-passed/10 transition-colors cursor-pointer"
            @click.stop="$emit('accept')"
          >Accept</button>
          <button class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-text-primary transition-colors cursor-pointer" @click.stop="$emit('edit')">Edit</button>
          <button class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-status-failed-text hover:border-status-failed/40 transition-colors cursor-pointer" @click.stop="$emit('discard')">Discard</button>
          <span
            class="material-symbols-outlined !text-body text-text-faint transition-transform duration-150 ease-out motion-reduce:transition-none"
            :class="expanded ? 'rotate-180' : ''"
            aria-hidden="true"
          >expand_more</span>
        </div>
      </div>

      <!-- What the capture becomes: the headline of the row. A capture is
           routinely several nodes, so each one gets its own legible line. -->
      <div v-if="!proposals" class="flex items-center gap-base py-tight">
        <!-- The spinner is a promise that a proposal is coming. Only show it when
             one actually is — background classification runs only with the switch
             on and an LLM configured. Otherwise the capture just rests unclassified,
             and a forever-spinner would be a lie. -->
        <template v-if="classifying">
          <span class="material-symbols-outlined !text-body text-text-faint animate-spin motion-reduce:animate-none">progress_activity</span>
          <span class="font-body text-body text-text-muted">Reading the capture — the DA's proposal will appear here.</span>
        </template>
        <span v-else class="font-body text-body text-text-faint">Not classified — edit it, or use “Classify now”.</span>
      </div>

      <ul v-else class="flex flex-col">
        <li
          v-for="(p, i) in proposals"
          :key="i"
          class="flex items-baseline gap-base py-0.5 min-w-0"
        >
          <span
            class="shrink-0 w-[74px] flex items-center gap-tight font-mono-data text-mono-data"
            :class="TARGET_META[p.target].text"
          >
            <span class="material-symbols-outlined !text-body leading-none" aria-hidden="true">{{ TARGET_META[p.target].icon }}</span>
            {{ TARGET_META[p.target].label }}
          </span>
          <span class="font-body text-body text-text-primary truncate">{{ p.title }}</span>
          <span v-if="p.projectLabel" class="shrink-0 font-mono-data text-mono-data text-text-muted truncate">{{ p.projectLabel }}</span>
          <span
            v-for="tag in p.tags"
            :key="tag"
            class="shrink-0 font-mono-data text-mono-data text-text-faint"
          >#{{ tag }}</span>
          <span v-if="p.dueLabel" class="shrink-0 ml-auto flex items-center gap-tight font-mono-data text-mono-data text-tertiary">
            <span class="material-symbols-outlined !text-body leading-none" aria-hidden="true">event</span>
            {{ p.dueLabel }}
          </span>
        </li>
      </ul>

      <!-- The capture itself, demoted to its source line. It is the truth, but
           it is not the answer to "what is this and where is it going". -->
      <p class="font-mono-data text-mono-data text-text-faint truncate">
        <span class="text-text-dim">›</span> {{ sourceLine }}
      </p>
    </div>

    <slot name="drawer" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { TARGET_META, type RawCaptureAction, type Target } from '~/utils/inboxTargets'

export interface InboxItemData {
  id: string
  type: string
  title: string
  subtitle: string
  /** the nodes the DA proposes this capture becomes; empty while unclassified */
  suggestedActions?: RawCaptureAction[]
  batchId?: string
  batchSource?: string
  batchSequence?: number
  batchTimestamp?: string
  batchContextTitle?: string
  hasPrep?: boolean
  flagged?: boolean
}

/** one node the capture becomes, as the row renders it */
export interface Proposal {
  target: Target
  title: string
  projectLabel: string
  dueLabel: string
  tags: string[]
}

const props = withDefaults(defineProps<{
  item: InboxItemData
  /** the DA's proposal, one line per node — null while still classifying */
  proposals: Proposal[] | null
  /** whether background classification is actually going to run: gates the
   *  "reading the capture" spinner so it never spins on captures nothing will
   *  touch (switch off or no LLM). */
  classifying?: boolean
  selected?: boolean
  isCursor?: boolean
  expanded?: boolean
  prepping?: boolean
}>(), {
  classifying: true,
})

defineEmits<{
  (e: 'toggle'): void
  (e: 'toggleSelect'): void
  (e: 'accept'): void
  (e: 'edit'): void
  (e: 'discard'): void
  (e: 'prep'): void
  (e: 'peek'): void
}>()

const batchNumber = computed(() => String((props.item.batchSequence ?? 0) + 1).padStart(2, '0'))

const batchTime = computed(() => {
  const match = (props.item.batchTimestamp || '').match(/\b\d{1,2}:\d{2}(?::\d{2})?\b/)
  return match ? match[0].slice(0, 5) : ''
})

// The source line is the capture flattened to one line: a multi-line "amanhã:"
// list reads as its first items, not as a lone word with everything hidden. The
// list markers go — they are structure the single line no longer has.
const sourceLine = computed(() => {
  const body = (props.item.subtitle || props.item.title || '').trim()
  return body
    .split('\n')
    .map(line => line.trim().replace(/^[-*•]\s*/, ''))
    .filter(Boolean)
    .join(' · ')
})
</script>
