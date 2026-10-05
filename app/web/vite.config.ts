import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development the UI runs on :5173 and proxies the Core API on :8080.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:8080', ws: true },
      '/go': 'http://localhost:8080',
    },
  },
})
