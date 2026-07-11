<template>
  <div class="settings-page">
    <header class="settings-header">
      <div class="settings-title">
        <h1 class="font-headline text-display text-text-primary">Settings</h1>
        <p class="settings-subtitle">
          <span v-if="settings?.configPath">
            Saved to <code class="settings-path">{{ settings.configPath }}</code>
          </span>
          <span v-else>Configure the assistant, editor, and runtime.</span>
        </p>
      </div>
      <UiButton variant="secondary" size="sm" icon="refresh" :loading="loading" @click="load">
        Reload
      </UiButton>
    </header>

    <div v-if="restartPending.length" class="restart-banner" role="status">
      <span class="material-symbols-outlined restart-icon" aria-hidden="true">warning</span>
      <div>
        <p class="restart-title">Restart Kernl to apply {{ restartPending.length }} saved change{{ restartPending.length === 1 ? '' : 's' }}</p>
        <p class="restart-detail">
          They are written to the config file, but this process is still running the values it booted with.
          The affected fields are marked under
          <span class="restart-keys">{{ pendingSections.join(', ') }}</span>.
        </p>
      </div>
    </div>

    <nav class="settings-tabs" aria-label="Settings sections">
      <button
        v-for="tab in TABS"
        :key="tab.id"
        type="button"
        class="settings-tab"
        :class="{ 'settings-tab--active': activeTab === tab.id }"
        :aria-current="activeTab === tab.id ? 'page' : undefined"
        @click="setActiveTab(tab.id)"
      >
        {{ tab.label }}
      </button>
    </nav>

    <UiErrorState
      v-if="loadError"
      bordered
      title="Settings unavailable"
      message="Kernl could not read the settings from the active server."
      :detail="loadError"
      @retry="load"
    />

    <div v-else-if="loading" class="settings-loading">
      <UiSkeleton v-for="index in 4" :key="index" classes="h-[76px]" />
    </div>

    <main v-else class="settings-content">
      <!-- Assistant -->
      <section v-if="activeTab === 'assistant'" class="settings-panel">
        <div class="panel-heading">
          <h2>Assistant</h2>
          <p>The name and base instruction of the local assistant. Stored in the graph, applied immediately.</p>
        </div>

        <UiErrorState
          v-if="daError"
          bordered
          title="Assistant identity unavailable"
          message="Kernl could not load the assistant identity."
          :detail="daError"
          @retry="loadDAIdentity"
        />

        <form v-else class="form-stack" @submit.prevent="saveDAIdentity">
          <SettingsFieldRow
            input-id="da-display-name"
            label="Display name"
            description="The human-facing name shown for the assistant."
            setting-key="da.displayName"
          >
            <UiInput id="da-display-name" v-model="da.displayName" :disabled="daLoading || daSaving" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="da-system-prompt"
            label="System prompt"
            description="The base instruction the assistant carries into every conversation."
            setting-key="da.systemPrompt"
            stacked
          >
            <UiTextarea
              id="da-system-prompt"
              v-model="da.systemPrompt"
              rows="14"
              :disabled="daLoading || daSaving"
              classes="w-full rounded border border-border-hairline bg-bg-base px-component py-base font-mono-data text-mono-data text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50 resize-y"
            />
          </SettingsFieldRow>

          <div class="panel-footer">
            <span class="save-status" :class="daStatus.tone">{{ daStatus.text }}</span>
            <UiButton type="submit" variant="accent" icon="save" :loading="daSaving" :disabled="!daDirty || daLoading">
              Save assistant
            </UiButton>
          </div>
        </form>
      </section>

      <!-- Editor -->
      <section v-else-if="activeTab === 'editor'" class="settings-panel">
        <div class="panel-heading">
          <h2>Editor</h2>
          <p>Note editor preferences. Stored in this browser, applied as you change them.</p>
        </div>

        <div class="form-stack">
          <SettingsFieldRow
            input-id="editor-view-mode"
            label="View mode"
            description="How markdown notes open by default."
            setting-key="editor.viewMode"
          >
            <UiSelect id="editor-view-mode" v-model="editor.viewMode">
              <option value="source">Source</option>
              <option value="live">Live preview</option>
              <option value="reading">Reading</option>
            </UiSelect>
          </SettingsFieldRow>

          <SettingsSwitchRow
            label="Line numbers"
            description="Show line numbers in source and live editor modes."
            setting-key="editor.lineNumbers"
            :checked="editor.lineNumbers"
            @toggle="editor.lineNumbers = !editor.lineNumbers"
          />
          <SettingsSwitchRow
            label="Typewriter mode"
            description="Keep the active line near the visual center."
            setting-key="editor.typewriter"
            :checked="editor.typewriter"
            @toggle="editor.typewriter = !editor.typewriter"
          />
          <SettingsSwitchRow
            label="Show note ID"
            description="Expose the graph node ID in note properties."
            setting-key="editor.showId"
            :checked="editor.showId"
            @toggle="editor.showId = !editor.showId"
          />

          <SettingsFieldRow
            input-id="editor-font"
            label="Editor font"
            description="The reading and editing texture of notes."
            setting-key="editor.font"
          >
            <UiSelect id="editor-font" v-model="editor.font">
              <option value="sans">Sans</option>
              <option value="serif">Serif</option>
              <option value="mono">Mono</option>
            </UiSelect>
          </SettingsFieldRow>

          <SettingsStepperRow
            label="Font size"
            description="Editor text size."
            setting-key="editor.fontSize"
            :value="editor.fontSize"
            suffix="px"
            :min="FONT_SIZE_MIN"
            :max="FONT_SIZE_MAX"
            @decrease="setFontSize(editor.fontSize - 1)"
            @increase="setFontSize(editor.fontSize + 1)"
          />
          <SettingsStepperRow
            label="Heading scale"
            description="Markdown heading scale in the note editor."
            setting-key="editor.headingScale"
            :value="Math.round(editor.headingScale * 100)"
            suffix="%"
            :min="Math.round(HEADING_SCALE_MIN * 100)"
            :max="Math.round(HEADING_SCALE_MAX * 100)"
            @decrease="setHeadingScale(editor.headingScale - 0.05)"
            @increase="setHeadingScale(editor.headingScale + 0.05)"
          />
        </div>

        <div class="panel-footer">
          <span class="save-status text-text-muted">Saved on this device as you change them.</span>
          <UiButton variant="secondary" icon="refresh" @click="resetEditor">Reset to defaults</UiButton>
        </div>
      </section>

      <!-- LLM -->
      <section v-else-if="activeTab === 'llm'" class="settings-panel">
        <div class="panel-heading">
          <h2>LLM</h2>
          <p>The provider behind chat, ingest, and note AI features.</p>
        </div>

        <form class="form-stack" @submit.prevent="save('llm')">
          <SettingsFieldRow
            input-id="llm-provider"
            label="Provider"
            description="Which client Kernl builds to talk to the model."
            setting-key="llm.provider"
            :pending="isPending('llm.provider')"
          >
            <UiSelect id="llm-provider" v-model="llm.provider" :disabled="saving === 'llm'">
              <option value="">Disabled</option>
              <option value="openai">OpenAI-compatible</option>
              <option value="anthropic">Anthropic</option>
              <option value="ollama">Ollama</option>
              <option value="noop">No-op (testing)</option>
            </UiSelect>
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="llm-model"
            label="Model"
            description="The model identifier sent to the provider."
            setting-key="llm.model"
            :pending="isPending('llm.model')"
          >
            <UiInput id="llm-model" v-model="llm.model" placeholder="kimi-k2.7" :disabled="saving === 'llm'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="llm-endpoint"
            label="Endpoint"
            description="Custom base URL. Leave empty to use the provider default."
            setting-key="llm.endpoint"
            :pending="isPending('llm.endpoint')"
          >
            <UiInput
              id="llm-endpoint"
              v-model="llm.endpoint"
              placeholder="http://localhost:4000"
              :disabled="saving === 'llm'"
            />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="llm-api-key"
            label="API key"
            :description="apiKeyDescription"
            setting-key="llm.api_key"
            :pending="isPending('llm.api_key')"
          >
            <div class="key-control">
              <UiInput
                id="llm-api-key"
                v-model="llm.apiKey"
                type="password"
                :placeholder="apiKeyPlaceholder"
                :disabled="saving === 'llm' || llm.clearKey"
              />
              <label class="key-clear">
                <input v-model="llm.clearKey" type="checkbox" :disabled="saving === 'llm'" />
                Remove the stored key
              </label>
            </div>
          </SettingsFieldRow>

          <div class="panel-footer">
            <span class="save-status" :class="status.llm.tone">{{ status.llm.text }}</span>
            <UiButton type="submit" variant="accent" icon="save" :loading="saving === 'llm'" :disabled="!dirty.llm">
              Save LLM
            </UiButton>
          </div>
        </form>
      </section>

      <!-- Vault -->
      <section v-else-if="activeTab === 'vault'" class="settings-panel">
        <div class="panel-heading">
          <h2>Vault</h2>
          <p>Where notes live on disk and how closely the watcher follows them.</p>
        </div>

        <form class="form-stack" @submit.prevent="save('vault')">
          <SettingsFieldRow
            input-id="vault-root"
            label="Vault root"
            description="Absolute path to the notes vault. Must already exist."
            setting-key="vault.root"
            :pending="isPending('vault.root')"
          >
            <UiInput id="vault-root" v-model="vault.root" placeholder="/home/you/vault" :disabled="saving === 'vault'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="vault-coalesce"
            label="Coalesce window"
            description="Quiet period before a filesystem change is emitted, in milliseconds."
            setting-key="vault.coalesceWindowMs"
            :pending="isPending('vault.coalesceWindowMs')"
          >
            <UiInput id="vault-coalesce" v-model="vault.coalesceWindowMs" type="number" :disabled="saving === 'vault'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="vault-move"
            label="Move window"
            description="How long a delete waits to be recognized as a move, in milliseconds."
            setting-key="vault.moveWindowMs"
            :pending="isPending('vault.moveWindowMs')"
          >
            <UiInput id="vault-move" v-model="vault.moveWindowMs" type="number" :disabled="saving === 'vault'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="vault-rescan"
            label="Rescan interval"
            description="Periodic full rescan, in seconds. Zero disables it."
            setting-key="vault.rescanIntervalSec"
            :pending="isPending('vault.rescanIntervalSec')"
          >
            <UiInput id="vault-rescan" v-model="vault.rescanIntervalSec" type="number" :disabled="saving === 'vault'" />
          </SettingsFieldRow>

          <div class="panel-footer">
            <span class="save-status" :class="status.vault.tone">{{ status.vault.text }}</span>
            <UiButton type="submit" variant="accent" icon="save" :loading="saving === 'vault'" :disabled="!dirty.vault">
              Save vault
            </UiButton>
          </div>
        </form>
      </section>

      <!-- Inbox -->
      <section v-else-if="activeTab === 'inbox'" class="settings-panel">
        <div class="panel-heading">
          <h2>Inbox</h2>
          <p>How the assistant pre-processes captures before you triage them.</p>
        </div>

        <form class="form-stack" @submit.prevent="save('inbox')">
          <SettingsSwitchRow
            label="Auto prep"
            description="Let the classifier write a primer for captures it reads as questions. Manual prep works either way."
            setting-key="inbox.auto_prep"
            :checked="inbox.autoPrep"
            :pending="isPending('inbox.auto_prep')"
            @toggle="inbox.autoPrep = !inbox.autoPrep"
          />

          <SettingsFieldRow
            input-id="inbox-subdir"
            label="DA subdirectory"
            description="Folder inside the vault where assistant-authored notes are written."
            setting-key="inbox.da_subdir"
            :pending="isPending('inbox.da_subdir')"
          >
            <UiInput id="inbox-subdir" v-model="inbox.daSubdir" placeholder="DA" :disabled="saving === 'inbox'" />
          </SettingsFieldRow>

          <div class="panel-footer">
            <span class="save-status" :class="status.inbox.tone">{{ status.inbox.text }}</span>
            <UiButton type="submit" variant="accent" icon="save" :loading="saving === 'inbox'" :disabled="!dirty.inbox">
              Save inbox
            </UiButton>
          </div>
        </form>
      </section>

      <!-- Runtime -->
      <section v-else-if="activeTab === 'runtime'" class="settings-panel">
        <div class="panel-heading">
          <h2>Runtime</h2>
          <p>Server, orchestrator, and sweep values. All of these take effect on the next start.</p>
        </div>

        <form class="form-stack" @submit.prevent="save('runtime')">
          <SettingsFieldRow
            input-id="runtime-port"
            label="Server port"
            description="Port the API and UI listen on."
            setting-key="server.port"
            :pending="isPending('server.port')"
          >
            <UiInput id="runtime-port" v-model="runtime.serverPort" type="number" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="runtime-worktree"
            label="Worktree root"
            description="Absolute path where per-bead git worktrees are created."
            setting-key="orchestrator.worktreeRoot"
            :pending="isPending('orchestrator.worktreeRoot')"
          >
            <UiInput id="runtime-worktree" v-model="runtime.worktreeRoot" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="runtime-max-beads"
            label="Max concurrent beads"
            description="How many beads one epic wave runs in parallel."
            setting-key="orchestrator.maxConcurrentBeads"
            :pending="isPending('orchestrator.maxConcurrentBeads')"
          >
            <UiInput id="runtime-max-beads" v-model="runtime.maxConcurrentBeads" type="number" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="runtime-runstate"
            label="Run-state path"
            description="Absolute path to the SQLite run-state database."
            setting-key="orchestrator.runStatePath"
            :pending="isPending('orchestrator.runStatePath')"
          >
            <UiInput id="runtime-runstate" v-model="runtime.runStatePath" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="runtime-retries"
            label="Stage retry attempts"
            description="How many times a failing stage is retried before the bead stops."
            setting-key="orchestrator.stageRetryAttempts"
            :pending="isPending('orchestrator.stageRetryAttempts')"
          >
            <UiInput id="runtime-retries" v-model="runtime.stageRetryAttempts" type="number" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="runtime-sweep-interval"
            label="Sweep interval"
            description="How often the sweeper ticks, in seconds. Zero disables the auto-tick."
            setting-key="sweep.auto_interval_seconds"
            :pending="isPending('sweep.auto_interval_seconds')"
          >
            <UiInput id="runtime-sweep-interval" v-model="runtime.sweepIntervalSec" type="number" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="runtime-stale-days"
            label="PR stale warning"
            description="Days before the sweeper flags a pull request as stale."
            setting-key="sweep.pr_stale_warn_days"
            :pending="isPending('sweep.pr_stale_warn_days')"
          >
            <UiInput id="runtime-stale-days" v-model="runtime.prStaleWarnDays" type="number" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="runtime-failure-limit"
            label="Failure threshold"
            description="Consecutive failures before the sweeper backs off."
            setting-key="sweep.failure_threshold"
            :pending="isPending('sweep.failure_threshold')"
          >
            <UiInput id="runtime-failure-limit" v-model="runtime.sweepFailureLimit" type="number" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <SettingsFieldRow
            input-id="runtime-backoff"
            label="Backoff schedule"
            description="Minutes to wait between retries, as a comma-separated list."
            setting-key="sweep.backoff_minutes"
            :pending="isPending('sweep.backoff_minutes')"
          >
            <UiInput id="runtime-backoff" v-model="runtime.sweepBackoffMinutes" placeholder="5, 15, 60" :disabled="saving === 'runtime'" />
          </SettingsFieldRow>

          <div class="panel-footer">
            <span class="save-status" :class="status.runtime.tone">{{ status.runtime.text }}</span>
            <UiButton type="submit" variant="accent" icon="save" :loading="saving === 'runtime'" :disabled="!dirty.runtime">
              Save runtime
            </UiButton>
          </div>
        </form>
      </section>

      <!-- Prompts / Agents: read-only for a stated reason -->
      <section v-else class="settings-panel">
        <div class="panel-heading">
          <h2>{{ activeTab === 'prompts' ? 'Prompts' : 'Agents' }}</h2>
          <p>{{ readOnlyBlurb }}</p>
        </div>

        <div class="form-stack">
          <SettingsInventoryRow
            v-for="row in activeTab === 'prompts' ? settings?.prompts || [] : settings?.agents || []"
            :key="row.key"
            :row="row"
          />
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import {
  FONT_SIZE_MAX,
  FONT_SIZE_MIN,
  HEADING_SCALE_MAX,
  HEADING_SCALE_MIN,
  useEditorSettings,
} from '~/composables/useEditorSettings'
import UiButton from '~/components/ui/UiButton.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiSelect from '~/components/ui/UiSelect.vue'
import UiSkeleton from '~/components/ui/UiSkeleton.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'
import SettingsFieldRow from '~/components/settings/SettingsFieldRow.vue'
import SettingsInventoryRow from '~/components/settings/SettingsInventoryRow.vue'
import SettingsStepperRow from '~/components/settings/SettingsStepperRow.vue'
import SettingsSwitchRow from '~/components/settings/SettingsSwitchRow.vue'

