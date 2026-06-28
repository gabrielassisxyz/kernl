<template>
  <dialog
    ref="dialogRef"
    class="ui-modal-card flex max-h-[86vh] w-full flex-col overflow-hidden rounded border border-border-default bg-surface-overlay"
    :class="[sizeClass, align === 'top' ? 'mt-[12vh] mb-auto' : 'm-auto']"
    @close="emit('close')"
    @click="onBackdropClick"
    aria-modal="true"
    :aria-labelledby="title ? titleId : undefined"
  >
    <header v-if="title || $slots.header" class="shrink-0 border-b border-border-hairline px-section py-component">
            <slot name="header">
              <p v-if="kicker" class="mb-tight font-mono-data text-mono-data text-text-faint">{{ kicker }}</p>
              <h2 :id="titleId" class="font-headline text-headline text-text-primary">{{ title }}</h2>
            </slot>
          </header>

          <div class="min-h-0 flex-1 overflow-y-auto px-section py-section">
            <slot />
          </div>

          <footer v-if="$slots.footer" class="shrink-0 border-t border-border-hairline bg-surface-container-low px-section py-base">
            <slot name="footer" />
    </footer>
  </dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'

type ModalSize = 'sm' | 'md' | 'lg' | 'xl'
type ModalAlign = 'center' | 'top'

const props = withDefaults(defineProps<{
  open: boolean
  title?: string
  kicker?: string
  size?: ModalSize
  align?: ModalAlign
  closeOnEsc?: boolean
  closeOnBackdrop?: boolean
}>(), {
  title: '',
  kicker: '',
  size: 'md',
  align: 'center',
  closeOnEsc: true,
  closeOnBackdrop: true,
})

const emit = defineEmits<{ (e: 'close'): void }>()
const titleId = `ui-modal-title-${Math.random().toString(36).slice(2)}`

const dialogRef = ref<HTMLDialogElement | null>(null)

const sizes: Record<ModalSize, string> = {
  sm: 'max-w-[360px]',
  md: 'max-w-[480px]',
  lg: 'max-w-lg',
  xl: 'max-w-5xl',
}

const sizeClass = computed(() => sizes[props.size])

watch(() => props.open, (isOpen) => {
  if (!dialogRef.value) return
  if (isOpen) {
    dialogRef.value.showModal()
  } else {
    dialogRef.value.close()
  }
})

onMounted(() => {
  if (props.open && dialogRef.value) {
    dialogRef.value.showModal()
  }
  
  // Remove native escape listener if closeOnEsc is false
  dialogRef.value?.addEventListener('cancel', (e) => {
    if (!props.closeOnEsc) e.preventDefault()
  })
})

function onBackdropClick(event: MouseEvent) {
  if (!props.closeOnBackdrop) return
  if (event.target === dialogRef.value) {
    emit('close')
  }
}
</script>

<style scoped>
.ui-modal-card {
  opacity: 0;
  transform: translateY(6px) scale(0.985);
  transition: opacity 160ms ease, transform 160ms cubic-bezier(0.22, 1, 0.36, 1), display 160ms ease allow-discrete, overlay 160ms ease allow-discrete;
}

.ui-modal-card[open] {
  opacity: 1;
  transform: translateY(0) scale(1);
}

@starting-style {
  .ui-modal-card[open] {
    opacity: 0;
    transform: translateY(6px) scale(0.985);
  }
}

.ui-modal-card::backdrop {
  background-color: rgba(0, 0, 0, 0);
  transition: background-color 160ms ease, display 160ms ease allow-discrete, overlay 160ms ease allow-discrete;
}

.ui-modal-card[open]::backdrop {
  background-color: rgba(0, 0, 0, 0.55);
}

@starting-style {
  .ui-modal-card[open]::backdrop {
    background-color: rgba(0, 0, 0, 0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .ui-modal-card,
  .ui-modal-card::backdrop {
    transition-duration: 1ms;
  }
}
</style>
