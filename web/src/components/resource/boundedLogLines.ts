import type { LogLine } from '../../api/types'

const encoder = new TextEncoder()
const sizes = new WeakMap<LogLine, number>()

function byteLength(line: LogLine) {
  let size = sizes.get(line)
  if (size === undefined) { size = encoder.encode(JSON.stringify(line)).byteLength; sizes.set(line, size) }
  return size
}

/** Bound once per render batch, encoding each immutable line only once. */
export function appendLogBatch<T extends LogLine>(current: T[], batch: T[]): T[] {
  const next = current.concat(batch)
  let bytes = 0
  let start = next.length
  while (start > 0 && next.length - start < 1_000) {
    const size = byteLength(next[start - 1])
    if (bytes + size > 1 << 20) break
    bytes += size
    start--
  }
  return next.slice(start)
}