type TabID = 'assistant' | 'editor' | 'llm' | 'vault' | 'inbox' | 'runtime' | 'prompts' | 'agents'
type ConfigSection = 'llm' | 'vault' | 'inbox' | 'runtime'
type SaveStatus = { text: string; tone: string }

interface ReadOnlySettingRow {
  key: string
  label: string
  description: string
  value: string
  source: string
  reason: string
}

interface SettingsResponse {
  configPath: string
  writable: boolean
  restartPending: string[]
  llm: { provider: string; model: string; endpoint: string; apiKeySet: boolean }
  vault: { root: string; coalesceWindowMs: number; moveWindowMs: number; rescanIntervalSec: number }
  inbox: { autoPrep: boolean; daSubdir: string }
  runtime: {
    serverPort: number
    worktreeRoot: string
    maxConcurrentBeads: number
    runStatePath: string
    stageRetryAttempts: number
    sweepIntervalSec: number
    prStaleWarnDays: number
    sweepFailureLimit: number
    sweepBackoffMinutes: number[]
  }
  prompts: ReadOnlySettingRow[]
  agents: ReadOnlySettingRow[]
}

const TABS: { id: TabID; label: string }[] = [
  { id: 'assistant', label: 'Assistant' },
  { id: 'editor', label: 'Editor' },
  { id: 'llm', label: 'LLM' },
  { id: 'vault', label: 'Vault' },
  { id: 'inbox', label: 'Inbox' },
  { id: 'runtime', label: 'Runtime' },
  { id: 'prompts', label: 'Prompts' },
  { id: 'agents', label: 'Agents' },
]

