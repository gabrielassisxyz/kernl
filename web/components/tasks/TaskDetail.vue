<template>
  <Transition
    enter-active-class="transition-opacity duration-150"
    enter-from-class="opacity-0"
    leave-active-class="transition-opacity duration-150"
    leave-to-class="opacity-0"
  >
    <div
      v-if="bead"
      class="absolute inset-0 z-40 bg-black/40"
      @click="$emit('close')"
    >
      <aside
        class="absolute top-0 right-0 h-full w-full max-w-[440px] bg-surface border-l border-border-hairline flex flex-col"
        @click.stop
      >
        <!-- Header -->
        <header class="px-section py-component border-b border-border-hairline flex items-start gap-component">
          <span
            class="mt-[7px] w-1.5 h-1.5 rounded-full shrink-0"
            :class="[dotClass, tone === 'running' ? 'animate-pulse' : '']"
          ></span>
          <div class="flex-1 min-w-0">
            <h2 class="font-headline text-text-primary leading-snug">{{ bead.title }}</h2>
            <div class="flex items-center gap-section mt-tight">
              <span class="font-mono-data text-mono-data text-text-faint">{{ bead.id }}</span>
              <span class="font-mono-data text-mono-data text-text-dim">{{ prettyState(bead.state) }}</span>
              <span class="font-mono-data text-mono-data text-text-muted">P{{ bead.priority }}</span>
            </div>
          </div>
          <button
            class="text-text-muted hover:text-text-primary transition-colors duration-150 shrink-0"
            title="Close"
            @click="$emit('close')"
          >
            <span class="material-symbols-outlined !text-[20px]">close</span>
          </button>
        </header>

        <!-- Body -->
        <div class="flex-1 overflow-y-auto px-section py-component flex flex-col gap-section">
          <section v-if="bead.description">
            <h3 class="font-label-caps text-label-caps text-text-muted uppercase mb-base">Description</h3>
            <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ bead.description }}</p>
          </section>

          <section v-if="bead.acceptance">
            <h3 class="font-label-caps text-label-caps text-text-muted uppercase mb-base">Acceptance</h3>
            <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ bead.acceptance }}</p>
          </section>

          <section v-if="bead.notes">
            <h3 class="font-label-caps text-label-caps text-text-muted uppercase mb-base">Notes</h3>
            <p class="font-body text-body text-text-muted whitespace-pre-wrap">{{ bead.notes }}</p>
          </section>

          <p
            v-if="!bead.description && !bead.acceptance && !bead.notes"
            class="font-body text-body text-text-dim"
          >
            No description, acceptance, or notes.
          </p>
        </div>

        <!-- Footer meta -->
        <footer class="px-section py-base border-t border-border-hairline flex items-center justify-between">
          <span class="font-mono-data text-mono-data text-text-faint">{{ prettyState(bead.type) }}</span>
          <span class="font-mono-data text-mono-data text-text-faint">updated {{ formatTimestamp(bead.updatedAt) }}</span>
        </footer>
      </aside>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Bead } from '~/composables/useBeads'
import { statusTone, statusDotClass, prettyState } from '~/utils/workflow'
import { formatTimestamp } from '~/utils/time'

const props = defineProps<{ bead: Bead | null }>()

defineEmits<{ (e: 'close'): void }>()

const tone = computed(() => (props.bead ? statusTone(props.bead) : 'neutral'))
const dotClass = computed(() => statusDotClass(tone.value))
</script>
