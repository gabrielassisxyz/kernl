<template>
  <div class="flex flex-col h-full min-h-0">
    <header class="px-section pt-margin pb-component border-b border-border-hairline shrink-0">
      <h1 class="font-headline text-display text-text-primary font-medium tracking-tight">Tags</h1>
      <p class="font-body text-body text-text-muted mt-tight">{{ summary }}</p>
    </header>

    <div class="flex flex-1 min-h-0">
      <!-- Subjects -->
      <aside class="w-[260px] shrink-0 border-r border-border-hairline overflow-y-auto p-base">
        <TagTree
          :tree="tree"
          :loading="loading"
          :error="error"
          :selected="selectedTag"
          empty-text="No tags yet. Tag a note, a task or a project to start a subject."
          @select="selectTag"
        />
      </aside>

      <!-- Everything under the selected subject -->
      <section class="flex-1 min-w-0 flex flex-col">
        <UiEmptyState
          v-if="!selectedTag"
          fill
          icon="tag"
          title="Pick a subject."
          body="A tag gathers everything about one subject — notes, tasks, projects, bookmarks — regardless of its type."
        />

        <template v-else>
          <div class="px-section py-component border-b border-border-hairline flex items-center gap-component flex-wrap">
            <h2 class="font-headline text-text-primary flex items-center gap-tight">
              <span class="material-symbols-outlined !text-body text-text-faint" aria-hidden="true">tag</span>
              {{ selectedTag }}
            </h2>
            <div class="flex items-center gap-tight">
              <button
                v-for="chip in typeChips"
                :key="chip.type"
                type="button"
                class="px-component py-tight rounded border font-mono-data text-mono-data transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
                :class="typeFilter === chip.type
                  ? 'border-primary/70 text-text-primary bg-surface-hover'
                  : 'border-border-hairline text-text-muted hover:text-text-primary hover:bg-surface'"
                @click="typeFilter = chip.type"
              >
                {{ chip.label }} {{ chip.count }}
              </button>
            </div>
          </div>

          <div class="flex-1 min-h-0 overflow-y-auto px-base py-base">
            <TagNodeList :tag="selectedTag" :type="typeFilter" @open="open" />
          </div>
        </template>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import TagNodeList from '~/components/tags/TagNodeList.vue'
import TagTree from '~/components/tags/TagTree.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import { useTags, type TagNode, type TaggedNode } from '~/composables/useTags'
import { labelForType } from '~/utils/nodeTypes'

// The browse surface for the one axis that crosses node types: pick a subject on
// the left, see everything under it — of every type — on the right.
const { tree, loading, error, loadTree } = useTags()

const route = useRoute()
const selectedTag = ref(typeof route.query.tag === 'string' ? route.query.tag : '')
const typeFilter = ref('')

onMounted(loadTree)

const summary = computed(() => {
  if (!tree.value.length) return 'Nothing tagged yet.'
  const subjects = tree.value.length === 1 ? '1 subject' : `${tree.value.length} subjects`
  return `${subjects} across your graph.`
})

// The counts on the tree are subtree-inclusive, so the chips for the selected tag
// come straight from it — no second request to learn what types are under it.
function findTag(nodes: TagNode[], name: string): TagNode | null {
  for (const node of nodes) {
    if (node.name === name) return node
    const hit = findTag(node.children ?? [], name)
    if (hit) return hit
  }
  return null
}

const typeChips = computed(() => {
  const node = findTag(tree.value, selectedTag.value)
  if (!node) return []
  const chips = [{ type: '', label: 'All', count: node.count }]
  for (const [type, count] of Object.entries(node.byType ?? {})) {
    if (count > 0) chips.push({ type, label: labelForType(type), count })
  }
  return chips
})

function selectTag(name: string) {
  selectedTag.value = name
  typeFilter.value = ''
  // Keep the URL shareable: a tag page is a query, and this is the query.
  navigateTo({ path: '/tags', query: { tag: name } })
}

function open(node: TaggedNode) {
  if (node.type === 'note') {
    navigateTo({ path: '/notes', query: { path: node.path } })
    return
  }
  if (node.type === 'task') {
    navigateTo({ path: '/tasks', query: { task: node.id } })
    return
  }
  // A project's own surface is its task list — the same drill-in Projects uses.
  navigateTo({ path: '/tasks', query: { project: node.id } })
}
</script>
