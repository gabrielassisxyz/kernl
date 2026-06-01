<template>
  <div class="px-margin pt-margin">
    <!-- Header -->
    <header class="mb-break">
      <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Good morning. Three things landed overnight.</h1>
    </header>
    
    <!-- Bento Grid Layout -->
    <div class="grid grid-cols-2 gap-section pb-margin">
      
      <!-- Pane 1: Today's Tasks -->
      <section class="bg-surface border border-border-hairline rounded-lg h-[340px] flex flex-col">
        <div class="px-component py-base border-b border-border-hairline flex items-center justify-between">
          <h2 class="font-label-caps text-label-caps text-text-muted">TODAY'S TASKS</h2>
          <span class="material-symbols-outlined text-text-faint text-sm">more_vert</span>
        </div>
        <div class="flex-grow overflow-y-auto p-base flex flex-col gap-tight">
          <!-- Task Item (Gate) -->
          <div class="flex items-center bg-surface-container-low border-l-2 border-status-gate py-2 px-base gap-component">
            <span class="font-mono-data text-mono-data text-status-gate">kr-842</span>
            <span class="font-body text-body text-text-primary">Review schema changes for orchestrator sync</span>
          </div>
          <!-- Standard Items -->
          <div class="flex items-center hover:bg-surface-hover py-2 px-base gap-component transition-colors group">
            <span class="font-mono-data text-mono-data text-text-faint group-hover:text-text-muted">kr-910</span>
            <span class="font-body text-body text-text-primary">Push daily metrics to vault</span>
          </div>
          <div class="flex items-center hover:bg-surface-hover py-2 px-base gap-component transition-colors group">
            <span class="font-mono-data text-mono-data text-text-faint group-hover:text-text-muted">kr-112</span>
            <span class="font-body text-body text-text-primary">Resolve circular dependencies in @kernl/core</span>
          </div>
          <div class="flex items-center hover:bg-surface-hover py-2 px-base gap-component transition-colors group">
            <span class="font-mono-data text-mono-data text-text-faint group-hover:text-text-muted">kr-334</span>
            <span class="font-body text-body text-text-primary">Update local environment variables for v1.0.5</span>
          </div>
          <div class="flex items-center hover:bg-surface-hover py-2 px-base gap-component transition-colors group">
            <span class="font-mono-data text-mono-data text-text-faint group-hover:text-text-muted">kr-029</span>
            <span class="font-body text-body text-text-primary">Backup knowledge graph to cold storage</span>
          </div>
        </div>
      </section>

      <!-- Pane 2: Active Projects -->
      <section class="bg-surface border border-border-hairline rounded-lg h-[340px] flex flex-col">
        <div class="px-component py-base border-b border-border-hairline flex items-center justify-between">
          <h2 class="font-label-caps text-label-caps text-text-muted">ACTIVE PROJECTS</h2>
          <span class="material-symbols-outlined text-text-faint text-sm">filter_list</span>
        </div>
        <div class="flex-grow p-component flex flex-col gap-base">
          <!-- Project Row -->
          <div class="flex items-center justify-between py-base border-b border-border-hairline last:border-0">
            <div class="flex items-center gap-component">
              <span class="w-1.5 h-1.5 rounded-full bg-status-running"></span>
              <span class="font-body text-body text-text-primary">Orchestrator V2</span>
            </div>
            <span class="font-label-caps text-[10px] text-text-muted bg-surface-container py-0.5 px-2 rounded">IN-FLIGHT</span>
          </div>
          <!-- Project Row -->
          <div class="flex items-center justify-between py-base border-b border-border-hairline last:border-0">
            <div class="flex items-center gap-component">
              <span class="w-1.5 h-1.5 rounded-full bg-status-gate animate-pulse"></span>
              <div class="flex flex-col">
                <span class="font-body text-body text-text-primary">Vault Security Audit</span>
                <span class="text-[10px] text-status-gate">Attention Required</span>
              </div>
            </div>
            <span class="font-label-caps text-[10px] text-status-gate bg-surface-container py-0.5 px-2 rounded">REVIEW</span>
          </div>
          <!-- Project Row -->
          <div class="flex items-center justify-between py-base border-b border-border-hairline last:border-0">
            <div class="flex items-center gap-component">
              <span class="w-1.5 h-1.5 rounded-full bg-status-running opacity-40"></span>
              <span class="font-body text-body text-text-primary">Documentation Refresh</span>
            </div>
            <span class="font-label-caps text-[10px] text-text-muted bg-surface-container py-0.5 px-2 rounded">QUEUED</span>
          </div>
        </div>
      </section>

      <!-- Pane 3: What Shipped Overnight -->
      <section class="bg-surface border border-border-hairline rounded-lg h-[340px] flex flex-col">
        <div class="px-component py-base border-b border-border-hairline">
          <h2 class="font-label-caps text-label-caps text-text-muted">WHAT SHIPPED OVERNIGHT</h2>
        </div>
        <div class="flex-grow p-component overflow-y-auto">
          <div class="flex flex-col gap-base border-l border-border-hairline ml-base pl-base">
            <!-- Ledger Entry -->
            <div class="relative">
              <div class="absolute -left-[21px] top-1 w-2.5 h-2.5 bg-status-passed rounded-full border-2 border-surface"></div>
              <span class="font-mono-data text-mono-data text-text-muted block">04:22 AM</span>
              <span class="font-mono-data text-mono-data text-status-passed">[kr-112]</span>
              <span class="font-body text-body text-text-primary">Closed by Agent Echo</span>
            </div>
            <!-- Ledger Entry -->
            <div class="relative">
              <div class="absolute -left-[21px] top-1 w-2.5 h-2.5 bg-status-passed rounded-full border-2 border-surface"></div>
              <span class="font-mono-data text-mono-data text-text-muted block">02:15 AM</span>
              <span class="font-mono-data text-mono-data text-status-passed">[kr-904]</span>
              <span class="font-body text-body text-text-primary">Closed by Agent Delta</span>
            </div>
            <!-- Ledger Entry -->
            <div class="relative">
              <div class="absolute -left-[21px] top-1 w-2.5 h-2.5 bg-status-passed rounded-full border-2 border-surface"></div>
              <span class="font-mono-data text-mono-data text-text-muted block">11:58 PM</span>
              <span class="font-mono-data text-mono-data text-status-passed">[kr-442]</span>
              <span class="font-body text-body text-text-primary">Closed by Agent Alpha</span>
            </div>
          </div>
        </div>
      </section>

      <!-- Pane 4: Quick Capture -->
      <section class="bg-surface border border-border-hairline rounded-lg h-[340px] flex flex-col">
        <div class="px-component py-base border-b border-border-hairline">
          <h2 class="font-label-caps text-label-caps text-text-muted">QUICK CAPTURE</h2>
        </div>
        <div class="flex-grow p-component flex flex-col justify-end gap-component">
          <div class="flex-grow flex items-center justify-center text-center opacity-30">
            <span class="material-symbols-outlined text-[48px]">lightbulb</span>
          </div>
          <div class="w-full">
            <label class="block font-mono-data text-[10px] text-text-faint mb-tight uppercase tracking-widest">Entry Stream</label>
            <div class="bg-surface-container-low border border-border-hairline rounded px-base py-component min-h-[44px] flex items-center focus-within:border-primary transition-colors">
              <input v-model="captureInput" @keyup.enter="submitCapture" class="w-full bg-transparent border-none p-0 focus:ring-0 font-body text-body text-text-primary placeholder:text-text-faint outline-none custom-caret" :placeholder="capturePlaceholder" type="text">
            </div>
          </div>
        </div>
      </section>
      
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const captureInput = ref('')
const capturePlaceholder = ref('capture a thought...')

const submitCapture = () => {
  if (captureInput.value.trim() !== '') {
    captureInput.value = ''
    capturePlaceholder.value = 'Thought logged...'
    setTimeout(() => {
      capturePlaceholder.value = 'capture a thought...'
    }, 2000)
  }
}
</script>
