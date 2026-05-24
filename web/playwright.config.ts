import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  webServer: {
    command: 'nuxt preview --port 13000',
    port: 13000,
    timeout: 120000,
  },
  use: {
    baseURL: 'http://localhost:13000/chat',
  },
});