const IDLE: SaveStatus = { text: '', tone: 'text-text-muted' }

const route = useRoute()
const router = useRouter()
const { settings: editor, setFontSize, setHeadingScale, reset: resetEditor } = useEditorSettings()

const settings = ref<SettingsResponse | null>(null)
const loading = ref(false)
const loadError = ref('')
const activeTab = ref<TabID>(readTab(route.query.tab))
const saving = ref<ConfigSection | ''>('')
const status = reactive<Record<ConfigSection, SaveStatus>>({
  llm: { ...IDLE },
  vault: { ...IDLE },
  inbox: { ...IDLE },
  runtime: { ...IDLE },
})

// Config forms are edited as strings so a half-typed number never collapses to 0.
const llm = reactive({ provider: '', model: '', endpoint: '', apiKey: '', clearKey: false })
const vault = reactive({ root: '', coalesceWindowMs: '', moveWindowMs: '', rescanIntervalSec: '' })
const inbox = reactive({ autoPrep: false, daSubdir: '' })
const runtime = reactive({
  serverPort: '',
  worktreeRoot: '',
  maxConcurrentBeads: '',
  runStatePath: '',
  stageRetryAttempts: '',
  sweepIntervalSec: '',
  prStaleWarnDays: '',
  sweepFailureLimit: '',
  sweepBackoffMinutes: '',
})

