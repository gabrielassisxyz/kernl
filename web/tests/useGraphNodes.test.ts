import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useGraphNodes } from '../composables/useGraphNodes';

const mockFetch = vi.fn();
global.fetch = mockFetch;

describe('useGraphNodes', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('loads nodes from API', async () => {
    const mockNodes = [
      { id: 'n1', title: 'Note 1', type: 'note' },
      { id: 'n2', title: 'Note 2', type: 'folder' },
    ];
    mockFetch.mockResolvedValueOnce({
      json: () => Promise.resolve(mockNodes),
    });

    const { nodes, loadNodes } = useGraphNodes();
    await loadNodes();

    expect(mockFetch).toHaveBeenCalledWith('/api/nodes');
    expect(nodes.value).toEqual(mockNodes);
  });

  it('sets selected node ID', () => {
    const { selectedNodeId, setSelected } = useGraphNodes();
    expect(selectedNodeId.value).toBe('');

    setSelected('n1');
    expect(selectedNodeId.value).toBe('n1');

    setSelected('');
    expect(selectedNodeId.value).toBe('');
  });
});
