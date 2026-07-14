<template>
  <UiModal
    :open="!!item"
    :title="item?.title || ''"
    kicker="PROCESS CAPTURE"
    size="lg"
    @close="$emit('close')"
  >
    <div class="flex flex-col gap-section">
      <!-- The capture itself, readable end to end. Triage is exactly when you
           need to read the whole thing, so it is never clipped away for good. -->
      <UiField label="Capture">
        <div class="rounded border border-border-hairline bg-surface-container-low px-base py-base">
          <p
            class="font-body text-body text-text-primary whitespace-pre-wrap"
            :class="expanded ? 'max-h-[40vh] overflow-y-auto' : 'line-clamp-3'"
          >{{ captureBody }}</p>
          <button
            v-if="isLongCapture"
            class="mt-tight font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
            @click="expanded = !expanded"
          >{{ expanded ? 'Collapse' : 'Show full capture' }}</button>
        </div>
      </UiField>

      <!-- One capture becomes N nodes: review, edit, add or drop each one. -->
      <div class="flex flex-col gap-base">
        <InboxActionEditor
          v-for="(action, i) in draft"
          :key="i"
          :action="action"
          :index="i"
          :projects="projects"
          :removable="draft.length > 1"
          @remove="draft.splice(i, 1)"
        />

        <UiButton variant="secondary" size="sm" icon="add" @click="addAction">Add action</UiButton>

        <p v-if="updateConflict" class="font-body text-body text-status-failed-text">
          An update is reviewed change by change against one note, so it has to be the only action.
          Remove the others, or pick a different target for this one.
        </p>
      </div>

      <!-- Stuck on where this goes? Argue it out with the DA. It only ever
           proposes: accepting rewrites the draft above, and you still process. -->
      <template v-if="item">
        <UiButton
          v-if="!daOpen"
          variant="ghost"
          size="sm"
          icon="neurology"
          @click="daOpen = true"
        >Ask the DA</UiButton>

        <InboxDaPanel
          v-else
          :capture-id="item.id"
          :draft="draftActions"
          class="-mx-section"
          @accept="onDaRouting"
          @close="daOpen = false"
        />
      </template>
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-base">
        <UiButton variant="ghost" @click="$emit('close')">Cancel</UiButton>
        <UiButton variant="primary" :loading="busy" :disabled="updateConflict" @click="confirm">
          {{ confirmLabel }}
        </UiButton>
      </div>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Project } from '~/composables/useProjects'
import type { InboxItemData } from '~/components/inbox/InboxRow.vue'
import InboxActionEditor from '~/components/inbox/InboxActionEditor.vue'
import InboxDaPanel from '~/components/inbox/InboxDaPanel.vue'
import {
  blankDraft,
  captureProvenance,
  fromDraft,
  normalizeActions,
  toDraft,
  type CaptureAction,
  type DraftAction,
} from '~/utils/inboxTargets'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiModal from '~/components/ui/UiModal.vue'

// A capture long enough that the collapsed view would hide something.
const LONG_CAPTURE_CHARS = 180

const props = defineProps<{
  item: InboxItemData | null
  projects: Project[]
  busy?: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', payload: { actions: CaptureAction[] }): void
}>()

const draft = ref<DraftAction[]>([])
const expanded = ref(false)
const daOpen = ref(false)

// What the DA is shown: the routing as it stands in the editor right now, not
// the suggestion it started from.
const draftActions = computed<CaptureAction[]>(() => draft.value.map(fromDraft))

// The DA proposes; this is where the user's acceptance lands. It replaces the
// whole draft — a routing is a set of nodes, not a patch — and still writes
// nothing: Process does that.
function onDaRouting(actions: CaptureAction[]) {
  draft.value = actions.map(a => toDraft(a, captureBody.value, provenance.value))
}

const captureBody = computed(() => props.item?.subtitle || props.item?.title || '')
const provenance = computed(() =>
  captureProvenance(props.item?.batchSource || props.item?.type, props.item?.batchTimestamp),
)
const isLongCapture = computed(() => captureBody.value.length > LONG_CAPTURE_CHARS || captureBody.value.includes('\n'))

const updateConflict = computed(
  () => draft.value.length > 1 && draft.value.some(a => a.target === 'update'),
)
const confirmLabel = computed(() =>
  draft.value.length > 1 ? `Process ${draft.value.length} actions` : 'Process',
)

// Re-seed the draft from the DA's suggested actions (or a single neutral note)
// each time a capture opens the modal.
watch(() => props.item, (item) => {
  if (!item) return
  expanded.value = false
  // A DA conversation belongs to the capture it was about.
  daOpen.value = false
  const suggested = normalizeActions(item.suggestedActions)
  draft.value = suggested.length > 0
    ? suggested.map(a => toDraft(a, captureBody.value, provenance.value))
    : [blankDraft(captureBody.value)]
}, { immediate: true })

function addAction() {
  if (!props.item) return
  draft.value.push(blankDraft(captureBody.value))
}

function confirm() {
  if (!props.item || draft.value.length === 0 || updateConflict.value) return
  emit('confirm', { actions: draft.value.map(fromDraft) })
}
</script>
