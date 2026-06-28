<template>
  <Transition name="ui-toast">
    <div
      v-if="message"
      class="fixed z-toast flex max-w-[min(520px,calc(100vw-32px))] items-center gap-component rounded border border-border-hairline bg-surface-container-high px-component py-base"
      :class="positionClass"
      role="status"
    >
      <span v-if="icon" class="material-symbols-outlined !text-[16px] text-text-muted" aria-hidden="true">{{ icon }}</span>
      <span class="min-w-0 flex-1 truncate font-body text-body text-text-primary">{{ message }}</span>
      <UiButton v-if="actionLabel" variant="ghost" size="xs" @click="$emit('action')">{{ actionLabel }}</UiButton>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import UiButton from './UiButton.vue'

const props = withDefaults(defineProps<{
  message: string
  actionLabel?: string
  icon?: string
  position?: 'bottom-left' | 'bottom-center'
}>(), {
  actionLabel: '',
  icon: '',
  position: 'bottom-left',
})

defineEmits<{ (e: 'action'): void }>()

const positionClass = computed(() =>
  props.position === 'bottom-center'
    ? 'bottom-section left-1/2 -translate-x-1/2'
    : 'bottom-section left-section'
)
</script>

<style scoped>
.ui-toast-enter-active,
.ui-toast-leave-active {
  transition: opacity 160ms ease, transform 160ms cubic-bezier(0.22, 1, 0.36, 1);
}

.ui-toast-enter-from,
.ui-toast-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

@media (prefers-reduced-motion: reduce) {
  .ui-toast-enter-active,
  .ui-toast-leave-active {
    transition-duration: 1ms;
  }
}
</style>
