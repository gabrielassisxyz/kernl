<template>
  <div class="settings-page">
    <header class="settings-header">
      <div>
        <h1 class="font-headline text-display text-text-primary">Settings</h1>
        <p class="mt-tight font-body text-body text-text-muted">
          Configure the local assistant, editor, runtime, and orchestration surfaces.
        </p>
      </div>
      <div class="header-actions">
        <UiButton variant="secondary" size="sm" icon="refresh" :loading="inventoryLoading" @click="loadAll">
          Refresh
        </UiButton>
      </div>
    </header>

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
      v-if="inventoryError"
      bordered
      title="Settings inventory unavailable"
      message="Kernl could not load the settings inventory from the active server."
      :detail="inventoryError"
      @retry="loadInventory"
    />

    <div v-else-if="inventoryLoading" class="settings-loading">
      <UiSkeleton v-for="index in 5" :key="index" classes="h-[76px]" />
    </div>

    <main v-else class="settings-content">
      <section v-if="activeTab === 'general'" class="settings-stack">
        <section class="settings-panel">
          <div class="panel-heading">
            <div>
              <h2>Configuration status</h2>
              <p>What is configured now, where it lives, and what needs a dedicated API later.</p>
            </div>
          </div>
          <div class="summary-list">
            <div v-for="item in inventory?.summary || []" :key="item.id" class="summary-row">
              <div>
                <p class="row-title">{{ item.label }}</p>
                <p class="row-description">{{ item.description }}</p>
              </div>
              <div class="row-meta">
                <span class="setting-value">{{ item.value }}</span>
                <span class="meta-chip" :class="statusChipClass(item.status)">{{ statusLabel(item.status) }}</span>
              </div>
            </div>
          </div>
        </section>

        <section class="settings-panel">
          <div class="panel-heading">
            <div>
              <h2>Editable now</h2>
              <p>The first slice keeps writes behind domain-specific APIs or local preference storage.</p>
            </div>
          </div>
          <div class="summary-list">
            <SettingsInventoryRow v-for="item in editableItems" :key="item.key" :item="item" />
          </div>
        </section>
      </section>

      <section v-else-if="activeTab === 'da'" class="settings-stack">
        <section class="settings-panel">
          <div class="panel-heading">
            <div>
              <h2>DA identity</h2>
              <p>Stored in the graph and applied to the local assistant.</p>
            </div>
            <div class="panel-meta">
              <span class="meta-chip meta-chip--graph">graph</span>
              <span class="meta-chip meta-chip--editable">editable</span>
            </div>
          </div>

          <UiErrorState
            v-if="daError"
            bordered
            title="DA identity unavailable"
            message="Kernl could not load the assistant identity."
            :detail="daError"
            @retry="loadDAIdentity"
          />

          <form v-else class="form-stack" @submit.prevent="saveDAIdentity">
            <div class="setting-row setting-row--field">
              <div class="setting-copy">
                <label class="row-title" for="da-display-name">Display name</label>
                <p class="row-description">The human-facing name shown for the assistant.</p>
                <p class="row-key">da.displayName</p>
              </div>
              <UiInput id="da-display-name" v-model="daDisplayName" :disabled="daLoading || daSaving" />
            </div>

            <div class="setting-row setting-row--field setting-row--textarea">
              <div class="setting-copy">
                <label class="row-title" for="da-system-prompt">System prompt</label>
                <p class="row-description">The base instruction used by the local assistant.</p>
                <p class="row-key">da.systemPrompt</p>
              </div>
              <UiTextarea
                id="da-system-prompt"
                v-model="daSystemPrompt"
                rows="12"
                :disabled="daLoading || daSaving"
                classes="w-full rounded border border-border-hairline bg-bg-base px-component py-base font-mono-data text-mono-data text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50 resize-y"
              />
            </div>

            <div class="panel-footer">
              <span class="save-status" :class="daStatusClass">{{ daStatusText }}</span>
              <UiButton type="submit" variant="accent" icon="save" :loading="daSaving" :disabled="!daDirty || daLoading">
                Save DA identity
              </UiButton>
            </div>
          </form>
        </section>
      </section>

      <section v-else-if="activeTab === 'editor'" class="settings-stack">
        <section class="settings-panel">
          <div class="panel-heading">
            <div>
              <h2>Editor preferences</h2>
              <p>Browser-local note editor preferences. Changes apply immediately.</p>
            </div>
            <div class="panel-meta">
              <span class="meta-chip meta-chip--local">localStorage</span>
              <span class="meta-chip meta-chip--editable">editable</span>
            </div>
          </div>

          <div class="form-stack">
            <div class="setting-row setting-row--field">
              <div class="setting-copy">
                <label class="row-title" for="editor-view-mode">View mode</label>
                <p class="row-description">Choose how markdown notes open by default.</p>
                <p class="row-key">editor.viewMode</p>
              </div>
              <UiSelect id="editor-view-mode" v-model="settings.viewMode">
                <option value="source">Source</option>
                <option value="live">Live preview</option>
                <option value="reading">Reading</option>
              </UiSelect>
            </div>

            <SettingsSwitchRow
              label="Line numbers"
              description="Show line numbers in source and live editor modes."
              setting-key="editor.lineNumbers"
              :checked="settings.lineNumbers"
              @toggle="settings.lineNumbers = !settings.lineNumbers"
            />
            <SettingsSwitchRow
              label="Typewriter mode"
              description="Keep the active line near the visual center."
              setting-key="editor.typewriter"
              :checked="settings.typewriter"
              @toggle="settings.typewriter = !settings.typewriter"
            />
            <SettingsSwitchRow
              label="Show note ID"
              description="Expose the graph node ID in note properties."
              setting-key="editor.showId"
              :checked="settings.showId"
              @toggle="settings.showId = !settings.showId"
            />

            <div class="setting-row setting-row--field">
              <div class="setting-copy">
                <label class="row-title" for="editor-font">Editor font</label>
                <p class="row-description">Tune the reading and editing texture of notes.</p>
                <p class="row-key">editor.font</p>
              </div>
              <UiSelect id="editor-font" v-model="settings.font">
                <option value="sans">Sans</option>
                <option value="serif">Serif</option>
                <option value="mono">Mono</option>
              </UiSelect>
            </div>

            <SettingsStepperRow
              label="Font size"
              description="Local editor text size."
              setting-key="editor.fontSize"
              :value="settings.fontSize"
              suffix="px"
              :min="FONT_SIZE_MIN"
              :max="FONT_SIZE_MAX"
              @decrease="setFontSize(settings.fontSize - 1)"
              @increase="setFontSize(settings.fontSize + 1)"
            />
            <SettingsStepperRow
              label="Heading scale"
              description="Markdown heading scale in the note editor."
              setting-key="editor.headingScale"
              :value="Math.round(settings.headingScale * 100)"
              suffix="%"
              :min="Math.round(HEADING_SCALE_MIN * 100)"
              :max="Math.round(HEADING_SCALE_MAX * 100)"
              @decrease="setHeadingScale(settings.headingScale - 0.05)"
              @increase="setHeadingScale(settings.headingScale + 0.05)"
            />
          </div>

          <div class="panel-footer">
            <span class="save-status text-text-muted">Saved automatically on this device.</span>
            <UiButton variant="secondary" icon="refresh" @click="reset">Reset editor preferences</UiButton>
          </div>
        </section>
      </section>

      <section v-else class="settings-stack">
        <section class="settings-panel">
          <div class="panel-heading">
            <div>
              <h2>{{ activeSection?.title || activeTabLabel }}</h2>
              <p>{{ activeSection?.description || 'Configuration values loaded from the active process.' }}</p>
            </div>
            <div class="panel-meta">
              <span class="meta-chip">read-only</span>
              <span class="meta-chip meta-chip--gate">requires API</span>
            </div>
          </div>

          <div class="form-stack">
            <SettingsInventoryRow v-for="item in activeSectionItems" :key="item.key" :item="item" />
            <div v-if="activeTab === 'advanced'" class="setting-row">
              <div class="setting-copy">
                <p class="row-title">Raw YAML editing</p>
                <p class="row-description">
                  Direct YAML editing is intentionally not exposed. Future writes should go through domain-specific APIs with validation and restart feedback.
                </p>
                <p class="row-key">kernl.yaml</p>
              </div>
              <span class="meta-chip meta-chip--gate">requires dedicated API</span>
            </div>
          </div>
        </section>
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
import SettingsInventoryRow from '~/components/settings/SettingsInventoryRow.vue'
import SettingsStepperRow from '~/components/settings/SettingsStepperRow.vue'
import SettingsSwitchRow from '~/components/settings/SettingsSwitchRow.vue'

