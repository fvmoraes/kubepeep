import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { parseDocument } from 'yaml'
import { getPermissions, getWorkloadYAML, saveDeploymentYAML } from '../../api/client'
import type { SelectionSummary } from '../../api/types'
import { csrfForGeneration } from '../../actions/csrf'
import { Button } from '../ui'
import { errorMessage } from '../resource/errors'
import { YamlViewer } from '../YamlViewer'

function metadata(value: string) {
  const document = parseDocument(value, { uniqueKeys: true })
  if (document.errors.length) throw new Error('Invalid YAML. Check indentation, duplicate keys and document structure.')
  const data = document.toJS({ maxAliasCount: 50 })
  if (data?.apiVersion !== 'apps/v1' || data?.kind !== 'Deployment' || !data?.metadata?.uid || !data?.metadata?.resourceVersion) throw new Error('Keep the Deployment API version, kind, UID and resourceVersion from the loaded document.')
  return data.metadata as { name: string; namespace: string; uid: string; resourceVersion: string }
}

export function DeploymentYamlEditor({ namespace, name, selection }: { namespace: string; name: string; selection: SelectionSummary }) {
  const client = useQueryClient()
  const [original, setOriginal] = useState<string>()
  const [draft, setDraft] = useState('')
  const [editing, setEditing] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])
  const permissions = useQuery({ queryKey: ['deployment-yaml-permissions', selection.generation, namespace, name], queryFn: ({ signal }) => getPermissions({ namespaces: [namespace], resourceNames: [name], capabilityIds: ['deployments.update'] }, signal, selection.generation), staleTime: 15_000 })
  const canEdit = permissions.data?.decisions.some((item) => item.capabilityId === 'deployments.update' && item.namespace === namespace && item.resourceName === name && item.decision === 'allowed') ?? false
  async function load() {
    controller.current?.abort(); const request = new AbortController(); controller.current = request
    setPending(true); setError(null); setSaved(false)
    try {
      const value = await getWorkloadYAML('deployments', namespace, name, request.signal)
      if (!request.signal.aborted) { setOriginal(value); setDraft(value); setEditing(false) }
    } catch (cause) { if (!request.signal.aborted) setError(cause) }
    finally { if (!request.signal.aborted) setPending(false) }
  }
  async function save() {
    if (!original || !canEdit) return
    setPending(true); setError(null); setSaved(false)
    const request = new AbortController(); controller.current = request
    try {
      const baseline = metadata(original)
      const updated = metadata(draft)
      if (updated.name !== name || updated.namespace !== namespace || updated.uid !== baseline.uid || updated.resourceVersion !== baseline.resourceVersion) throw new Error('Keep the name, namespace, UID and resourceVersion unchanged. Reload YAML to use a newer version.')
      const csrf = await csrfForGeneration(selection.generation)
      await saveDeploymentYAML(namespace, name, { confirmed: true, action: 'updateDeployment', consequenceCode: 'UPDATE_DEPLOYMENT', target: { clusterProfileId: selection.clusterProfileId, context: selection.context, namespace, kind: 'Deployment', name }, expectedGeneration: selection.generation, expectedUid: baseline.uid, expectedResourceVersion: baseline.resourceVersion, yaml: draft }, csrf, request.signal)
      if (request.signal.aborted) return
      setSaved(true); setEditing(false); setOriginal(undefined)
      void client.invalidateQueries({ queryKey: ['resources', 'workloads'] })
      void client.invalidateQueries({ queryKey: ['workspace-detail', selection.generation] })
    } catch (cause) { if (!request.signal.aborted) setError(cause) }
    finally { if (!request.signal.aborted) setPending(false) }
  }
  return <section className="grid gap-3" aria-label="Deployment YAML editor">
    {saved ? <p role="status" className="text-content text-kp-green">Deployment saved. Load YAML to inspect the new version.</p> : null}
    {!editing ? <><YamlViewer value={original} pending={pending} error={error} onLoad={() => void load()} diffTarget={{ collection: 'deployments', namespace, name, generation: selection.generation }} />{original !== undefined ? <Button disabled={!canEdit || pending} disabledReason="Updating this Deployment requires deployments.update permission." onClick={() => { setDraft(original); setEditing(true); setError(null) }}>Edit YAML</Button> : null}</> : <>
      <label htmlFor="deployment-yaml" className="text-content font-bold">Deployment YAML</label>
      <textarea id="deployment-yaml" className="yaml-editor" spellCheck={false} autoCapitalize="off" autoComplete="off" value={draft} disabled={pending} onChange={(event) => setDraft(event.target.value)} />
      <p className="m-0 text-content text-kp-overlay-text">Saving updates this Deployment in {namespace}. Pod template changes trigger a rollout. Conflicting changes require reloading the YAML.</p>
      {error ? <p role="alert" className="text-content text-kp-red">{errorMessage(error)}</p> : null}
      <div className="flex flex-wrap gap-2"><Button disabled={pending || draft === original} onClick={() => void save()}>{pending ? 'Saving…' : 'Save Deployment'}</Button><Button variant="secondary" disabled={pending} onClick={() => { setEditing(false); setDraft(original ?? ''); setError(null) }}>Cancel editing</Button></div>
    </>}
  </section>
}