const da = reactive({ displayName: '', systemPrompt: '' })
const daSaved = reactive({ displayName: '', systemPrompt: '' })
const daLoading = ref(false)
const daSaving = ref(false)
const daError = ref('')
const daStatus = ref<SaveStatus>({ ...IDLE })

// The last payload written to the server, per section. Dirty state is a
// comparison against this, so a save that lands leaves the form clean.
const baseline = reactive<Record<ConfigSection, string>>({ llm: '', vault: '', inbox: '', runtime: '' })

const restartPending = computed(() => settings.value?.restartPending || [])

// A pending key ("sweep.backoff_minutes") tells the user nothing about where to
// look for it, so the banner points at the tabs that carry the marked fields.
const SECTION_OF_KEY: Record<string, string> = {
  llm: 'LLM',
  vault: 'Vault',
  inbox: 'Inbox',
  server: 'Runtime',
  orchestrator: 'Runtime',
  sweep: 'Runtime',
}
const pendingSections = computed(() => [
  ...new Set(restartPending.value.map((key) => SECTION_OF_KEY[key.split('.')[0]]).filter(Boolean)),
])
const daDirty = computed(
  () => da.displayName !== daSaved.displayName || da.systemPrompt !== daSaved.systemPrompt,
)
const dirty = computed(() => ({
  llm: JSON.stringify(llmPayload()) !== baseline.llm,
  vault: JSON.stringify(vaultPayload()) !== baseline.vault,
  inbox: JSON.stringify(inboxPayload()) !== baseline.inbox,
  runtime: JSON.stringify(runtimePayload()) !== baseline.runtime,
}))

