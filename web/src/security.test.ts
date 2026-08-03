import { readdir, readFile } from 'node:fs/promises'
import { extname, join } from 'node:path'
import { describe, expect, it } from 'vitest'

const forbiddenBrowserPersistence = [
  /navigator\s*\.\s*serviceWorker/,
  /\bcaches\s*\.\s*(?:open|match|put|delete)\s*\(/,
  /\bindexedDB\b/,
  /\b(?:localStorage|sessionStorage)\b/,
  /\b(?:PersistQueryClientProvider|persistQueryClient|createSyncStoragePersister|createAsyncStoragePersister)\b/,
] as const

async function productionSources(directory: string): Promise<string[]> {
  const files: string[] = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) {
      files.push(...await productionSources(path))
      continue
    }
    if (!['.ts', '.tsx'].includes(extname(entry.name)) || entry.name.includes('.test.')) {
      continue
    }
    files.push(path)
  }
  return files
}

describe('browser persistence boundary', () => {
  it('does not register offline storage or a query persister', async () => {
    const sourceRoot = join(process.cwd(), 'src')
    const violations: string[] = []
    for (const path of await productionSources(sourceRoot)) {
      const source = await readFile(path, 'utf8')
      for (const pattern of forbiddenBrowserPersistence) {
        if (pattern.test(source)) {
          violations.push(`${path}: ${pattern.source}`)
        }
      }
    }
    expect(violations).toEqual([])
  })
})
