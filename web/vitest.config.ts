import { defineConfig } from 'vitest/config';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url)).replace(/\/$/, '');

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '~': root,
      '@': root,
    },
  },
  test: {
    environment: 'happy-dom',
    globals: true,
  },
});
