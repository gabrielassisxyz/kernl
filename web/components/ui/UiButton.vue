<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :class="classes"
  >
    <span
      v-if="loading"
      class="material-symbols-outlined animate-spin"
      :style="iconStyle"
      aria-hidden="true"
    >
      progress_activity
    </span>
    <span v-if="icon && !loading" class="material-symbols-outlined" :style="iconStyle" aria-hidden="true">{{ icon }}</span>
    <slot />
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'success' | 'accent'
type ButtonSize = 'xs' | 'sm' | 'md'

const props = withDefaults(defineProps<{
  variant?: ButtonVariant
  size?: ButtonSize
  type?: 'button' | 'submit' | 'reset'
  icon?: string
  iconSize?: number
  loading?: boolean
  disabled?: boolean
  block?: boolean
}>(), {
  variant: 'secondary',
  size: 'md',
  type: 'button',
  icon: '',
  iconSize: 16,
  loading: false,
  disabled: false,
  block: false,
})

// Size the glyph via inline style, not a `!text-[Npx]` utility. The base
// `.material-symbols-outlined` sets font-size:24px, and Tailwind's important
// utilities live in a cascade layer where `!important` beats any unlayered
// `!important` override — so an external consumer can't shrink the icon by
// fighting the class. An inline font-size wins outright without `!important`.
const iconStyle = computed(() => ({ fontSize: `${props.iconSize}px` }))

const base = 'inline-flex items-center justify-center gap-tight rounded border font-body transition-colors duration-150 cursor-pointer outline-none focus-visible:border-primary/70 focus-visible:ring-1 focus-visible:ring-primary/30 disabled:cursor-not-allowed disabled:opacity-45'

const sizes: Record<ButtonSize, string> = {
  xs: 'h-6 px-base text-mono-data',
  sm: 'h-7 px-component text-body',
  md: 'h-9 px-component text-body',
}

const variants: Record<ButtonVariant, string> = {
  primary: 'border-primary/40 bg-primary text-on-primary hover:bg-primary-container',
  secondary: 'border-border-hairline bg-surface-container-low text-text-muted hover:border-border-default hover:bg-surface-hover hover:text-text-primary',
  ghost: 'border-transparent bg-transparent text-text-muted hover:bg-surface-hover hover:text-text-primary',
  danger: 'border-status-failed/40 bg-status-failed/10 text-status-failed-text hover:bg-status-failed/15',
  success: 'border-status-passed/40 bg-status-passed/10 text-status-passed hover:bg-status-passed/15',
  accent: 'border-da-accent/40 bg-da-accent/10 text-da-accent-text hover:bg-da-accent/15',
}

const classes = computed(() => [
  base,
  sizes[props.size],
  variants[props.variant],
  props.block ? 'w-full' : '',
])
</script>

<style scoped>
@media (pointer: coarse) {
  button {
    min-height: 44px;
    min-width: 44px;
  }
}
</style>
