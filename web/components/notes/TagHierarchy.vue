<template>
  <div class="tag-hierarchy p-4 h-full overflow-y-auto bg-[#0F1217] text-[13px]">
    <h3 class="text-[11.5px] uppercase font-semibold tracking-[0.12em] text-[#666D7C] mb-4">Tags</h3>
    <div v-if="loading" class="text-[#9098A7]">Loading...</div>
    <div v-else>
      <div v-for="(node, name) in tree" :key="name" class="mb-2">
        <div class="flex items-center gap-2 cursor-pointer text-[#D6DBE3] hover:bg-[#1D222D] px-2 py-1.5 rounded-[4px]" @click="toggle(name)">
          <span class="text-[#666D7C] font-mono text-[10px] w-3 flex justify-center">{{ expanded[name] ? '▼' : '▶' }}</span>
          <span class="font-medium">#{{ name }}</span>
        </div>
        <div v-if="expanded[name]" class="ml-4 pl-3 border-l border-[#1B2029] mt-1 flex flex-col gap-1">
          <div v-for="file in node.files" :key="file" class="py-1 px-2 text-[#9098A7] hover:text-[#D6DBE3] hover:bg-[#141821] rounded-[4px] cursor-pointer truncate font-mono" @click="$emit('select', file)">
            {{ file }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const tree = ref({})
const expanded = ref({})
const loading = ref(true)

defineEmits(['select'])

const toggle = (name) => {
  expanded.value[name] = !expanded.value[name]
}

onMounted(async () => {
  try {
    const res = await fetch('/api/vault/list')
    if (res.ok) {
      const data = await res.json()
      const builtTree = {}
      
      for (const file of data.files) {
        try {
          const fileRes = await fetch(`/api/vault/file?path=${encodeURIComponent(file)}`)
          if (fileRes.ok) {
            const text = await fileRes.text()
            if (text.startsWith('---\n')) {
              const end = text.indexOf('\n---\n', 4)
              if (end !== -1) {
                const fmText = text.substring(4, end)
                const tagsMatch = fmText.match(/tags:\s*\[(.*?)\]/)
                if (tagsMatch) {
                  const tags = tagsMatch[1].split(',').map(t => t.trim().replace(/['"]/g, ''))
                  tags.forEach(tag => {
                    if (!builtTree[tag]) builtTree[tag] = { files: [] }
                    builtTree[tag].files.push(file)
                  })
                }
              }
            }
          }
        } catch (e) {
          console.error('Error fetching file for tags', e)
        }
      }
      tree.value = builtTree
    }
  } catch (e) {
    console.error('Error fetching tags', e)
  } finally {
    loading.value = false
  }
})
</script>
