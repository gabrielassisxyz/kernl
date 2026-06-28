<template>
  <UiModal :open="!!bead" size="xl" @close="$emit('close')">
    <template #header>
      <div v-if="bead" class="flex items-start justify-between gap-section">
        <div class="min-w-0">
          <div class="flex items-center gap-tight font-mono-data text-mono-data mb-tight">
            <span class="w-1.5 h-1.5 rounded-full shrink-0" :class="[dotClass, isRunning ? 'animate-pulse' : '']"></span>
            <span class="text-text-muted">{{ bead.id }}</span>
            <span class="text-text-dim">·</span>
            <span class="text-text-muted">{{ prettyState(bead.state) }}</span>
            <span
              v-if="bead.requiresHumanAction"
              class="ml-tight px-tight font-mono-data text-mono-data text-status-gate bg-status-gate/15 border border-status-gate/40"
            >GATE</span>
          </div>
          <h2 class="font-headline text-headline text-text-primary truncate">{{ bead.title }}</h2>
        </div>
        <UiIconButton icon="close" label="Close bead detail" @click="$emit('close')" />
      </div>
    </template>

    <div v-if="bead" class="-mx-section -my-section grid h-[64vh] min-h-0 grid-cols-1 divide-y divide-border-hairline lg:grid-cols-2 lg:divide-x lg:divide-y-0">
      <div class="overflow-y-auto px-section py-component flex flex-col gap-section">
        <section v-if="bead.description">
          <h3 class="font-label-caps text-label-caps text-text-muted mb-base">Description</h3>
          <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ bead.description }}</p>
        </section>
        <section v-if="bead.acceptance">
          <h3 class="font-label-caps text-label-caps text-text-muted mb-base">Acceptance</h3>
          <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ bead.acceptance }}</p>
        </section>
        <section v-if="bead.notes">
          <h3 class="font-label-caps text-label-caps text-text-muted mb-base">Notes</h3>
          <p class="font-body text-body text-text-muted whitespace-pre-wrap">{{ bead.notes }}</p>
        </section>
        <p
          v-if="!bead.description && !bead.acceptance && !bead.notes"
          class="font-body text-body text-text-muted"
        >No detail recorded for this bead.</p>
      </div>

      <div class="flex flex-col min-h-[200px] lg:min-h-0 bg-surface-container-low/40">
        <header class="flex items-center justify-between px-section py-base border-b border-border-hairline shrink-0">
          <h3 class="font-label-caps text-label-caps text-text-muted">Agent Log</h3>
        </header>
        <div class="flex-1 min-h-0 px-section">
          <AgentLogPane :epic-id="epicId" :bead-id="bead.id" />
        </div>
      </div>
    </div>

    <template #footer>
      <div v-if="bead" class="flex flex-col gap-base">
        <div class="flex items-center justify-between gap-section">
          <div class="flex items-center gap-base">
            <template v-if="bead.requiresHumanAction">
              <UiButton variant="accent" :disabled="busy" @click="gate('approve')">Approve</UiButton>
              <UiButton variant="danger" :disabled="busy" @click="gate('reject')">Reject</UiButton>
            </template>
            <span v-else class="font-mono-data text-mono-data text-text-faint">no gate pending</span>
          </div>

          <div class="flex items-center gap-base">
            <UiButton variant="ghost" size="xs" :disabled="busy" @click="act('rollback')">rollback</UiButton>
            <UiButton variant="ghost" size="xs" :disabled="busy" @click="act('refine-scope')">refine-scope</UiButton>
            <UiButton variant="danger" size="xs" :disabled="busy" @click="act('mark-terminal')">mark-terminal</UiButton>
          </div>
        </div>
        <p v-if="actionMsg" class="font-mono-data text-mono-data" :class="actionErr ? 'text-status-failed-text' : 'text-status-passed'">{{ actionMsg }}</p>
      </div>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { Bead } from '~/composables/useBeads'
import { statusTone, statusDotClass, prettyState } from '~/utils/workflow'
import AgentLogPane from '~/components/orchestrator/AgentLogPane.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiIconButton from '~/components/ui/UiIconButton.vue'
import UiModal from '~/components/ui/UiModal.vue'

const props = defineProps<{
  bead: Bead | null
  epicId: string
}>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'mutated'): void }>()

const tone = computed(() => (props.bead ? statusTone(props.bead) : 'neutral'))
const dotClass = computed(() => statusDotClass(tone.value))
const isRunning = computed(() => tone.value === 'running')

const busy = ref(false)
const actionMsg = ref('')
const actionErr = ref(false)

function flash(msg: string, err = false) {
  actionMsg.value = msg
  actionErr.value = err
}

watch(() => props.bead?.id, () => { actionMsg.value = ''; actionErr.value = false })

// Gate approve/reject: resolve the matching approval.
async function gate(action: 'approve' | 'reject') {
  if (!props.bead) return
  if (action === 'reject' && !confirm('Reject this gate?')) return
  busy.value = true
  try {
    const res = await fetch('/api/approvals')
    if (!res.ok) throw new Error(`approvals ${res.status}`)
    const approvals: Array<{ id: string; beadId?: string }> = await res.json()
    const match = approvals.find((a) => a.beadId === props.bead!.id)
    if (!match) { flash('No pending approval found for this bead.', true); return }
    const ar = await fetch(`/api/approvals/${encodeURIComponent(match.id)}/actions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action }),
    })
    if (!ar.ok) throw new Error(`action ${ar.status}`)
    flash(action === 'approve' ? 'Approved.' : 'Rejected.')
    emit('mutated')
  } catch (e) {
    flash(e instanceof Error ? e.message : String(e), true)
  } finally {
    busy.value = false
  }
}

// Bead lifecycle actions.
async function act(kind: 'rollback' | 'mark-terminal' | 'refine-scope') {
  if (!props.bead) return
  if (!confirm(`${kind} ${props.bead.id}?`)) return
  busy.value = true
  try {
    const res = await fetch(`/api/beads/${encodeURIComponent(props.bead.id)}/${kind}`, { method: 'POST' })
    if (!res.ok) throw new Error(`${kind} ${res.status}`)
    flash(`${kind} sent.`)
    emit('mutated')
  } catch (e) {
    flash(e instanceof Error ? e.message : String(e), true)
  } finally {
    busy.value = false
  }
}

</script>
