import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  css: [
    // IBM Plex Sans — weights actually used: 400 (body), 500 (medium), 600 (semibold/headline), 700 (bold)
    '@fontsource/ibm-plex-sans/400.css',
    '@fontsource/ibm-plex-sans/500.css',
    '@fontsource/ibm-plex-sans/600.css',
    '@fontsource/ibm-plex-sans/700.css',
    // IBM Plex Mono — weights actually used: 400, 500 (mono-data), 600
    '@fontsource/ibm-plex-mono/400.css',
    '@fontsource/ibm-plex-mono/500.css',
    '@fontsource/ibm-plex-mono/600.css',
    // Material Symbols Outlined — self-hosted subset (46 icons); @font-face + class rule
    '~/assets/css/fonts.css',
    '~/assets/css/tailwind.css',
  ],
  vite: {
    plugins: [
      tailwindcss(),
    ],
    build: {
      // The notes editor pulls in CodeMirror + the lezer markdown grammar — a
      // single cohesive ~620 KB chunk. It's lazy-loaded (the editor mounts only
      // when a note is opened, via defineAsyncComponent in pages/notes.vue), so
      // it never weighs on first paint of any route. Splitting it further would
      // add round-trips without real benefit, so we raise the warning ceiling
      // rather than suppress it outright.
      chunkSizeWarningLimit: 700,
    },
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
      ],
    },
  },
  devtools: { enabled: false },
  compatibilityDate: '2026-05-23',
});
