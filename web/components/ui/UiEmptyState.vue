<template>
  <div :class="classes">
    <div v-if="icon" class="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-surface-container-high border border-border-hairline">
      <span class="material-symbols-outlined !text-display text-text-primary" aria-hidden="true">{{ icon }}</span>
    </div>
    <div class="flex max-w-[48ch] flex-col items-center gap-tight text-center">
      <p class="font-body text-body text-text-primary font-medium">{{ title }}</p>
      <p v-if="body" class="font-body text-body text-text-muted mt-1">{{ body }}</p>
    </div>
    <div class="mt-base flex flex-col items-center gap-base">
      <slot name="action">
        <UiButton v-if="actionLabel" variant="primary" :icon="actionIcon" @click="$emit('action')">
          {{ actionLabel }}
        </UiButton>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import UiButton from './UiButton.vue'

const props = withDefaults(defineProps<{
  icon?: string
  title: string
  body?: string
  bordered?: boolean
  fill?: boolean
  actionLabel?: string
  actionIcon?: string
}>(), {
  icon: '',
  body: '',
  bordered: false,
  fill: false,
  actionLabel: '',
  actionIcon: 'add',
})

defineEmits<{ (e: 'action'): void }>()

const classes = computed(() => [
  'flex flex-col items-center justify-center gap-base text-text-muted',
  props.fill ? 'flex-1 min-h-0' : 'py-[64px]',
  props.bordered ? 'rounded border border-border-default bg-surface' : '',
])
</script>
