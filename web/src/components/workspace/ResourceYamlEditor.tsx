import { useEffect, useId, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { parseDocument } from 'yaml'
import { APIError, getPermissions, getResourceYAML, saveResourceYAML, type ResourceYAMLDocument } from '../../api/client'
import type { SelectionSummary } from '../../api/types'
import { csrfForGeneration } from '../../actions/csrf'
import { Button } from '../ui'
import { errorMessage } from '../resource/errors'
import { YamlViewer } from '../YamlViewer'

const diffCollections = new Set(['pods', 'deployments', 'statefulsets', 'daemonsets', 'jobs', 'cronjobs', 'replicasets', 'services', 'ingresses', 'endpoint-slices', 'configmaps'])

function identity(value: string) {
  const document = parseDocument(value, { uniqueKeys: true })
  if (document.errors.length) throw new Error('Invalid YAML. Check indentation, duplicate keys and document structure.')
  const data = document.toJS({ maxAliasCount: 50 })
  for (const field of data?.kind === 'Secret' ? ['data'] : data?.kind === 'ConfigMap' ? ['binaryData'] : []) {
    for (const value of Object.values(data[field] ?? {})) {
      if (typeof value !== 'string') throw new Error(`${field} values must be Base64 strings. No automatic conversion is performed.`)
      try {
        const encoded = value.replace(/[\r\n]/g, '')
        if (btoa(atob(encoded)) !== encoded) throw new Error('Invalid Base64')
      } catch { throw new Error(`${field} values must be valid Base64. Use ${data.kind === 'Secret' ? 'stringData' : 'data'} for UTF-8 text.`) }
    }
  }
  if (typeof data?.apiVersion !== 'string' || typeof data?.kind !== 'string' || typeof data?.metadata?.name !== 'string' || typeof data?.metadata?.uid !== 'string' || typeof data?.metadata?.resourceVersion !== 'string') throw new Error('Keep the API version, kind, name, UID and resourceVersion from the loaded document.')
  return { apiVersion: data.apiVersion as string, kind: data.kind as string, name: data.metadata.name as string, namespace: (data.metadata.namespace ?? '') as string, uid: data.metadata.uid as string, resourceVersion: data.metadata.resourceVersion as string }
}

function saveError(error: unknown) {
  if (error instanceof APIError) {
    if (error.status === 413) return 'This YAML document exceeds the 2 MiB editor limit.'
    if (error.status === 409) return 'Version changed. Your draft is preserved; copy it before reloading.'
    if (error.status === 400 || error.status === 422) return 'Invalid YAML or immutable field. Your draft is preserved.'
  }
  return errorMessage(error)
}

export function ResourceYamlEditor({ collection, namespace, name, selection }: { collection: string; namespace: string | null; name: string; selection: SelectionSummary }) {
  const client = useQueryClient()
  const editorId = useId()
  const [original, setOriginal] = useState<ResourceYAMLDocument>()
  const [draft, setDraft] = useState('')
  const [editing, setEditing] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState('')
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])
  const capability = `yaml.${collection}.update`
  const permissions = useQuery({
    queryKey: ['yaml-permissions', selection.generation, collection, namespace, name],
    queryFn: ({ signal }) => getPermissions({ namespaces: namespace ? [namespace] : undefined, resourceNames: [name], capabilityIds: [capability], refresh: true }, signal, selection.generation),
    staleTime: 15_000,
  })
  const canEdit = !permissions.isError && permissions.data?.generation === selection.generation && (permissions.data?.decisions.some((item) => item.capabilityId === capability && (item.namespace ?? '') === (namespace ?? '') && item.resourceName === name && item.decision === 'allowed') ?? false)

  async function load() {
    controller.current?.abort()
    const request = new AbortController()
    controller.current = request
    setPending(true); setError(null); setSaved('')
    try {
      const value = await getResourceYAML(collection, namespace, name, selection.generation, request.signal)
      if (!request.signal.aborted) { setOriginal(value); setDraft(''); setEditing(false) }
    } catch (cause) { if (!request.signal.aborted) setError(cause) }
    finally { if (!request.signal.aborted) setPending(false) }
  }

  async function save() {
    if (!original || !canEdit || pending) return
    setPending(true); setError(null); setSaved('')
    const request = new AbortController()
    controller.current = request
    try {
      if (new TextEncoder().encode(draft).length > 2 * 1024 * 1024) throw new Error('YAML exceeds the 2 MiB editor limit.')
      const baseline = identity(original.yaml)
      const updated = identity(draft)
      if (baseline.kind !== original.kind || updated.name !== name || updated.namespace !== (namespace ?? '') || Object.keys(baseline).some((key) => updated[key as keyof typeof updated] !== baseline[key as keyof typeof baseline])) throw new Error('Keep apiVersion, kind, name, namespace, UID and resourceVersion unchanged. Copy your changes before loading a newer version.')
      const csrf = await csrfForGeneration(selection.generation)
      if (request.signal.aborted) return
      await saveResourceYAML(collection, namespace, name, { confirmed: true, action: 'updateResource', consequenceCode: 'UPDATE_RESOURCE', target: { clusterProfileId: selection.clusterProfileId, context: selection.context, namespace: namespace ?? '', kind: baseline.kind, name }, expectedGeneration: selection.generation, expectedUid: baseline.uid, expectedResourceVersion: baseline.resourceVersion, yaml: draft }, csrf, request.signal)
      if (request.signal.aborted) return
      setSaved(original.kind); setEditing(false); setOriginal(undefined); setDraft('')
      void client.invalidateQueries({ queryKey: ['resources'] })
      void client.invalidateQueries({ queryKey: ['workspace-detail', selection.generation] })
    } catch (cause) { if (!request.signal.aborted) setError(cause) }
    finally { if (!request.signal.aborted) setPending(false) }
  }

  return <section className="resource-yaml-editor grid gap-2" aria-label="Resource YAML editor">
    {collection === 'secrets' || collection === 'configmaps' ? <p className="m-0 text-content text-kp-subtext">{collection === 'secrets' ? 'Original Kubernetes format: data = Base64; stringData = UTF-8 text (converted by Kubernetes when saved).' : 'Original Kubernetes format: data = UTF-8 text; binaryData = Base64.'} Values are never decoded or converted by this editor.</p> : null}
    {saved ? <p role="status" className="text-content text-kp-green">{saved} saved. Load YAML to inspect the new version.</p> : null}
    {!editing ? <>
      {collection === 'secrets' && !original ? <p className="m-0 text-content text-kp-overlay-text">Load YAML to reveal this Secret’s full document. It is kept only while this tab is open.</p> : null}
      <YamlViewer value={original?.yaml} pending={pending} error={error ? new Error(saveError(error)) : null} onLoad={() => void load()} diffTarget={namespace && diffCollections.has(collection) ? { collection, namespace, name, generation: selection.generation } : undefined} actions={original ? <>
        <Button disabled={!canEdit || pending} onClick={() => { setDraft(original.yaml); setEditing(true); setError(null) }}>Edit YAML</Button>
        {!canEdit ? <span className="text-content text-kp-overlay-text">{permissions.isPending ? 'Checking update permission…' : permissions.isError ? 'Update permission could not be confirmed.' : 'Read only · update permission required.'}</span> : null}
        {!canEdit && !permissions.isPending ? <Button variant="secondary" disabled={permissions.isFetching} onClick={() => void permissions.refetch()}>Check permission again</Button> : null}
      </> : undefined} />
    </> : <>
      <div className="flex flex-wrap gap-2"><Button disabled={pending || !canEdit || draft === original?.yaml} onClick={() => void save()}>{pending ? 'Saving…' : `Save ${original?.kind}`}</Button><Button variant="secondary" disabled={pending} onClick={() => { setEditing(false); setDraft(''); setError(null) }}>Cancel editing</Button></div>
      <label htmlFor={editorId} className="text-content font-bold">{original?.kind} YAML</label>
      <textarea id={editorId} className="yaml-editor" spellCheck={false} autoCapitalize="off" autoComplete="off" value={draft} disabled={pending} onChange={(event) => setDraft(event.target.value)} />
      <p className="m-0 text-content text-kp-overlay-text" hidden={Boolean(error)}>Save updates this {original?.kind}.</p>
      {error ? <p role="alert" className="m-0 text-content text-kp-red">{saveError(error)}</p> : null}

    </>}
  </section>
}
