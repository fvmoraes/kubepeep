import { brotliDecompressSync, gunzipSync } from 'node:zlib'
import { describe, expect, it } from 'vitest'

import { precompressAssets, preloadShellFonts, shouldPrecompress } from './assetPlugins'

// The plugins decide the asset contract the Go handler negotiates against, so
// a silent change here is a silent change to what ships. Rollup's hooks are
// invoked directly with a minimal context rather than through a real build.

function runGenerateBundle(bundle: Record<string, unknown>) {
  const plugin = precompressAssets()
  const hook = plugin.generateBundle as { handler: (this: unknown, options: unknown, bundle: unknown) => void }
  const emitted: Array<{ fileName: string; source: Buffer }> = []
  hook.handler.call({ emitFile: (file: { fileName: string; source: Buffer }) => emitted.push(file) }, {}, bundle)
  return emitted
}

function chunk(code: string) {
  return { type: 'chunk' as const, code }
}

function asset(source: string) {
  return { type: 'asset' as const, source }
}

describe('precompressAssets', () => {
  it('emits both encodings for a text asset above the threshold', () => {
    const code = 'x'.repeat(4096)
    const emitted = runGenerateBundle({ 'assets/app-AbCd1234.js': chunk(code) })

    expect(emitted.map((file) => file.fileName).sort()).toEqual([
      'assets/app-AbCd1234.js.br',
      'assets/app-AbCd1234.js.gz',
    ])
    // The sibling must decode back to the exact bytes the handler would have
    // served as identity, or the client silently receives different code.
    const brotli = emitted.find((file) => file.fileName.endsWith('.br'))!
    const gzip = emitted.find((file) => file.fileName.endsWith('.gz'))!
    expect(brotliDecompressSync(brotli.source).toString()).toBe(code)
    expect(gunzipSync(gzip.source).toString()).toBe(code)
  })

  it('leaves a file below the threshold uncompressed', () => {
    expect(runGenerateBundle({ 'assets/tiny-AbCd1234.js': chunk('export {}') })).toEqual([])
  })

  it('skips fonts and images, which are already compressed formats', () => {
    const payload = 'x'.repeat(4096)
    const emitted = runGenerateBundle({
      'assets/roboto-latin-400-normal-AbCd1234.woff2': asset(payload),
      'assets/roboto-latin-400-normal-AbCd1234.woff': asset(payload),
      'assets/icon-AbCd1234.png': asset(payload),
    })
    expect(emitted).toEqual([])
  })

  it('never compresses index.html, which the Go handler serves without negotiating', () => {
    expect(runGenerateBundle({ 'index.html': asset('<!doctype html>'.repeat(200)) })).toEqual([])
  })

  it('never compresses a sourcemap, which Go has no MIME entry for', () => {
    expect(runGenerateBundle({ 'assets/app-AbCd1234.js.map': asset('{"version":3}'.repeat(200)) })).toEqual([])
  })

  it('does not recurse into the siblings it just emitted', () => {
    const payload = 'x'.repeat(4096)
    expect(runGenerateBundle({ 'assets/app-AbCd1234.js.br': asset(payload) })).toEqual([])
    expect(runGenerateBundle({ 'assets/app-AbCd1234.js.gz': asset(payload) })).toEqual([])
  })
})

describe('shouldPrecompress', () => {
  it.each([
    ['assets/app-AbCd1234.js', 1024, true],
    ['assets/app-AbCd1234.js', 1023, false],
    ['assets/index-AbCd1234.css', 8192, true],
    ['assets/logo-AbCd1234.svg', 8192, true],
    ['index.html', 8192, false],
    ['assets/font-AbCd1234.woff2', 8192, false],
  ] as const)('%s at %d bytes -> %s', (fileName, bytes, want) => {
    expect(shouldPrecompress(fileName, bytes)).toBe(want)
  })
})

function runTransformIndexHtml(bundle: Record<string, unknown>) {
  const plugin = preloadShellFonts()
  const hook = plugin.transformIndexHtml as {
    handler: (this: unknown, html: string, context: unknown) => { tags: Array<{ attrs: Record<string, string>; injectTo: string }> }
  }
  const context = {
    error: (message: string) => {
      // Rollup's error() throws, and the plugin relies on that to stop the
      // build rather than ship a missing preload.
      throw new Error(String(message))
    },
  }
  return hook.handler.call(context, '<html></html>', { bundle })
}

describe('preloadShellFonts', () => {
  const warmBundle = {
    'assets/roboto-latin-400-normal-BqEyEoaF.woff2': asset(''),
    'assets/roboto-latin-700-normal-BZpUvMxY.woff2': asset(''),
    'assets/roboto-mono-latin-400-normal-GekRknry.woff2': asset(''),
    'assets/roboto-latin-ext-400-normal-C3tdtHj3.woff2': asset(''),
  }

  it('preloads exactly the two faces the shell paints, resolving the hashed names', () => {
    const { tags } = runTransformIndexHtml(warmBundle)
    expect(tags.map((tag) => tag.attrs.href)).toEqual([
      '/assets/roboto-latin-400-normal-BqEyEoaF.woff2',
      '/assets/roboto-latin-700-normal-BZpUvMxY.woff2',
    ])
  })

  it('leaves Roboto Mono and latin-ext unpreloaded, since lazy routes own them', () => {
    const { tags } = runTransformIndexHtml(warmBundle)
    const hrefs = tags.map((tag) => tag.attrs.href).join(' ')
    expect(hrefs).not.toContain('mono')
    expect(hrefs).not.toContain('latin-ext')
  })

  it('marks every preload as a CORS font fetch so the cache entry is shared', () => {
    const { tags } = runTransformIndexHtml(warmBundle)
    for (const tag of tags) {
      expect(tag.attrs).toMatchObject({ rel: 'preload', as: 'font', type: 'font/woff2', crossorigin: '' })
    }
  })

  it('appends to head so the charset declaration stays first', () => {
    const { tags } = runTransformIndexHtml(warmBundle)
    for (const tag of tags) {
      expect(tag.injectTo).toBe('head')
    }
  })

  it('fails the build when a subset is renamed, instead of dropping the preload', () => {
    expect(() => runTransformIndexHtml({ 'assets/roboto-latin-400-normal-BqEyEoaF.woff2': asset('') }))
      .toThrow(/the shell font preload would be dropped/)
  })
})
