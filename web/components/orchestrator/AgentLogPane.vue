<template>
  <div ref="scrollEl" class="flex-1 overflow-y-auto font-mono-data text-mono-data leading-relaxed" @scroll.passive="onScroll">
    <!-- No session -->
    <div v-if="status === 'idle'" class="h-full flex flex-col items-center justify-center text-text-faint gap-tight">
      <span class="material-symbols-outlined !text-display">terminal</span>
      <p class="font-body text-body">No active session</p>
    </div>

    <div v-else-if="status === 'connecting' && blocks.length === 0" class="h-full flex items-center justify-center text-text-faint">
      <span class="font-body text-body">Connecting to session…</span>
    </div>

    <!-- Log stream -->
    <div v-else class="flex flex-col gap-tight py-base">
      <template v-for="(b, i) in blocks" :key="i">
        <!-- Turn separator -->
        <hr v-if="b.kind === 'separator'" class="border-0 border-t border-border-hairline my-base" />

        <!-- Thinking -->
        <div v-else-if="b.kind === 'thinking'" class="flex flex-col">
          <button
            class="flex items-center gap-tight text-status-gate text-left hover:text-status-gate/80 transition-colors"
            @click="b.collapsed = !b.collapsed"
          >
            <span class="material-symbols-outlined !text-body">{{ b.collapsed ? 'chevron_right' : 'expand_more' }}</span>
            <span class="font-label-caps text-mono-data tracking-widest">Thinking</span>
          </button>
          <p v-if="!b.collapsed" class="pl-[1em] mt-tight text-mono-data italic text-text-muted whitespace-pre-wrap">{{ b.text }}</p>
        </div>

        <!-- Plain text -->
        <p v-else-if="b.kind === 'text'" class="text-text-primary whitespace-pre-wrap">{{ b.text }}</p>

        <!-- Tool call -->
        <div v-else-if="b.kind === 'tool'" class="flex flex-col">
          <div class="flex items-center gap-tight">
            <span :class="toolMarkClass(b)">{{ toolMark(b) }}</span>
            <span class="text-primary">{{ b.name }}</span>
          </div>
          <pre v-if="b.input" class="m-0 font-mono-data pl-[1em] text-text-muted whitespace-pre-wrap break-all">{{ truncate(b.input, b.expanded) }}</pre>
          <pre v-if="b.result" class="m-0 font-mono-data pl-[1em] whitespace-pre-wrap break-all" :class="b.error ? 'text-status-failed-text' : 'text-status-passed'">↳ {{ truncate(b.result, b.expanded) }}</pre>
          <button
            v-if="isTruncated(b)"
            class="self-start pl-[1em] text-text-faint hover:text-text-muted transition-colors"
            @click="b.expanded = !b.expanded"
          >{{ b.expanded ? '…less' : '…more' }}</button>
        </div>

        <!-- Stderr -->
        <p v-else-if="b.kind === 'stderr'" class="text-status-failed-text whitespace-pre-wrap">[stderr] {{ b.text }}</p>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onBeforeUnmount } from 'vue'

const props = defineProps<{
  epicId: string
  beadId: string
}>()

type Block =
  | { kind: 'separator' }
  | { kind: 'thinking'; text: string; collapsed: boolean; key?: string }
  | { kind: 'text'; text: string; key?: string }
  | { kind: 'tool'; name: string; input: string; result: string; error: boolean; expanded: boolean; key?: string }
  | { kind: 'stderr'; text: string }

const blocks = ref<Block[]>([])
const status = ref<'idle' | 'connecting' | 'open'>('connecting')

const scrollEl = ref<HTMLElement | null>(null)
let stick = true
let es: EventSource | null = null

function onScroll() {
  const el = scrollEl.value
  if (!el) return
  stick = el.scrollHeight - el.scrollTop - el.clientHeight < 40
}

