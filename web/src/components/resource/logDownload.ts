import type { AggregatedLogLine } from './podLogStream'

export const LOG_DOWNLOAD_BYTES = 64 * 1024 * 1024
export function formatLogLine(line: AggregatedLogLine) {
  return `${line.timestamp ? `${line.timestamp} ` : ''}${line.pod}/${line.container} ${line.text}${line.truncated ? ' [truncated]' : ''}\n`
}

/** A separate session capture: evicting viewer rows never evicts captured logs. */
export class LogCapture {
  private chunks: string[] = []
  private chunk = ''
  bytes = 0
  lines = 0
  truncated = false
  constructor(private maximum = LOG_DOWNLOAD_BYTES) {}
  append(line: AggregatedLogLine) {
    const text = formatLogLine(line)
    const size = new TextEncoder().encode(text).length
    if (this.bytes + size > this.maximum) { this.truncated = true; return false }
    this.bytes += size
    this.lines++
    this.truncated ||= line.truncated
    this.chunk += text
    if (this.chunk.length >= 64 * 1024) { this.chunks.push(this.chunk); this.chunk = '' }
    return true
  }
  blob() { return new Blob([...this.chunks, this.chunk], { type: 'text/plain;charset=utf-8' }) }
}

export function saveLogFile(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = name.replace(/[^a-zA-Z0-9._-]/g, '-')
  anchor.click()
  // Keep the URL alive until the browser/WebView has consumed the download.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
