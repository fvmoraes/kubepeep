import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

import { precompressAssets, preloadShellFonts } from './build/assetPlugins'

export default defineConfig({
  plugins: [react(), tailwindcss(), precompressAssets(), preloadShellFonts()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    sourcemap: false,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    // Build plugins ship no runtime code but decide the asset contract the Go
    // handler negotiates against, so they are covered alongside the app.
    include: ['src/**/*.test.{ts,tsx}', 'build/**/*.test.ts'],
    maxWorkers: 4,
    testTimeout: 10_000,
    css: true,
  },
})