let rafScroll = 0
function scrollToBottom() {
  if (!stick) return
  cancelAnimationFrame(rafScroll)
  rafScroll = requestAnimationFrame(() => {
    const el = scrollEl.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

// ── truncation helpers (3 lines / 240 chars) ────────────────────────────────
const MAX_CHARS = 240
const MAX_LINES = 3
function isTruncated(b: Extract<Block, { kind: 'tool' }>): boolean {
  const longest = Math.max(b.input.length, b.result.length)
  const lines = Math.max(b.input.split('\n').length, b.result.split('\n').length)
  return longest > MAX_CHARS || lines > MAX_LINES
}
function truncate(s: string, expanded: boolean): string {
  if (expanded) return s
  let out = s.split('\n').slice(0, MAX_LINES).join('\n')
  if (out.length > MAX_CHARS) out = out.slice(0, MAX_CHARS)
  return out.length < s.length ? out + '…' : out
}

function toolMark(b: Extract<Block, { kind: 'tool' }>): string {
  if (!b.result) return '▶'
  return b.error ? '✗' : '✓'
}
function toolMarkClass(b: Extract<Block, { kind: 'tool' }>): string {
  if (!b.result) return 'text-primary'
  return b.error ? 'text-status-failed-text' : 'text-status-passed'
}

// ── block append / merge helpers ────────────────────────────────────────────
function appendText(text: string, key?: string) {
  const last = blocks.value[blocks.value.length - 1]
  if (last && last.kind === 'text' && (!key || last.key === key)) {
    last.text += text
  } else {
    blocks.value.push({ kind: 'text', text, key })
  }
}
function appendThinking(text: string, key?: string) {
  const last = blocks.value[blocks.value.length - 1]
  if (last && last.kind === 'thinking' && (!key || last.key === key)) {
    last.text += text
    last.collapsed = false
  } else {
    blocks.value.push({ kind: 'thinking', text, collapsed: false, key })
  }
}
function upsertTool(opts: { key: string; name?: string; input?: string; result?: string; error?: boolean }) {
  let tool = blocks.value.find(
    (b): b is Extract<Block, { kind: 'tool' }> => b.kind === 'tool' && b.key === opts.key
  )
  if (!tool) {
    tool = { kind: 'tool', name: opts.name || 'tool', input: opts.input || '', result: '', error: false, expanded: false, key: opts.key }
    blocks.value.push(tool)
  }
  if (opts.name) tool.name = opts.name
  if (opts.input !== undefined) tool.input = opts.input
  if (opts.result !== undefined) tool.result = opts.result
  if (opts.error !== undefined) tool.error = opts.error
}
function collapseThinking() {
  for (const b of blocks.value) if (b.kind === 'thinking') b.collapsed = true
}
function separator() {
  const last = blocks.value[blocks.value.length - 1]
  collapseThinking()
  if (!last || last.kind === 'separator') return
  blocks.value.push({ kind: 'separator' })
}

// ── inner-event router (opencode + Claude Code dialects) ─────────────────────
function handleInner(inner: any) {
  if (!inner || typeof inner !== 'object') return
  const type = inner.type

  // opencode dialect
  if (type === 'message.part.updated' && inner.part) {
    const part = inner.part
    const key = part.id || part.messageID || ''
    if (part.type === 'text') appendText(part.text || '', 'oc-text-' + key)
    else if (part.type === 'reasoning') appendThinking(part.text || '', 'oc-think-' + key)
    else if (part.type === 'tool') {
      const state = part.state || {}
      const name = part.tool || state.title || 'tool'
      const input = state.input ? stringifyInput(state.input) : ''
      const hasResult = state.status === 'completed' || state.status === 'error' || state.output !== undefined
      upsertTool({
        key: 'oc-tool-' + (part.callID || part.id || key),
        name,
        input,
        result: hasResult ? String(state.output ?? '') : undefined,
        error: state.status === 'error',
      })
    }
    scrollToBottom()
    return
  }
  if (type === 'session_idle') { separator(); scrollToBottom(); return }

  // Claude Code dialect
  if (type === 'assistant' && inner.message?.content) {
    for (const block of inner.message.content) {
      if (block.type === 'text') appendText(block.text || '', 'cc-text-' + (inner.message.id || ''))
      else if (block.type === 'thinking') appendThinking(block.thinking || '', 'cc-think-' + (inner.message.id || ''))
      else if (block.type === 'tool_use') {
        upsertTool({ key: 'cc-tool-' + block.id, name: block.name || 'tool', input: stringifyInput(block.input) })
      }
    }
    scrollToBottom()
    return
  }
  if (type === 'tool_use') {
    upsertTool({ key: 'cc-tool-' + inner.id, name: inner.name || 'tool', input: stringifyInput(inner.input) })
    scrollToBottom()
    return
  }
  if (type === 'tool_result') {
    const content = Array.isArray(inner.content)
      ? inner.content.map((c: any) => c.text ?? '').join('\n')
      : String(inner.content ?? '')
    upsertTool({ key: 'cc-tool-' + (inner.tool_use_id || ''), result: content, error: !!inner.is_error })
    scrollToBottom()
    return
  }
  if (type === 'result') { separator(); scrollToBottom(); return }
  // unknown inner types are dropped silently
}

function stringifyInput(input: any): string {
  if (input == null) return ''
  if (typeof input === 'string') return input
  try {
    // Prefer the salient single arg if there's one obvious string field.
    const entries = Object.entries(input)
    if (entries.length === 1 && typeof entries[0][1] === 'string') return entries[0][1] as string
    return JSON.stringify(input)
  } catch {
    return String(input)
  }
}

function handleTerminalEvent(raw: string) {
  let ev: any
  try { ev = JSON.parse(raw) } catch { return }
  if (ev.type === 'stdout') {
    let inner: any
    try { inner = JSON.parse(ev.content) } catch { return }
    handleInner(inner)
  } else if (ev.type === 'stderr') {
    blocks.value.push({ kind: 'stderr', text: String(ev.content ?? '') })
    scrollToBottom()
  }
  // 'exit' and other outer types ignored
}

// ── session lifecycle ───────────────────────────────────────────────────────
function closeStream() {
  if (es) { es.close(); es = null }
}

async function start() {
  closeStream()
  blocks.value = []
  stick = true
  status.value = 'connecting'
  let sessionId: string | null = null
  try {
    const res = await fetch(`/api/epics/${encodeURIComponent(props.epicId)}/sessions`)
    if (res.ok) {
      const sessions: Array<{ sessionId: string; beadId: string }> = await res.json()
      const match = sessions.find((s) => s.beadId === props.beadId)
      sessionId = match?.sessionId ?? null
    }
  } catch { /* fall through to idle */ }

  if (!sessionId) { status.value = 'idle'; return }

  es = new EventSource(`/api/sessions/${encodeURIComponent(sessionId)}/events`)
  es.onopen = () => { status.value = 'open' }
  es.onmessage = (e) => { status.value = 'open'; handleTerminalEvent(e.data) }
  es.onerror = () => { /* EventSource auto-retries; keep the buffer visible */ }
}

watch(() => [props.epicId, props.beadId], start, { immediate: true })
onBeforeUnmount(closeStream)
</script>
