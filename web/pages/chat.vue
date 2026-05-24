<template>
  <div>
    <ScopeSelector />
    <div class="mt-4 bg-[#1e293b] rounded-lg border border-[#334155] flex flex-col h-[calc(100vh-12rem)]">
      <ChatMessageList
        :messages="messages"
        :error="error"
        :is-streaming="isStreaming"
        class="flex-1"
      />
      <PermissionBanner
        v-if="pendingPermission"
        :permission="pendingPermission"
        @approve="onApprove"
        @deny="onDeny"
        @deny-feedback="onDenyFeedback"
      />
      <ChatInput :disabled="isStreaming" @send="onSend" />
    </div>
  </div>
</template>

<script setup lang="ts">
const {
  messages,
  pendingPermission,
  error,
  isStreaming,
  sendMessage,
  resolvePermission,
  loadSession,
} = useChatSession();

const { selectedNodeId } = useGraphNodes();

onMounted(() => {
  loadSession();
});

async function onSend(content: string) {
  await sendMessage(content, selectedNodeId.value || undefined);
}

async function onApprove() {
  const p = pendingPermission.value;
  if (p) await resolvePermission(p.tool_call_id, 'approve');
}

async function onDeny() {
  const p = pendingPermission.value;
  if (p) await resolvePermission(p.tool_call_id, 'deny');
}

async function onDenyFeedback() {
  const p = pendingPermission.value;
  if (p)
    await resolvePermission(
      p.tool_call_id,
      'deny_with_feedback',
      'User indicated this is not relevant.'
    );
}
</script>
