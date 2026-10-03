import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  build: { outDir: 'dist' },
  preview: { port: 4173, strictPort: true },
})
