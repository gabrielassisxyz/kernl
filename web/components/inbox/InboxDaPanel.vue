<template>
  <div class="flex flex-col gap-base border-t border-da-accent/30 bg-da-accent/[0.04] px-component py-base">
    <div class="flex items-center gap-base">
      <span class="font-mono-data text-mono-data text-da-accent-text">Ask the DA</span>
      <span class="font-mono-data text-mono-data text-text-dim truncate">where should this go, and why</span>
      <button
        v-if="messages.length > 0"
        class="ml-auto shrink-0 font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
        @click="reset"
      >Clear</button>
    </div>

    <!-- The conversation. It stays short by design: this is a question about one
         capture, not a chat session you scroll. -->
    <ul v-if="messages.length > 0" class="flex flex-col gap-base max-h-[28vh] overflow-y-auto">
      <li v-for="(m, i) in messages" :key="i" class="flex gap-base min-w-0">
        <span
          class="shrink-0 w-[36px] font-mono-data text-mono-data"
          :class="m.role === 'user' ? 'text-text-faint' : 'text-da-accent-text'"
        >{{ m.role === 'user' ? 'you' : 'DA' }}</span>
        <p class="font-body text-body text-text-primary whitespace-pre-wrap min-w-0">{{ m.content }}</p>
      </li>
    </ul>

    <!-- The proposal: the nodes the DA thinks this capture should become.
         Accepting only replaces what is on screen — the capture is still written
         by the user, when they process it. -->
    <div v-if="routing" class="flex flex-col gap-tight rounded border border-da-accent/40 bg-surface p-base">
      <p v-if="routing.rationale" class="font-body text-body text-text-muted">{{ routing.rationale }}</p>

      <ul class="flex flex-col gap-tight">
        <li v-for="(action, i) in proposed" :key="i" class="flex items-baseline gap-base min-w-0">
          <span
            class="shrink-0 w-[84px] flex items-center gap-tight px-tight rounded border font-mono-data text-mono-data"
            :class="TARGET_META[action.target].chip"
          >
            <span class="material-symbols-outlined !text-body leading-none" aria-hidden="true">{{ TARGET_META[action.target].icon }}</span>
            {{ TARGET_META[action.target].label }}
          </span>
          <span class="font-body text-body text-text-primary truncate">{{ action.title }}</span>
          <span v-if="action.dueDate" class="shrink-0 ml-auto font-mono-data text-mono-data text-tertiary">{{ action.dueDate }}</span>
        </li>
      </ul>

      <!-- "Accept" promised a write this button does not do. It updates the nodes
           on screen; processing the capture is still a separate, deliberate act. -->
      <div class="flex items-center gap-base pt-tight font-mono-data text-mono-data">
        <span class="text-text-dim truncate">Updates the nodes above — nothing is written yet.</span>
        <div class="ml-auto shrink-0 flex items-center gap-base">
          <button
            class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-text-primary transition-colors cursor-pointer"
            @click="dismissRouting"
          >Dismiss</button>
          <button
            class="px-base py-0.5 rounded border border-status-passed/40 text-status-passed hover:bg-status-passed/10 transition-colors cursor-pointer"
            @click="acceptRouting"
          >Update</button>
        </div>
      </div>
    </div>

    <!-- An edit to a note that ALREADY EXISTS — "add this book to Anti-library".
         It is not one of the nodes the capture becomes (an update merges hunk by
         hunk and cannot ride along with a fan-out), so it is its own proposal,
         and it is the one thing here that writes to the vault the moment you say
         yes. Warned in words and in colour, because the button beside it does not. -->
    <div v-if="noteEdit" class="flex flex-col gap-tight rounded border border-status-gate/50 bg-status-gate/[0.06] p-base">
      <div class="flex items-baseline gap-base min-w-0">
        <span class="shrink-0 flex items-center gap-tight px-tight rounded border border-status-gate/50 text-tertiary font-mono-data text-mono-data">
          <span class="material-symbols-outlined !text-body leading-none" aria-hidden="true">sync</span>
          Edit
        </span>
        <span class="font-body text-body text-text-primary truncate">{{ noteEditTarget }}</span>
      </div>

      <pre
        v-for="hunk in noteEdit.hunks"
        :key="hunk.id"
        class="max-h-[18vh] overflow-auto rounded bg-surface px-base py-tight font-mono-data text-mono-data text-status-passed whitespace-pre-wrap"
      >{{ hunk.content }}</pre>

      <div class="flex items-center gap-base pt-tight font-mono-data text-mono-data">
        <span class="text-status-gate truncate">Writes to the note immediately.</span>
        <div class="ml-auto shrink-0 flex items-center gap-base">
          <button
            class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-text-primary transition-colors cursor-pointer"
            @click="dismissDiff"
          >Dismiss</button>
          <button
            class="px-base py-0.5 rounded border border-status-gate/50 text-tertiary hover:bg-status-gate/10 transition-colors cursor-pointer disabled:opacity-45"
            :disabled="writing"
            @click="writeNoteEdit"
          >{{ writing ? 'Writing…' : 'Write to note' }}</button>
        </div>
      </div>
    </div>

    <div class="flex items-center gap-base">
      <input
        ref="inputEl"
        v-model="question"
        class="flex-1 min-w-0 h-8 rounded border border-border-hairline bg-surface px-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-faint focus:border-primary/70"
        :placeholder="isStreaming ? 'The DA is thinking…' : 'Why a task? Should this be two nodes?'"
        :disabled="isStreaming"
        @keydown.enter.prevent="ask"
        @keydown.escape.prevent.stop="$emit('close')"
      />
      <button
        class="shrink-0 px-base py-1 rounded border border-da-accent/40 text-da-accent-text hover:bg-da-accent/10 transition-colors font-mono-data text-mono-data disabled:opacity-45 disabled:cursor-not-allowed cursor-pointer"
        :disabled="isStreaming || !question.trim()"
        @click="ask"
      >Ask</button>
    </div>

    <p v-if="error" class="font-body text-body text-status-failed-text">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useChatSession } from '~/composables/useChatSession'
