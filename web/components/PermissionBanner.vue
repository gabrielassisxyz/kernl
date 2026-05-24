<template>
  <div class="border-t border-[#334155] bg-[#450a0a]/50 p-3 flex items-center justify-between gap-2 flex-wrap">
    <span class="text-sm text-[#fca5a5]">
      Agent wants to read: {{ permission.node_path || permission.node_id }}
    </span>
    <div class="flex gap-2">
      <button
        class="bg-[#16a34a] text-white px-3 py-1 rounded font-mono text-xs hover:bg-[#22c55e] transition-colors"
        @click="$emit('approve')"
      >
        Permitir
      </button>
      <button
        class="bg-[#dc2626] text-white px-3 py-1 rounded font-mono text-xs hover:bg-[#ef4444] transition-colors"
        @click="$emit('deny')"
      >
        Negar
      </button>
      <button
        class="bg-[#ca8a04] text-white px-3 py-1 rounded font-mono text-xs hover:bg-[#eab308] transition-colors"
        @click="$emit('deny-feedback')"
      >
        Nao e relevante
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { PermissionEvent } from '~/composables/useChatSession';

defineProps<{
  permission: PermissionEvent;
}>();

defineEmits<{
  approve: [];
  deny: [];
  'deny-feedback': [];
}>();
</script>
