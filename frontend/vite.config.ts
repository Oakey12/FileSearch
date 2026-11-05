import { defineConfig } from 'vite'

// Две страницы: окно поиска (index.html) и окно рыжего кота (cat.html, процесс с ключом --cat).
export default defineConfig({
  build: {
    rollupOptions: {
      input: {
        main: 'index.html',
        cat: 'cat.html',
      },
    },
  },
})