type TabID = 'general' | 'da' | 'prompts' | 'editor' | 'runtime' | 'agents' | 'llm' | 'vault' | 'advanced'

interface SettingsSummaryItem {
  id: string
  label: string
  value: string
  status: string
  description: string
}

interface SettingItem {
  key: string
  label: string
  description: string
  value: string
  source: string
  status: string
  editPath: string
  editable: boolean
  sensitive: boolean
  restartRequired: boolean
}

interface SettingsSection {
  id: string
  title: string
  description: string
  items: SettingItem[]
}

interface SettingsInventory {
  summary: SettingsSummaryItem[]
  sections: SettingsSection[]
}

interface DAIdentity {
  display_name?: string
  system_prompt?: string
  DisplayName?: string
  SystemPrompt?: string
}

const TABS: { id: TabID; label: string }[] = [
  { id: 'general', label: 'General' },
  { id: 'da', label: 'DA' },
  { id: 'prompts', label: 'Prompts' },
  { id: 'editor', label: 'Editor' },
  { id: 'runtime', label: 'Runtime' },
  { id: 'agents', label: 'Agents' },
  { id: 'llm', label: 'LLM' },
  { id: 'vault', label: 'Vault' },
  { id: 'advanced', label: 'Advanced' },
]

const route = useRoute()
const router = useRouter()
const { settings, setFontSize, setHeadingScale, reset } = useEditorSettings()