import {
  TARGET_META,
  normalizeActions,
  type CaptureAction,
} from '~/utils/inboxTargets'

const props = defineProps<{
  /** the capture under discussion — it scopes the DA's session */
  captureId: string
  /** the routing currently on screen, so the DA argues with THIS, not its own first guess */
  draft: CaptureAction[]
}>()

const emit = defineEmits<{
  /** the user accepted the DA's routing: it replaces the draft */
  (e: 'accept', actions: CaptureAction[]): void
  (e: 'close'): void
}>()

const {
  messages,
  routingSuggestion,
  diffSuggestion,
  isStreaming,
  error,
  sendMessage,
  dismissRouting,
  applyDiff,
  dismissDiff,
  newConversation,
} = useChatSession()

const question = ref('')
const writing = ref(false)
const inputEl = ref<HTMLInputElement | null>(null)

// The DA's other proposal: an edit to a note that already exists. It arrives on
// its own event because it is not a node the capture becomes — "add this book to
// Anti-library" touches a note that was already there.
const noteEdit = computed(() => diffSuggestion.value)

// "Anti-library.md" reads better than a uuid when you are deciding whether to
// let something write to it.
const noteEditTarget = computed(() => {
  const path = diffSuggestion.value?.notePath || ''
  return path.split('/').pop() || 'the matching note'
})

// The one button here that actually writes. The routing card only redraws the
// nodes on screen; this one changes a file in the vault.
async function writeNoteEdit() {
  const edit = noteEdit.value
  if (!edit || writing.value) return
  writing.value = true
  try {
    await applyDiff(edit.hunks)
  } catch (e) {
    console.error('apply note edit failed', e)
  } finally {
    writing.value = false
  }
}

const routing = computed(() => {
  // A routing proposed for a different capture is stale — the panel is mounted
  // per capture, but the session is not.
  const r = routingSuggestion.value
  return r && r.captureId === props.captureId ? r : null
})

// An unrenderable target is dropped rather than blanking the whole proposal,
// exactly as the row and the drawer do.
const proposed = computed<CaptureAction[]>(() =>
  routing.value ? normalizeActions(routing.value.actions) : [],
)

async function ask() {
  const text = question.value.trim()
  if (!text || isStreaming.value) return
  question.value = ''
  await sendMessage(text, props.captureId, props.draft)
}

function acceptRouting() {
  if (proposed.value.length === 0) return
  emit('accept', proposed.value)
  dismissRouting()
}

function reset() {
  newConversation()
  question.value = ''
}

onMounted(async () => {
  await nextTick()
  inputEl.value?.focus()
})
</script>
