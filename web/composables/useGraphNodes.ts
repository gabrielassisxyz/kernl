import { ref } from 'vue';

export interface GraphNode {
  id: string;
  title: string;
  type: string;
}

export function useGraphNodes() {
  const nodes = ref<GraphNode[]>([]);
  const selectedNodeId = ref<string>('');

  async function loadNodes(): Promise<void> {
    const res = await fetch('/api/nodes');
    nodes.value = await res.json();
  }

  function setSelected(id: string): void {
    selectedNodeId.value = id;
  }

  return { nodes, selectedNodeId, loadNodes, setSelected };
}