const apiKeyDescription = computed(() =>
  settings.value?.llm.apiKeySet
    ? 'A key is stored. Kernl never sends it back, so leave this blank to keep it.'
    : 'Provider credential. Stored in the config file, never returned by the API.',
)
const apiKeyPlaceholder = computed(() =>
  llm.clearKey ? 'Will be removed on save' : settings.value?.llm.apiKeySet ? '•••••••• stored' : 'sk-…',
)
const readOnlyBlurb = computed(() =>
  activeTab.value === 'prompts'
    ? 'Prompt text lives in Go today. Editing it needs a prompt store, not a config field, so these are shown for reference.'
    : 'Agents and pools are nested structures. They stay in the config file until they get an editor of their own.',
)

watch(
  () => route.query.tab,
  (tab) => {
    activeTab.value = readTab(tab)
  },
)

onMounted(() => {
  load()
  loadDAIdentity()
})

function readTab(raw: unknown): TabID {
  const value = Array.isArray(raw) ? raw[0] : raw
  return TABS.some((tab) => tab.id === value) ? (value as TabID) : 'assistant'
}

function setActiveTab(tab: TabID) {
  activeTab.value = tab
  router.replace({ query: tab === 'assistant' ? {} : { tab } })
}

function isPending(key: string) {
  return restartPending.value.includes(key)
}