const inventory = ref<SettingsInventory | null>(null)
const inventoryLoading = ref(false)
const inventoryError = ref('')
const activeTab = ref<TabID>(readTab(route.query.tab))

const daDisplayName = ref('')
const daSystemPrompt = ref('')
const daInitial = ref({ displayName: '', systemPrompt: '' })
const daLoading = ref(false)
const daSaving = ref(false)
const daError = ref('')
const daStatusText = ref('')
const daStatusClass = ref('text-text-muted')

const activeTabLabel = computed(() => TABS.find((tab) => tab.id === activeTab.value)?.label || 'Settings')
const activeSection = computed(() => inventory.value?.sections.find((section) => section.id === activeTab.value))
const activeSectionItems = computed(() => {
  if (activeTab.value === 'advanced') return []
  return activeSection.value?.items || []
})
const editableItems = computed(() =>
  (inventory.value?.sections || []).flatMap((section) => section.items.filter((item) => item.editable)),
)
const daDirty = computed(
  () => daDisplayName.value !== daInitial.value.displayName || daSystemPrompt.value !== daInitial.value.systemPrompt,
)

watch(
  () => route.query.tab,
  (tab) => {
    activeTab.value = readTab(tab)
  },
)

onMounted(() => {
  loadAll()
})

function readTab(raw: unknown): TabID {
  const value = Array.isArray(raw) ? raw[0] : raw
  return TABS.some((tab) => tab.id === value) ? (value as TabID) : 'general'
}

function setActiveTab(tab: TabID) {
  activeTab.value = tab
  router.replace({ query: tab === 'general' ? {} : { tab } })
  if (tab === 'da' && !daDisplayName.value && !daLoading.value) {
    loadDAIdentity()
  }
}

async function loadAll() {
  await Promise.all([loadInventory(), loadDAIdentity()])
}

async function loadInventory() {
  inventoryLoading.value = true
  inventoryError.value = ''
  try {
    const response = await fetch('/api/settings/inventory')
    if (!response.ok) throw new Error(await response.text())
    inventory.value = await response.json()
  } catch (error) {
    inventoryError.value = error instanceof Error ? error.message : 'Unknown error'
  } finally {
    inventoryLoading.value = false
  }
}

async function loadDAIdentity() {
  daLoading.value = true
  daError.value = ''
  try {
    const response = await fetch('/api/da/identity')
    if (!response.ok) throw new Error(await response.text())
    const data = (await response.json()) as DAIdentity
    daDisplayName.value = data.display_name || data.DisplayName || ''
    daSystemPrompt.value = data.system_prompt || data.SystemPrompt || ''
    daInitial.value = {
      displayName: daDisplayName.value,
      systemPrompt: daSystemPrompt.value,
    }
  } catch (error) {
    daError.value = error instanceof Error ? error.message : 'Unknown error'
  } finally {
    daLoading.value = false
  }
}

async function saveDAIdentity() {
  daSaving.value = true
  daStatusText.value = ''
  try {
    const response = await fetch('/api/da/identity', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        display_name: daDisplayName.value,
        system_prompt: daSystemPrompt.value,
      }),
    })
    if (!response.ok) throw new Error(await response.text())
    daInitial.value = {
      displayName: daDisplayName.value,
      systemPrompt: daSystemPrompt.value,
    }
    daStatusText.value = 'Saved.'
    daStatusClass.value = 'text-status-passed'
  } catch (error) {
    daStatusText.value = error instanceof Error ? error.message : 'Save failed.'
    daStatusClass.value = 'text-status-failed-text'
  } finally {
    daSaving.value = false
  }
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    configured: 'configured',
    missing: 'missing',
    editable: 'editable',
    readOnly: 'read-only',
  }
  return labels[status] || status
}

function statusChipClass(status: string) {
  return {
    'meta-chip--editable': status === 'configured' || status === 'editable',
    'meta-chip--danger': status === 'missing',
  }
}

</script>
