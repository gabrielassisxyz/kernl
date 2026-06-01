<template>
  <div class="absolute top-4 right-4 w-80 bg-[#181C26] border border-[#242935] rounded-md shadow-xl z-40 overflow-hidden flex flex-col max-h-[70vh]">
    <div class="bg-[#141821] border-b border-[#1B2029] px-4 py-2 text-[11.5px] uppercase font-semibold tracking-[0.12em] text-[#9098A7] flex justify-between items-center">
      <span>DA Suggestion</span>
      <span class="bg-[#1D222D] text-[#6B7BB0] font-mono px-1.5 py-0.5 rounded-sm">{{ hunks.length }}</span>
    </div>
    <div class="overflow-y-auto">
      <div v-for="hunk in hunks" :key="hunk.id" class="p-4 border-b border-[#1B2029] last:border-0 relative">
        <div class="absolute -left-1 top-4 bottom-4 w-1 bg-[#6B7BB0] rounded-r-sm"></div>
        <div class="text-[13px] text-[#D6DBE3] bg-[#1D222D] p-3 rounded font-mono mb-3 overflow-x-auto whitespace-pre">
          <span class="text-[#6D9A78]">+ {{ hunk.content }}</span>
        </div>
        <div class="flex gap-2">
          <button @click="$emit('accept', hunk)" class="flex-1 bg-[#1D222D] border border-[#242935] text-[#D6DBE3] hover:bg-[#6D9A78] hover:text-[#F0F4F8] hover:border-transparent px-3 py-1.5 rounded-[4px] text-sm transition-colors">Accept</button>
          <button @click="$emit('reject', hunk)" class="flex-1 bg-transparent border border-[#242935] text-[#9098A7] hover:bg-[#1D222D] px-3 py-1.5 rounded-[4px] text-sm transition-colors">Reject</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  hunks: Array
})
</script>
