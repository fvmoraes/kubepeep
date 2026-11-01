import { basename } from 'node:path'
import { constants as zlibConstants, brotliCompressSync, gzipSync } from 'node:zlib'
import type { Plugin } from 'vite'

// The Go binary serves the embedded dist over plain HTTP/1.1 with no
// compression middleware. Emitting the encoded siblings at build time lets the
// handler negotiate them without paying CPU per request, and keeps the
// single-binary deployment: no runtime compressor and no extra dependency,
// since Node already ships zlib.
//
// index.html is served by a dedicated handler that never negotiates encoding,
// so compressing it would only embed an unreachable file. Sourcemaps are off,
// and if they are ever enabled they should be reviewed before shipping
// compressed: Go has no MIME entry for .map, so ServeContent would sniff the
// compressed bytes and label a sourcemap as gzip.
const compressibleAsset = /\.(?:js|css|svg|json)$/

// Below roughly one KiB the encoded sibling stops paying for itself and only
// adds files to the embedded filesystem, which both binaries carry.
const minimumCompressedBytes = 1024

/** shouldPrecompress decides whether an emitted file earns encoded siblings. */
export function shouldPrecompress(fileName: string, byteLength: number): boolean {
  return compressibleAsset.test(fileName) && byteLength >= minimumCompressedBytes
}

export function precompressAssets(): Plugin {
  return {
    name: 'kubepeep-precompress-assets',
    apply: 'build',
    enforce: 'post',
    generateBundle: {
      // Compress what every other plugin already finished producing.
      order: 'post',
      handler(_options, bundle) {
        for (const [fileName, output] of Object.entries(bundle)) {
          const raw = Buffer.from(output.type === 'chunk' ? output.code : output.source)
          if (!shouldPrecompress(fileName, raw.byteLength)) continue
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

// The entry CSS declares every @font-face, so the browser cannot discover a
// font file until it has downloaded and parsed that stylesheet: two serial
// hops before text settles into its final face. Preloading the faces the shell
// actually paints collapses that into one.
//
// Only Roboto latin 400 and 700 are preloaded. 400 is --font-sans on :root and
// body; 700 is --text-title, --text-heading and --text-column, so the sidebar,
// headings and table headers need it on first paint. latin-ext covers
// characters the English-only UI rarely paints, and Roboto Mono belongs to
// YAML, logs and the terminal, which are lazy routes: preloading either would
// spend bandwidth for users who never reach them.
//
// No subset is removed from the entry CSS. styles.css sets font-synthesis:
// none, so a missing 700 would render bold text at 400 instead of synthesizing
// it, which is a visible regression rather than an optimization.
export const preloadedShellFonts = [
  /^roboto-latin-400-normal-[^.]+\.woff2$/,
  /^roboto-latin-700-normal-[^.]+\.woff2$/,
]

export function preloadShellFonts(): Plugin {
  let base = '/'
  return {
    name: 'kubepeep-preload-shell-fonts',
    apply: 'build',
    enforce: 'post',
    configResolved(config) {
      base = config.base
    },
    transformIndexHtml: {
      order: 'post',
      handler(html, context) {
        const emitted = Object.keys(context.bundle ?? {})
        const tags = preloadedShellFonts.flatMap((pattern) => {
          const file = emitted.find((name) => pattern.test(basename(name)))
          if (!file) {
            // Losing a preload silently would quietly restore the two-hop
            // critical path. If @fontsource renames a subset, that is a human
            // decision, so stop the build instead of shipping the regression.
            this.error(`no emitted font matched ${pattern}: the shell font preload would be dropped`)
            return []
          }
          return [{
            tag: 'link',
            // A @font-face fetch is always CORS mode. Without crossorigin the
            // preload lands in a different cache entry and the font is
            // downloaded twice.
            attrs: { rel: 'preload', as: 'font', type: 'font/woff2', crossorigin: '', href: `${base}${file}` },
            // Appended rather than prepended: the charset declaration must
            // stay first, and the preload scanner tokenizes the whole head
            // before fetching, so position inside head does not delay this.
            injectTo: 'head' as const,
          }]
        })
        return { html, tags }
      },
    },
  }
}
