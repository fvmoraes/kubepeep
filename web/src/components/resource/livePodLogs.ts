import { APIError, getSession } from '../../api/client'
import { appendLogBatch } from './boundedLogLines'
import { consumeAggregateStream, type AggregatedLogLine, type LogTarget } from './podLogStream'

export function logAccessLost(error: unknown) {
  return error instanceof APIError && ([401, 403, 409].includes(error.status) || ['FORBIDDEN', 'AUTHENTICATION_UNAVAILABLE', 'AUTHORIZATION_UNAVAILABLE', 'GENERATION_CHANGED', 'CSRF_REJECTED'].includes(error.code))
}

function delay(milliseconds: number, signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    signal.throwIfAborted()
    const abort = () => { clearTimeout(timer); reject(signal.reason) }
    const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve() }, milliseconds)
    signal.addEventListener('abort', abort, { once: true })
  })
}

/** Each exact target reconnects independently, retaining its own overlap cursor. */
export async function followPodLogs(target: LogTarget, generation: string, signal: AbortSignal, onLine: (line: AggregatedLogLine) => void, onStatus: (status: string) => void, onUnavailable?: () => void) {
  let recent: AggregatedLogLine[] = []
  let latest = 0
  let attempts = 0
  while (!signal.aborted) {
    const controller = new AbortController()
    const abort = () => controller.abort(signal.reason)
    signal.addEventListener('abort', abort, { once: true })
    let watchdog: ReturnType<typeof setTimeout>
    const activity = () => { clearTimeout(watchdog); watchdog = setTimeout(() => controller.abort(new Error('The log connection stopped responding.')), 45_000) }
    activity()
    const boundary = latest
    const replay = new Map<string, number>()
    for (const line of recent) {
      if (!line.timestamp) continue
      const key = JSON.stringify([line.timestamp, line.text, line.truncated])
      replay.set(key, (replay.get(key) ?? 0) + 1)
    }
    onStatus(attempts ? 'Reconnecting…' : 'Connecting…')
    try {
      const session = await getSession(controller.signal)
      if (session.generation !== generation) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed.' })
      const since = latest ? `${Math.min(14400, Math.max(1, Math.ceil((Date.now() - latest) / 1000) + 1))}s` : ''
      const terminal = await consumeAggregateStream(target, { generation }, session, {
        timestamps: true, tailLines: latest ? 2000 : 200, since,
        onOpen: () => onStatus('Live'), onActivity: activity,
      }, controller.signal, (line) => {
        const time = line.timestamp ? Date.parse(line.timestamp) : NaN
        if (boundary && Number.isFinite(time) && time < boundary) return
        const key = JSON.stringify([line.timestamp, line.text, line.truncated])
        const count = line.timestamp ? replay.get(key) ?? 0 : 0
        if (count) { replay.set(key, count - 1); return }
        if (Number.isFinite(time)) latest = Math.max(latest, time)
        recent = appendLogBatch(recent, [line])
        onLine(line)
      })
      if (terminal.reason === 'container_terminated') { onStatus('Container ended'); return }
      if (['generation_changed', 'server_shutdown'].includes(terminal.reason)) throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The log session ended because the connection changed.' })
      if (terminal.reason === 'limit_reached') onStatus('Reconnecting · stream limit')
    } catch (error) {
      if (signal.aborted) return
      if (error instanceof APIError && error.code === 'AUTHORIZATION_UNAVAILABLE') {
        recent = []; latest = 0
        onUnavailable?.()
      } else if (logAccessLost(error)) throw error
      onStatus('Reconnecting…')
    } finally {
      clearTimeout(watchdog!)
      controller.abort()
      signal.removeEventListener('abort', abort)
    }
    attempts++
    onStatus('Reconnecting…')
    await delay(Math.min(10_000, 1000 * 2 ** Math.min(attempts - 1, 4)), signal)
  }
}
