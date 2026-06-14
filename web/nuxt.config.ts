import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  css: ['~/assets/css/tailwind.css'],
  vite: {
    plugins: [
      tailwindcss(),
    ],
  },
  app: {
    head: {
      title: 'Kernl',
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1.0' },
      ],
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: 'data:image/svg+xml,%3Csvg%20xmlns=%22http://www.w3.org/2000/svg%22%20viewBox=%220%200%2024%2024%22%3E%3Crect%20width=%2224%22%20height=%2224%22%20rx=%225%22%20fill=%22%23151821%22/%3E%3Cpath%20d=%22M6%208l4%204-4%204M12.5%2016h5%22%20fill=%22none%22%20stroke=%22%237B8FE0%22%20stroke-width=%222%22%20stroke-linecap=%22round%22%20stroke-linejoin=%22round%22/%3E%3C/svg%3E' },
        { rel: 'stylesheet', href: 'https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@300;400;500;600;700&family=IBM+Plex+Mono:wght@400;450;500&family=Material+Symbols+Outlined:wght,FILL@100..700,0..1&display=swap' },
      ],
    },
  },
  devtools: { enabled: false },
  compatibilityDate: '2026-05-23',
});
