import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    port: 5273,
    proxy: {
      '/api': { target: 'http://127.0.0.1:1321', changeOrigin: true },
    },
  },
})
