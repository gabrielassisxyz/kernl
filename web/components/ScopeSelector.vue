<template>
  <div class="flex items-center gap-2">
    <label class="text-sm text-[#94a3b8] whitespace-nowrap">Scope:</label>
    <select
      v-model="selected"
      class="bg-[#1e293b] border border-[#334155] text-[#e2e8f0] p-1.5 rounded font-mono text-sm min-w-[200px]"
      @change="onChange"
    >
      <option value="">All notes</option>
      <option v-for="node in nodes" :key="node.id" :value="node.id">
        {{ node.title }} ({{ node.type }})
      </option>
    </select>
  </div>
</template>

<script setup lang="ts">
const { nodes, selectedNodeId, loadNodes, setSelected } = useGraphNodes();

const selected = ref('');

onMounted(() => {
  loadNodes();
});

function onChange() {
  setSelected(selected.value);
}
</script>
