<template>
  <div class="p-8 w-full h-full text-[#D6DBE3] bg-[#0F1217]">
    <header class="mb-8 border-b border-[#1B2029] pb-4">
      <h1 class="text-[22px] font-medium tracking-tight mb-2">Kernl - Home</h1>
      <p class="text-[13px] text-[#9098A7]">Orchestrate your autonomous agents.</p>
    </header>
    
    <div class="grid grid-cols-2 gap-6">
      <div class="bg-[#181C26] border border-[#242935] rounded-md p-4 transition-colors hover:bg-[#1D222D] hover:border-[#6B7BB0]">
        <div class="flex items-center gap-2 mb-4">
          <div class="w-1.5 h-1.5 rounded-full bg-[#8089A0]"></div>
          <h2 class="text-[11.5px] font-medium text-[#D6DBE3] uppercase tracking-[0.12em]">In Flight</h2>
        </div>
        <div class="text-[13px] text-[#9098A7]">No active beads.</div>
      </div>
      
      <div class="bg-[#181C26] border border-[#242935] rounded-md p-4 transition-colors hover:bg-[#1D222D] hover:border-[#6B7BB0]">
        <div class="flex items-center gap-2 mb-4">
          <div class="w-1.5 h-1.5 rounded-full bg-[#C99A4A]"></div>
          <h2 class="text-[11.5px] font-medium text-[#D6DBE3] uppercase tracking-[0.12em]">Needs Attention</h2>
        </div>
        <div class="text-[13px] text-[#9098A7]">All clear.</div>
      </div>
    </div>
  </div>
</template>

<script setup>
</script>