function llmPayload() {
  const payload: Record<string, unknown> = {
    provider: llm.provider,
    model: llm.model.trim(),
    endpoint: llm.endpoint.trim(),
  }
  // Omitting apiKey means "keep what is stored"; an empty string means "remove it".
  if (llm.clearKey) payload.apiKey = ''
  else if (llm.apiKey) payload.apiKey = llm.apiKey
  return payload
}

function vaultPayload() {
  return {
    root: vault.root.trim(),
    coalesceWindowMs: toInt(vault.coalesceWindowMs),
    moveWindowMs: toInt(vault.moveWindowMs),
    rescanIntervalSec: toInt(vault.rescanIntervalSec),
  }
}

function inboxPayload() {
  return { autoPrep: inbox.autoPrep, daSubdir: inbox.daSubdir.trim() }
}

function runtimePayload() {
  return {
    serverPort: toInt(runtime.serverPort),
    worktreeRoot: runtime.worktreeRoot.trim(),
    maxConcurrentBeads: toInt(runtime.maxConcurrentBeads),
    runStatePath: runtime.runStatePath.trim(),
    stageRetryAttempts: toInt(runtime.stageRetryAttempts),
    sweepIntervalSec: toInt(runtime.sweepIntervalSec),
    prStaleWarnDays: toInt(runtime.prStaleWarnDays),
    sweepFailureLimit: toInt(runtime.sweepFailureLimit),
    sweepBackoffMinutes: runtime.sweepBackoffMinutes
      .split(',')
      .map((part) => toInt(part))
      .filter((minutes) => minutes > 0),
  }
}

function toInt(raw: string) {
  const parsed = Number.parseInt(String(raw).trim(), 10)
  return Number.isNaN(parsed) ? 0 : parsed
}

