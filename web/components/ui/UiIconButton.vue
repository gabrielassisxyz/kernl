<template>
  <button
    :type="type"
    :disabled="disabled"
    :title="label"
    :aria-label="label"
    :class="classes"
  >
    <span class="material-symbols-outlined" :class="iconSizeClass" aria-hidden="true">{{ icon }}</span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'

type IconButtonVariant = 'ghost' | 'secondary' | 'primary' | 'danger'
type IconButtonSize = 'sm' | 'md'

const props = withDefaults(defineProps<{
  icon: string
  label: string
  variant?: IconButtonVariant
  size?: IconButtonSize
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
}>(), {
  variant: 'ghost',
  size: 'md',
  type: 'button',
  disabled: false,
})

const base = 'inline-flex shrink-0 items-center justify-center rounded border transition-colors duration-150 cursor-pointer outline-none focus-visible:border-primary/70 focus-visible:ring-1 focus-visible:ring-primary/30 disabled:cursor-not-allowed disabled:opacity-45'

const sizes: Record<IconButtonSize, string> = {
  sm: 'h-7 w-7',
  md: 'h-8 w-8',
}

const iconSizes: Record<IconButtonSize, string> = {
  sm: '!text-[16px]',
  md: '!text-[18px]',
}

const variants: Record<IconButtonVariant, string> = {
  ghost: 'border-transparent bg-transparent text-text-muted hover:bg-surface-hover hover:text-text-primary',
  secondary: 'border-border-hairline bg-surface-container-low text-text-muted hover:border-border-default hover:bg-surface-hover hover:text-text-primary',
  primary: 'border-primary/40 bg-primary text-on-primary hover:bg-primary-container',
  danger: 'border-status-failed/40 bg-status-failed/10 text-status-failed-text hover:bg-status-failed/15',
}

const classes = computed(() => [base, sizes[props.size], variants[props.variant]])
const iconSizeClass = computed(() => iconSizes[props.size])
</script>

<style scoped>
@media (pointer: coarse) {
  button {
    min-height: 44px;
    min-width: 44px;
  }
}
</style>
