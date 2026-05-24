import { defineNuxtConfig } from 'nuxt/config';

export default defineNuxtConfig({
  ssr: false,
  app: {
    baseURL: '/chat/',
    head: {
      title: 'Kernl Chat',
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1.0' },
      ],
    },
  },
  devtools: { enabled: false },
  compatibilityDate: '2026-05-23',
});
