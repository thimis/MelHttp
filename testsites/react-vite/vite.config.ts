import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: { outDir: 'dist' },
  preview: { port: 4173, strictPort: true },
})
