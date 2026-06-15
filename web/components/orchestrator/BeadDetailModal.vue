<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="bead"
        class="fixed inset-0 z-[60] flex items-center justify-center p-section bg-black/50"
        @click.self="$emit('close')"
        @keydown.esc="$emit('close')"
      >
        <div
          class="w-full max-w-5xl h-[80vh] flex flex-col rounded-lg border border-border-hairline bg-surface overflow-hidden"
        >
          <!-- header -->
          <header class="flex items-start justify-between gap-section px-section py-component border-b border-border-hairline shrink-0">
            <div class="min-w-0">
              <div class="flex items-center gap-tight font-mono-data text-[11px] mb-tight">
                <span class="w-1.5 h-1.5 rounded-full shrink-0" :class="[dotClass, isRunning ? 'animate-pulse' : '']"></span>
                <span class="text-text-faint">{{ bead.id }}</span>
                <span class="text-text-dim">·</span>
                <span class="text-text-faint">{{ prettyState(bead.state) }}</span>
                <span
                  v-if="bead.requiresHumanAction"
                  class="ml-tight px-tight font-mono-data text-[10px] tracking-widest text-status-gate bg-status-gate/15 border border-status-gate/40"
                >GATE</span>
              </div>
              <h2 class="font-headline text-headline text-text-primary truncate">{{ bead.title }}</h2>
            </div>
            <button
              class="material-symbols-outlined text-text-muted hover:text-text-primary transition-colors !text-[20px] shrink-0"
              @click="$emit('close')"
            >close</button>
          </header>

          <!-- body: two panes -->
          <div class="flex-1 grid grid-cols-1 lg:grid-cols-2 min-h-0 divide-y lg:divide-y-0 lg:divide-x divide-border-hairline">
            <!-- left: detail -->
            <div class="overflow-y-auto hide-scrollbar px-section py-component flex flex-col gap-section">
              <section v-if="bead.description">
                <h3 class="font-label-caps text-[10px] tracking-widest text-text-muted uppercase mb-base">Description</h3>
                <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ bead.description }}</p>
              </section>
              <section v-if="bead.acceptance">
                <h3 class="font-label-caps text-[10px] tracking-widest text-text-muted uppercase mb-base">Acceptance</h3>
                <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ bead.acceptance }}</p>
              </section>
              <section v-if="bead.notes">
                <h3 class="font-label-caps text-[10px] tracking-widest text-text-muted uppercase mb-base">Notes</h3>
                <p class="font-body text-body text-text-muted whitespace-pre-wrap">{{ bead.notes }}</p>
              </section>
              <p
                v-if="!bead.description && !bead.acceptance && !bead.notes"
                class="font-body text-body text-text-faint"
              >No detail recorded for this bead.</p>
            </div>

            <!-- right: live agent log -->
            <div class="flex flex-col min-h-[200px] lg:min-h-0 bg-surface-container-low/40">
              <header class="flex items-center justify-between px-section py-base border-b border-border-hairline shrink-0">
                <h3 class="font-label-caps text-[10px] tracking-widest text-text-muted uppercase">Agent Log</h3>
              </header>
              <div class="flex-1 min-h-0 px-section">
                <AgentLogPane :epic-id="epicId" :bead-id="bead.id" />
              </div>
            </div>
          </div>

          <!-- footer: actions -->
          <footer class="shrink-0 border-t border-border-hairline px-section py-base flex items-center justify-between gap-section">
            <!-- gate actions (amber) -->
            <div class="flex items-center gap-base">
              <template v-if="bead.requiresHumanAction">
                <button
                  class="px-component py-1.5 rounded font-body text-body bg-status-gate/15 text-status-gate border border-status-gate/40 hover:bg-status-gate/25 transition-colors disabled:opacity-50"
                  :disabled="busy"
                  @click="gate('approve')"
                >Approve</button>
                <button
                  class="px-component py-1.5 rounded font-body text-body text-text-muted border border-border-hairline hover:text-status-failed hover:border-status-failed/40 transition-colors disabled:opacity-50"
                  :disabled="busy"
                  @click="gate('reject')"
                >Reject</button>
              </template>
              <span v-else class="font-mono-data text-[11px] text-text-dim">no gate pending</span>
            </div>

            <!-- bead actions (ghost) -->
            <div class="flex items-center gap-section font-mono-data text-[11px]">
              <button class="text-text-muted hover:text-text-primary transition-colors disabled:opacity-50" :disabled="busy" @click="act('rollback')">rollback</button>
              <button class="text-text-muted hover:text-text-primary transition-colors disabled:opacity-50" :disabled="busy" @click="act('refine-scope')">refine-scope</button>
              <button class="text-text-muted hover:text-status-failed transition-colors disabled:opacity-50" :disabled="busy" @click="act('mark-terminal')">mark-terminal</button>
            </div>
          </footer>

          <p v-if="actionMsg" class="shrink-0 px-section pb-base font-mono-data text-[11px]" :class="actionErr ? 'text-status-failed' : 'text-status-passed'">{{ actionMsg }}</p>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import type { Bead } from '~/composables/useBeads'
import { statusTone, statusDotClass, prettyState } from '~/utils/workflow'
import AgentLogPane from '~/components/orchestrator/AgentLogPane.vue'

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

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.bead) emit('close')
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 160ms ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
</style>
