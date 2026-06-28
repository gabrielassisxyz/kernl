<template>
  <div :class="classes">
    <div class="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-status-failed/10 border border-status-failed/20">
      <span class="material-symbols-outlined !text-display text-status-failed" aria-hidden="true">{{ icon }}</span>
    </div>
    <div class="flex max-w-[56ch] flex-col items-center gap-tight text-center">
      <p class="font-body text-body text-text-primary font-medium">{{ title }}</p>
      <p v-if="message" class="font-body text-body text-text-muted mt-1">{{ message }}</p>
      <p v-if="detail" class="mt-tight p-tight bg-surface-container-high rounded border border-border-hairline font-mono-data text-mono-data text-status-failed-text max-w-full overflow-hidden text-ellipsis">{{ detail }}</p>
    </div>
    <div class="mt-base flex items-center gap-component">
      <slot name="actions">
        <UiButton v-if="retryLabel" variant="secondary" icon="refresh" @click="$emit('retry')">
          {{ retryLabel }}
        </UiButton>
        <UiButton v-if="secondaryLabel" variant="ghost" :icon="secondaryIcon" @click="$emit('secondary')">
          {{ secondaryLabel }}
        </UiButton>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import UiButton from './UiButton.vue'

const props = withDefaults(defineProps<{
  title: string
  message?: string
  detail?: string | null
  retryLabel?: string
  secondaryLabel?: string
  secondaryIcon?: string
  icon?: string
  fill?: boolean
  bordered?: boolean
}>(), {
  message: '',
  detail: '',
  retryLabel: 'Retry',
  secondaryLabel: '',
  secondaryIcon: 'settings',
  icon: 'cloud_off',
  fill: false,
  bordered: false,
})

defineEmits<{ 
  (e: 'retry'): void
  (e: 'secondary'): void 
}>()

const classes = computed(() => [
  'flex flex-col items-center justify-center gap-base text-text-muted',
  props.fill ? 'flex-1 min-h-0' : 'py-[64px]',
  props.bordered ? 'rounded border border-border-default bg-surface' : '',
])
</script>
