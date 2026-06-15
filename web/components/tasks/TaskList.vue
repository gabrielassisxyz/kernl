<template>
  <div class="flex-1 overflow-auto px-section py-base">
    <table class="w-full border-collapse">
      <thead>
        <tr class="border-b border-border-hairline text-left">
          <th class="w-6 py-base pr-base"></th>
          <th class="py-base pr-section font-label-caps text-label-caps text-text-muted uppercase">Title</th>
          <th class="py-base pr-section font-label-caps text-label-caps text-text-muted uppercase">ID</th>
          <th class="py-base pr-section font-label-caps text-label-caps text-text-muted uppercase">State</th>
          <th
            class="py-base pr-section font-label-caps text-label-caps uppercase cursor-pointer select-none whitespace-nowrap"
            :class="sortKey === 'priority' ? 'text-text-primary' : 'text-text-muted hover:text-text-primary'"
            @click="toggleSort('priority')"
          >
            Priority<span class="font-mono-data ml-tight">{{ sortIndicator('priority') }}</span>
          </th>
          <th
            class="py-base font-label-caps text-label-caps uppercase cursor-pointer select-none whitespace-nowrap"
            :class="sortKey === 'updatedAt' ? 'text-text-primary' : 'text-text-muted hover:text-text-primary'"
            @click="toggleSort('updatedAt')"
          >
            Updated<span class="font-mono-data ml-tight">{{ sortIndicator('updatedAt') }}</span>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="bead in sorted"
          :key="bead.id"
          class="group border-b border-border-hairline hover:bg-surface-hover cursor-pointer transition-colors duration-150"
          @click="$emit('open', bead)"
        >
          <td class="py-base pr-base align-middle">
            <span
              class="block w-1.5 h-1.5 rounded-full"
              :class="[dotClass(bead), tone(bead) === 'running' ? 'animate-pulse' : '']"
            ></span>
          </td>
          <td class="py-base pr-section font-body text-body text-text-primary max-w-0 truncate">{{ bead.title }}</td>
          <td class="py-base pr-section font-mono-data text-mono-data text-text-faint whitespace-nowrap">{{ bead.id }}</td>
          <td class="py-base pr-section font-mono-data text-mono-data text-text-dim whitespace-nowrap">{{ prettyState(bead.state) }}</td>
          <td class="py-base pr-section font-mono-data text-mono-data text-text-muted whitespace-nowrap">P{{ bead.priority }}</td>
          <td class="py-base font-mono-data text-mono-data text-text-faint whitespace-nowrap">{{ formatTimestamp(bead.updatedAt) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Bead } from '~/composables/useBeads'
import { statusTone, statusDotClass, prettyState } from '~/utils/workflow'
import { formatTimestamp } from '~/utils/time'

const props = defineProps<{ tasks: Bead[] }>()

defineEmits<{ (e: 'open', bead: Bead): void }>()

type SortKey = 'priority' | 'updatedAt'
const sortKey = ref<SortKey>('priority')
const sortAsc = ref(true)

function toggleSort(key: SortKey) {
  if (sortKey.value === key) {
    sortAsc.value = !sortAsc.value
  } else {
    sortKey.value = key
    // Priority ascends (P0 first); recency descends (newest first) by default.
    sortAsc.value = key === 'priority'
  }
}

function sortIndicator(key: SortKey): string {
  if (sortKey.value !== key) return ''
  return sortAsc.value ? '↑' : '↓'
}

const sorted = computed(() => {
  const rows = [...props.tasks]
  rows.sort((a, b) => {
    let cmp: number
    if (sortKey.value === 'priority') {
      cmp = a.priority - b.priority
    } else {
      cmp = new Date(a.updatedAt).getTime() - new Date(b.updatedAt).getTime()
    }
    return sortAsc.value ? cmp : -cmp
  })
  return rows
})

const tone = (b: Bead) => statusTone(b)
const dotClass = (b: Bead) => statusDotClass(statusTone(b))
</script>
