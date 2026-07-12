<template>
  <div
    class="group flex items-center gap-component p-component rounded-lg border bg-surface hover:bg-surface-hover transition-colors cursor-pointer relative"
    :class="[
      selected ? 'border-primary/40 ring-1 ring-primary/30' : 'border-border-hairline hover:border-border-default',
      isCursor ? 'bg-surface-hover' : ''
    ]"
    @click="$emit('select')"
  >
    <!-- select checkbox -->
    <button
      class="shrink-0 w-4 h-4 rounded-sm border flex items-center justify-center transition-all"
      :class="[
        selected ? 'bg-primary border-primary opacity-100' : 'border-border-default opacity-0 group-hover:opacity-100',
        isCursor ? 'opacity-100' : ''
      ]"
      @click.stop="$emit('toggleSelect')"
    >
      <span v-if="selected" class="material-symbols-outlined !text-mono-data text-on-primary">check</span>
    </button>

    <!-- body -->
    <div class="flex flex-col flex-1 min-w-0">
      <div class="flex items-center gap-base mb-tight">
        <span class="font-mono-data text-mono-data tracking-widest px-tight text-text-faint border border-border-hairline">{{ item.type || 'ITEM' }}</span>
        <span v-if="item.batchId" class="font-mono-data text-mono-data px-tight border border-border-hairline text-text-faint bg-surface-container-low">
          #{{ batchNumber }}
        </span>
        <span v-if="batchTime" class="font-mono-data text-mono-data text-text-faint">{{ batchTime }}</span>
        <h3 class="font-headline text-text-primary truncate">{{ item.title }}</h3>
      </div>
      <p v-if="showSubtitle" class="font-body text-text-muted truncate text-mono-data">{{ item.subtitle }}</p>
    </div>

    <!-- DA suggestion: one chip per node the capture becomes -->
    <div class="shrink-0 flex items-center gap-tight">
      <span v-if="!chips" class="flex items-center gap-tight font-mono-data text-mono-data text-text-faint">
        <span class="material-symbols-outlined !text-body animate-spin">progress_activity</span>
        DA classifying…
      </span>
      <template v-else>
        <span
          v-for="(chip, i) in visibleChips"
          :key="i"
          class="flex items-center gap-tight px-base py-0.5 rounded border font-mono-data text-mono-data"
          :class="TARGET_META[chip.target].chip"
        >
          <span class="material-symbols-outlined !text-body">{{ TARGET_META[chip.target].icon }}</span>
          {{ chip.label }}
        </span>
        <span v-if="overflowCount > 0" class="font-mono-data text-mono-data text-text-faint">+{{ overflowCount }}</span>
      </template>
    </div>

    <!-- row actions (always visible). Manual processing never depends on a DA suggestion. -->
    <div class="shrink-0 flex items-center gap-base font-mono-data text-mono-data pl-base">
      <!-- DA briefing: peek when present, generate on demand otherwise -->
      <button v-if="item.hasPrep" class="px-base py-0.5 rounded border border-da-accent/40 text-da-accent-text hover:bg-da-accent/10 transition-colors" @click.stop="$emit('peek')">◆ Brief</button>
      <button v-else class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-da-accent-text hover:border-da-accent/40 transition-colors disabled:opacity-50" :disabled="prepping" @click.stop="$emit('prep')">{{ prepping ? '…' : 'Prep' }}</button>
      <div class="w-[1px] h-3 bg-border-hairline"></div>
      <template v-if="chips && chips.length > 0">
        <button class="px-base py-0.5 rounded border border-status-passed/40 text-status-passed hover:bg-status-passed/10 transition-colors" @click.stop="$emit('accept')">Accept</button>
        <button class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-text-primary transition-colors" @click.stop="$emit('edit')">Edit</button>
      </template>
      <button v-else class="px-base py-0.5 rounded border border-primary/40 text-primary hover:bg-primary/10 transition-colors" @click.stop="$emit('edit')">Process…</button>
      <button class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-status-failed-text hover:border-status-failed/40 transition-colors" @click.stop="$emit('discard')">Discard</button>
    </div>
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

/** one rendered suggestion chip: what the capture becomes, in one glance */
export interface SuggestionChip {
  target: Target
  label: string
}

const MAX_VISIBLE_CHIPS = 3

const props = defineProps<{
  item: InboxItemData
  /** the DA's proposal, one chip per node — null while still classifying */
  chips: SuggestionChip[] | null
  selected?: boolean
  isCursor?: boolean
  prepping?: boolean
}>()

defineEmits<{
  (e: 'select'): void
  (e: 'toggleSelect'): void
  (e: 'accept'): void
  (e: 'edit'): void
  (e: 'discard'): void
  (e: 'prep'): void
  (e: 'peek'): void
}>()

// A capture can fan out into more chips than the row can hold; show the first
// few and count the rest, so the row stays one line.
const visibleChips = computed(() => (props.chips || []).slice(0, MAX_VISIBLE_CHIPS))
const overflowCount = computed(() => Math.max(0, (props.chips?.length || 0) - MAX_VISIBLE_CHIPS))

const batchNumber = computed(() => String((props.item.batchSequence ?? 0) + 1).padStart(2, '0'))
const batchTime = computed(() => {
  const ts = props.item.batchTimestamp || ''
  const match = ts.match(/\b\d{1,2}:\d{2}(?::\d{2})?\b/)
  return match ? match[0].slice(0, 5) : ''
})

// Captures often carry subtitle === title (body mirrors the one-liner); skip the dupe.
const showSubtitle = computed(() => {
  const sub = (props.item.subtitle || '').trim()
  return sub.length > 0 && sub !== (props.item.title || '').trim()
})
</script>