const payloadFor: Record<ConfigSection, () => unknown> = {
  llm: llmPayload,
  vault: vaultPayload,
  inbox: inboxPayload,
  runtime: runtimePayload,
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const response = await fetch('/api/settings')
    if (!response.ok) throw new Error(await errorText(response))
    hydrate(await response.json())
  } catch (error) {
    loadError.value = message(error)
  } finally {
    loading.value = false
  }
}

function hydrate(data: SettingsResponse) {
  settings.value = data

  llm.provider = data.llm.provider
  llm.model = data.llm.model
  llm.endpoint = data.llm.endpoint
  llm.apiKey = ''
  llm.clearKey = false

  vault.root = data.vault.root
  vault.coalesceWindowMs = String(data.vault.coalesceWindowMs)
  vault.moveWindowMs = String(data.vault.moveWindowMs)
  vault.rescanIntervalSec = String(data.vault.rescanIntervalSec)

  inbox.autoPrep = data.inbox.autoPrep
  inbox.daSubdir = data.inbox.daSubdir

  runtime.serverPort = String(data.runtime.serverPort)
  runtime.worktreeRoot = data.runtime.worktreeRoot
  runtime.maxConcurrentBeads = String(data.runtime.maxConcurrentBeads)
  runtime.runStatePath = data.runtime.runStatePath
  runtime.stageRetryAttempts = String(data.runtime.stageRetryAttempts)
  runtime.sweepIntervalSec = String(data.runtime.sweepIntervalSec)
  runtime.prStaleWarnDays = String(data.runtime.prStaleWarnDays)
  runtime.sweepFailureLimit = String(data.runtime.sweepFailureLimit)
  runtime.sweepBackoffMinutes = (data.runtime.sweepBackoffMinutes || []).join(', ')

  rebaseline()
}

function rebaseline() {
  for (const section of Object.keys(payloadFor) as ConfigSection[]) {
    baseline[section] = JSON.stringify(payloadFor[section]())
  }
}

async function save(section: ConfigSection) {
  saving.value = section
  status[section] = { ...IDLE }
  try {
    const response = await fetch(`/api/settings/${section}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payloadFor[section]()),
    })
    if (!response.ok) throw new Error(await errorText(response))

    hydrate(await response.json())
    status[section] = { text: 'Saved to the config file.', tone: 'text-status-passed' }
  } catch (error) {
    status[section] = { text: message(error), tone: 'text-status-failed-text' }
  } finally {
    saving.value = ''
  }
}

async function loadDAIdentity() {
  daLoading.value = true
  daError.value = ''
  try {
    const response = await fetch('/api/da/identity')
    if (!response.ok) throw new Error(await errorText(response))
    const data = await response.json()
    // The DA endpoint still answers with Go field names rather than the camelCase
    // the rest of the REST surface uses, so accept both shapes.
    da.displayName = data.display_name ?? data.DisplayName ?? ''
    da.systemPrompt = data.system_prompt ?? data.SystemPrompt ?? ''
    Object.assign(daSaved, da)
  } catch (error) {
    daError.value = message(error)
  } finally {
    daLoading.value = false
  }
}

async function saveDAIdentity() {
  daSaving.value = true
  daStatus.value = { ...IDLE }
  try {
    const response = await fetch('/api/da/identity', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ display_name: da.displayName, system_prompt: da.systemPrompt }),
    })
    if (!response.ok) throw new Error(await errorText(response))

    Object.assign(daSaved, da)
    daStatus.value = { text: 'Saved.', tone: 'text-status-passed' }
  } catch (error) {
    daStatus.value = { text: message(error), tone: 'text-status-failed-text' }
  } finally {
    daSaving.value = false
  }
}

// The API answers errors as {"error": "..."}; fall back to raw text so a proxy or
// crash page still surfaces something the user can act on.
async function errorText(response: Response) {
  const raw = await response.text()
  try {
    const parsed = JSON.parse(raw)
    return parsed.error || raw
  } catch {
    return raw || `Request failed with ${response.status}`
  }
}

function message(error: unknown) {
  return error instanceof Error ? error.message : 'Unknown error'
}
</script>
