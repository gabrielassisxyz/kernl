<template>
  <div class="flex items-center gap-2">
    <label class="font-body text-body text-text-muted whitespace-nowrap">Scope:</label>
    <UiSelect
      v-model="selected"
      classes="h-8 min-w-[200px] rounded border border-border-hairline bg-surface-container-low px-base font-mono-data text-mono-data text-text-primary outline-none transition-colors focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50"
      @change="onChange"
    >
      <option value="">All notes</option>
      <option v-for="node in nodes" :key="node.id" :value="node.id">
        {{ node.title }} ({{ node.type }})
      </option>
    </UiSelect>
  </div>
</template>

<script setup lang="ts">
import UiSelect from '~/components/ui/UiSelect.vue'

const { nodes, selectedNodeId, loadNodes, setSelected } = useGraphNodes();

const selected = ref('');

onMounted(() => {
  loadNodes();
});

function onChange() {
  setSelected(selected.value);
}
</script>
