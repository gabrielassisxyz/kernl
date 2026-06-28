<template>
  <div class="border-t border-border-hairline bg-status-failed/10 p-component flex items-center justify-between gap-base flex-wrap">
    <span class="font-body text-body text-status-failed-text">
      Agent wants to read: {{ permission.node_path || permission.node_id }}
    </span>
    <div class="flex gap-base">
      <UiButton variant="success" size="sm" @click="$emit('approve')">Allow</UiButton>
      <UiButton variant="danger" size="sm" @click="$emit('deny')">Deny</UiButton>
      <UiButton variant="secondary" size="sm" @click="$emit('deny-feedback')">Not relevant</UiButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { PermissionEvent } from '~/composables/useChatSession';
import UiButton from '~/components/ui/UiButton.vue'

defineProps<{
  permission: PermissionEvent;
}>();

defineEmits<{
  approve: [];
  deny: [];
  'deny-feedback': [];
}>();
</script>
