import { useEffect, useRef, useState } from 'react'
import { getDynamicYAML } from '../../api/client'
import { parseDynamicCollection } from '../../navigation/dynamic'
import type { DynamicRow } from '../../api/types'
import { Facts } from '../resource/Facts'
import { dateTime } from '../resource/format'
import { YamlViewer } from '../YamlViewer'
import type { WorkspaceEntry } from './ResourceWorkspaceProvider'

export function DynamicOverview({ value }: { value: DynamicRow }) {
  return <><Facts facts={[
    { label: 'Name', value: value.name }, { label: 'Namespace', value: value.namespace || 'Cluster' },
    ...value.columns.flatMap((column, index) => ['name', 'namespace', 'uid', 'resource version', 'created'].includes(column.name.toLowerCase()) || column.format === 'name' ? [] : [{ label: column.name, value: value.cells[index] ?? '—' }]),
    { label: 'Created', value: value.createdAt ? dateTime(value.createdAt) : 'Unknown' },
    { label: 'Resource version', value: value.resourceVersion || '—' }, { label: 'UID', value: value.uid || '—' },
  ]} />{value.truncated ? <p className="text-content text-kp-yellow">Some printer columns were shortened. Open YAML to inspect the full resource.</p> : null}</>
}

export function DynamicYAML({ entry, generation }: { entry: WorkspaceEntry; generation: string }) {
  const controller = useRef<AbortController | null>(null)
  const [value, setValue] = useState<string>()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown>(null)
  useEffect(() => () => controller.current?.abort(), [])
  async function load() {
    const resource = parseDynamicCollection(entry.collection)
    if (!resource) return
    controller.current?.abort()
    const request = new AbortController(); controller.current = request
    setPending(true); setError(null)
    try { const result = await getDynamicYAML(resource, entry.namespace, entry.name, request.signal, generation); if (!request.signal.aborted) setValue(result.yaml) }
    catch (error) { if (!request.signal.aborted) setError(error) }
    finally { if (!request.signal.aborted) setPending(false) }
  }
  return <><p className="m-0 text-content text-kp-overlay-text">Read-only resource YAML. Values keep their original encoding. The document loads only when requested.</p><YamlViewer value={value} pending={pending} error={error} onLoad={() => void load()} /></>
}
