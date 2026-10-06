import { useEffect, useRef, useState } from 'react'
import { getSecretData } from '../../api/client'
import type { ConfigMapDetail, ContainerSpec, EnvironmentSource, ResourceRef } from '../../api/types'
import { Button } from '../ui'
import { errorMessage } from '../resource/errors'

export function DataEntries({ entries, empty = 'No data entries.' }: { entries: ConfigMapDetail['entries']; empty?: string }) {
  if (!entries.length) return <p className="text-sm text-kp-overlay-text">{empty}</p>
  return <div className="grid min-w-0 gap-2">{entries.map((entry) => <details key={entry.key} className="min-w-0 rounded-lg border border-kp-overlay-0 bg-kp-surface-1">
    <summary className="cursor-pointer break-all px-3 py-2 text-sm text-kp-sky">{entry.key} · {entry.encoding}{entry.truncated ? ' · truncated' : ''}</summary>
    <pre className="m-0 max-h-80 overflow-auto whitespace-pre-wrap border-t border-kp-overlay-0 p-3 text-sm text-kp-subtext [overflow-wrap:anywhere]">{entry.value}</pre>
  </details>)}</div>
}

export function SecretData({ namespace, name, generation }: { namespace: string; name: string; generation: string }) {
  const [data, setData] = useState<ConfigMapDetail | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => { controller.current?.abort() }, [])
  async function reveal() {
    controller.current?.abort()
    const request = new AbortController()
    controller.current = request
    setPending(true); setError(null); setData(null)
    try {
      const value = await getSecretData(namespace, name, request.signal, generation)
      if (!request.signal.aborted) setData(value)
    } catch (cause) { if (!request.signal.aborted) setError(cause) }
    finally { if (!request.signal.aborted) setPending(false) }
  }
  return <section className="grid gap-3" aria-label="Secret data">
    <p className="m-0 text-sm text-kp-subtext">Values are revealed on request and cleared when you leave this tab.</p>
    <div className="flex gap-2"><Button variant="secondary" disabled={pending} onClick={() => void reveal()}>{pending ? 'Loading data…' : data ? 'Refresh data' : 'Reveal data'}</Button>{data || pending ? <Button variant="ghost" onClick={() => { controller.current?.abort(); setData(null); setPending(false); setError(null) }}>Hide data</Button> : null}</div>
    {error ? <p role="alert" className="text-sm text-kp-red">{errorMessage(error)}</p> : null}
    {data ? <DataEntries entries={data.entries} empty="This Secret has no data." /> : null}
  </section>
}

export function WorkloadEnvironment({ containers, namespace, onOpen }: { containers: ContainerSpec[]; namespace: string; onOpen: (ref: ResourceRef) => void }) {
  function sourceLink(source: EnvironmentSource) {
    const reference = source.kind === 'Secret' || source.kind === 'ConfigMap'
    const label = `${source.kind}: ${source.name}${source.key ? ` / ${source.key}` : ''}`
    return reference ? <button type="button" className="text-left text-sm text-kp-mauve hover:underline [overflow-wrap:anywhere]" onClick={() => onOpen({ kind: source.kind, name: source.name, namespace })}>{label}</button> : <span className="text-sm text-kp-subtext">{label}</span>
  }
  return <div className="grid gap-5">
    <p className="m-0 max-w-[65ch] text-sm text-kp-overlay-text">Environment declared in the Pod specification. Open a Secret or ConfigMap reference to inspect its data. Values changed inside a running process are not part of this specification.</p>
    {containers.map((container) => <section key={container.name} className="min-w-0">
      <h3 className="mb-2 text-base font-semibold">{container.name}</h3>
      {!container.environment?.length && !container.envFrom?.length ? <p className="text-sm text-kp-overlay-text">No environment variables declared.</p> : null}
      {container.envFrom?.map((source, index) => <p key={`${source.kind}/${source.name}/${index}`} className="my-2 text-sm">{sourceLink(source)} <span className="text-kp-overlay-text">all keys{source.prefix ? ` · prefix ${source.prefix}` : ''}{source.optional ? ' · optional' : ''}</span></p>)}
      <dl className="grid min-w-0 gap-2">{container.environment?.map((variable) => <div key={variable.name} className="grid min-w-0 gap-1 border-b border-kp-divider py-2 sm:grid-cols-[minmax(10rem,1fr)_minmax(0,2fr)]">
        <dt className="break-all font-mono text-sm text-kp-text">{variable.name}</dt>
        <dd className="m-0 min-w-0">{variable.source ? sourceLink(variable.source) : <pre className="m-0 whitespace-pre-wrap text-sm text-kp-subtext [overflow-wrap:anywhere]">{variable.value === '' ? '(empty)' : variable.value}</pre>}</dd>
      </div>)}</dl>
    </section>)}
  </div>
}
