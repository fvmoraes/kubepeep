import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { constants as zlibConstants, brotliCompressSync, gzipSync } from 'node:zlib'
import type { Plugin } from 'vite'
import { defineConfig } from 'vitest/config'

// The Go binary serves the embedded dist over plain HTTP/1.1 with no
// compression middleware. Emitting the encoded siblings at build time lets the
// handler negotiate them without paying CPU per request, and keeps the
// single-binary deployment: no runtime compressor and no extra dependency,
// since Node already ships zlib.
const compressibleAsset = /\.(?:js|css|html|svg|json|map)$/
// Below roughly one KiB the encoded sibling stops paying for itself and only
// adds files to the embedded filesystem.
const minimumCompressedBytes = 1024

function precompressAssets(): Plugin {
  return {
    name: 'kubepeep-precompress-assets',
    apply: 'build',
    enforce: 'post',
    generateBundle: {
      // Compress what every other plugin already finished producing.
      order: 'post',
      handler(_options, bundle) {
        for (const [fileName, output] of Object.entries(bundle)) {
          if (!compressibleAsset.test(fileName)) continue
          const raw = Buffer.from(output.type === 'chunk' ? output.code : output.source)
          if (raw.byteLength < minimumCompressedBytes) continue
          this.emitFile({
            type: 'asset',
            fileName: `${fileName}.br`,
            source: brotliCompressSync(raw, {
              params: {
                [zlibConstants.BROTLI_PARAM_QUALITY]: zlibConstants.BROTLI_MAX_QUALITY,
                [zlibConstants.BROTLI_PARAM_SIZE_HINT]: raw.byteLength,
              },
            }),
          })
          this.emitFile({ type: 'asset', fileName: `${fileName}.gz`, source: gzipSync(raw, { level: 9 }) })
        }
      },
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), precompressAssets()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    sourcemap: false,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    maxWorkers: 4,
    testTimeout: 10_000,
    css: true,
  },
})
