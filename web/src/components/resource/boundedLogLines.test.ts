import { expect, it } from 'vitest'
import { appendLogBatch } from './boundedLogLines'

it('retains the newest 1000 lines in order without mutating visible batches', () => {
  const original = [{ text: 'old', timestamp: null, truncated: false }]
  const next = appendLogBatch(original, Array.from({ length: 1200 }, (_, index) => ({ text: String(index), timestamp: null, truncated: false })))
  expect(next).toHaveLength(1000)
  expect(next[0].text).toBe('200')
  expect(next.at(-1)?.text).toBe('1199')
  expect(original[0].text).toBe('old')
})

it('bounds UTF-8 bytes as well as the number of lines', () => {
  const next = appendLogBatch([], Array.from({ length: 100 }, () => ({ text: '界'.repeat(10000), timestamp: null, truncated: false })))
  expect(next.length).toBeGreaterThan(0)
  expect(next.reduce((sum, line) => sum + new TextEncoder().encode(JSON.stringify(line)).byteLength, 0)).toBeLessThanOrEqual(1 << 20)
})
