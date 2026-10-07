import { expect, it } from 'vitest'
import { LogCapture, formatLogLine } from './logDownload'
import { appendLogBatch } from './boundedLogLines'

it('keeps the session independently of the bounded viewer and marks size limits explicitly', () => {
  const capture = new LogCapture()
  const lines = Array.from({ length: 1200 }, (_, index) => ({ pod: 'ns/api', container: 'main', timestamp: null, text: `line ${index}`, truncated: false }))
  lines.forEach((line) => capture.append(line))
  expect(appendLogBatch([], lines)).toHaveLength(1000)
  expect(capture.lines).toBe(1200)
  expect(capture.truncated).toBe(false)
  expect(capture.blob().size).toBe(capture.bytes)
  const limited = new LogCapture(10)
  expect(limited.append(lines[0])).toBe(false)
  expect(limited.truncated).toBe(true)
  expect(limited.blob().size).toBeLessThanOrEqual(10)
})

it('exports timestamps, exact targets and truncation markers as plain text', () => {
  expect(formatLogLine({ pod: 'ns/api', container: 'main', timestamp: '2026-10-06T12:00:00Z', text: '<script>text</script>', truncated: true })).toBe('2026-10-06T12:00:00Z ns/api/main <script>text</script> [truncated]\n')
})
