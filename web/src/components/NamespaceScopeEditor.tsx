import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'

import {
  APIError,
  createNamespaceScope,
  deleteNamespaceScope,
  getNamespaceScopes,
  getSession,
  getStatus,
  selectNamespaceScope,
  updateNamespaceScope,
  validateNamespaceScope,
  type NamespaceScope,
  type NamespaceScopeMode,
  type NamespaceScopeValidation,
  type SelectionSummary,
} from '../api/client'
import { NamespaceInputError, parseNamespaceInput, validateNamespaceMode } from '../namespaces/namespaceInput'
import { StatePanel } from './StatePanel'
import { Badge, Button, Card, CardContent, CardHeader, Input, Select } from './ui'

interface NamespaceScopeFormProps {
  selection: SelectionSummary
  csrfToken: string | null
  scope?: NamespaceScope | null
  onSaved?: (scope: NamespaceScope) => void
  onCancel?: () => void
}

const emptyValidation: NamespaceScopeValidation = {
  valid: [],
  validCount: 0,
  duplicateCount: 0,
  discardedEmptyCount: 0,
  invalid: [],
  invalidCount: 0,
  existence: { checked: false, reasonCode: 'MODE_ALL' },
}

function messageFor(error: Error): string {
  return error instanceof APIError ? error.message : 'The local API is offline.'
}

export function NamespaceScopeForm({ selection, csrfToken, scope = null, onSaved, onCancel }: NamespaceScopeFormProps) {
  const [name, setName] = useState(scope?.name ?? '')
  const [mode, setMode] = useState<NamespaceScopeMode>(scope?.mode ?? 'single')
  const [rawInput, setRawInput] = useState(scope ? scope.namespaces.join('\n') : (selection.defaultNamespace ?? ''))
  const [defaultNamespace, setDefaultNamespace] = useState(scope?.defaultNamespace ?? selection.defaultNamespace ?? '')
  const [serverValidation, setServerValidation] = useState<NamespaceScopeValidation | null>(null)
  const editing = scope !== null

  const parsed = useMemo(() => {
    if (mode === 'all') {
      return { validation: emptyValidation, error: null }
    }
    try {
      return { validation: parseNamespaceInput(rawInput), error: null }
    } catch (error) {
      return {
        validation: emptyValidation,
        error: error instanceof NamespaceInputError ? error.message : 'The namespace input is invalid.',
      }
    }
  }, [mode, rawInput])

  const effectiveDefaultNamespace = mode === 'all'
    ? ''
    : parsed.validation.valid.includes(defaultNamespace)
      ? defaultNamespace
      : (parsed.validation.valid[0] ?? '')

  const updateMode = (value: NamespaceScopeMode) => {
    setMode(value)
    setServerValidation(null)
  }

  const updateRawInput = (value: string) => {
    setRawInput(value)
    setServerValidation(null)
  }

  const requestBody = () => ({
    clusterProfileId: selection.clusterProfileId,
    name: name.trim() || undefined,
    context: selection.context,
    mode,
    ...(mode === 'all' ? {} : { rawInput, defaultNamespace: effectiveDefaultNamespace || null }),
  })

  const validation = useMutation({
    mutationFn: () => validateNamespaceScope(requestBody(), csrfToken!),
    onSuccess: setServerValidation,
  })
  const save = useMutation({
    mutationFn: () => editing
      ? updateNamespaceScope(scope.id, {
        ...requestBody(),
        name: name.trim(),
        version: scope.version,
        expectedGeneration: selection.generation,
      }, csrfToken!)
      : createNamespaceScope({ ...requestBody(), name: name.trim() }, csrfToken!),
    onSuccess: (scope) => {
      onSaved?.(scope)
      if (!editing) {
        setName('')
      }
    },
  })

  const shownValidation = serverValidation ?? parsed.validation
  const modeError = parsed.error ?? validateNamespaceMode(mode, parsed.validation)
  const canContactServer = csrfToken !== null && !validation.isPending && !save.isPending
  const canValidate = canContactServer && name.trim() !== '' && modeError === null
  const canSave = canContactServer && name.trim() !== '' && modeError === null

  const removeItem = (item: string) => {
    const remaining = [
      ...parsed.validation.valid.filter((candidate) => candidate !== item),
      ...parsed.validation.invalid.map((candidate) => candidate.input).filter((candidate) => candidate !== item),
    ]
    updateRawInput(remaining.join('\n'))
  }

  return (
    <Card aria-labelledby="scope-editor-title">
      <CardContent className="grid gap-4 p-4">
        <CardHeader>
          <div>
            <h1 id="scope-editor-title" className="text-xl text-kp-text">{editing ? `Edit ${scope.name}` : 'Create a namespace scope'}</h1>
            <p className="mt-0.5 text-sm text-kp-overlay-text">Namespace scopes bound this installation to explicit context namespaces.</p>
          </div>
          <span className="mono text-xs text-kp-overlay-text">{selection.generation}</span>
        </CardHeader>

        <div className="grid gap-3 md:grid-cols-[minmax(100px,0.6fr)_minmax(160px,1fr)_minmax(180px,1.4fr)]" aria-label="Scope origin">
          <label className="grid gap-1"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Profile</span><Input aria-label="Scope cluster profile" value={String(selection.clusterProfileId)} readOnly /></label>
          <label className="grid gap-1"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Context</span><Input aria-label="Scope context" value={selection.context} readOnly /></label>
          <label className="grid gap-1"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Name</span><Input aria-label="Scope name" value={name} maxLength={120} onChange={(event) => setName(event.target.value)} placeholder="Finance workloads" /></label>
        </div>

        <fieldset className="m-0 flex flex-wrap items-center gap-2 border-0 p-0">
          <legend className="mb-1.5 w-full text-2xs uppercase tracking-wider text-kp-overlay-text">Mode</legend>
          {(['single', 'list', 'all'] as const).map((candidate) => (
            <label key={candidate} className="relative">
              <input type="radio" name="scope-mode" value={candidate} checked={mode === candidate} onChange={() => updateMode(candidate)} className="peer absolute opacity-0 pointer-events-none" />
              <span className={`block min-w-[86px] rounded-md border px-3 py-1.5 text-center text-sm cursor-pointer transition-colors peer-focus-visible:outline-2 peer-focus-visible:outline-kp-mauve peer-focus-visible:outline-offset-1 ${mode === candidate ? 'border-kp-accent-border bg-kp-accent-bg text-kp-mauve font-medium' : 'border-kp-overlay-1 bg-kp-surface-3 text-kp-subtext hover:border-kp-overlay-3 hover:text-kp-text'}`}>{candidate}</span>
            </label>
          ))}
        </fieldset>

        {mode === 'all' ? (
          <div className="rounded-r-md border-l-2 border-kp-yellow-border bg-kp-yellow-bg px-3 py-2.5 text-sm text-kp-yellow leading-relaxed" role="note">
            All mode stores no wildcard. The backend will activate only namespaces returned by Kubernetes after confirming list permission.
          </div>
        ) : (
          <>
            <label className="grid gap-1">
              <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Namespace input</span>
              <textarea
                aria-label="Namespace input"
                value={rawInput}
                onChange={(event) => updateRawInput(event.target.value)}
                rows={8}
                placeholder={'payments, billing\n---\n- invoices'}
                spellCheck={false}
                className="min-h-[150px] w-full rounded-md border border-kp-overlay-0 bg-kp-crust px-2.5 py-2 text-sm text-kp-text leading-relaxed focus:border-kp-mauve focus:shadow-focus focus:outline-none resize-y"
              />
            </label>
            <p className="m-0 text-xs text-kp-overlay-text">Plain delimiters, strict JSON arrays/objects, and simple YAML sequences are accepted.</p>
          </>
        )}

        <div className="grid grid-cols-2 gap-2 md:grid-cols-4" aria-label="Namespace validation counters">
          <div className="rounded-lg border border-kp-overlay-0 bg-kp-surface-1 px-2.5 py-2"><strong className="block text-2xl text-kp-text">{shownValidation.validCount}</strong><span className="block text-2xs uppercase tracking-wider text-kp-overlay-text">valid</span></div>
          <div className="rounded-lg border border-kp-overlay-0 bg-kp-surface-1 px-2.5 py-2"><strong className="block text-2xl text-kp-text">{shownValidation.duplicateCount}</strong><span className="block text-2xs uppercase tracking-wider text-kp-overlay-text">duplicates removed</span></div>
          <div className="rounded-lg border border-kp-overlay-0 bg-kp-surface-1 px-2.5 py-2"><strong className="block text-2xl text-kp-text">{shownValidation.discardedEmptyCount}</strong><span className="block text-2xs uppercase tracking-wider text-kp-overlay-text">empty removed</span></div>
          <div className="rounded-lg border border-kp-overlay-0 bg-kp-surface-1 px-2.5 py-2"><strong className="block text-2xl text-kp-text">{shownValidation.invalidCount}</strong><span className="block text-2xs uppercase tracking-wider text-kp-overlay-text">invalid</span></div>
        </div>

        {mode !== 'all' && (parsed.validation.valid.length > 0 || parsed.validation.invalid.length > 0) ? (
          <div className="flex flex-wrap gap-1.5" aria-label="Parsed namespaces">
            {parsed.validation.valid.map((namespace) => (
              <button key={`valid-${namespace}`} type="button" className="inline-flex items-center gap-1.5 rounded-full border border-kp-blue-border bg-kp-blue-bg px-2.5 py-1 text-xs text-kp-sky cursor-pointer hover:border-kp-overlay-3" onClick={() => removeItem(namespace)} aria-label={`Remove namespace ${namespace}`} title="Remove this namespace from the input">
                {namespace}<span aria-hidden="true">×</span>
              </button>
            ))}
            {parsed.validation.invalid.map(({ input }) => (
              <button key={`invalid-${input}`} type="button" className="inline-flex items-center gap-1.5 rounded-full border border-kp-red-border bg-kp-red-bg px-2.5 py-1 text-xs text-kp-red cursor-pointer hover:border-kp-overlay-3" onClick={() => removeItem(input)} aria-label={`Remove invalid namespace ${input}`} title="Remove this entry from the input">
                {input}<span aria-hidden="true">×</span>
              </button>
            ))}
          </div>
        ) : null}

        {mode !== 'all' ? (
          <label className="grid max-w-[340px] gap-1">
            <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Default namespace</span>
            <Select aria-label="Default namespace" value={effectiveDefaultNamespace} onChange={(event) => setDefaultNamespace(event.target.value)} disabled={parsed.validation.valid.length === 0}>
              <option value="">Choose a valid namespace</option>
              {parsed.validation.valid.map((namespace) => <option key={namespace} value={namespace}>{namespace}</option>)}
            </Select>
          </label>
        ) : null}

        {modeError ? <p className="m-0 text-xs text-kp-red" role="alert">{modeError}</p> : null}
        {serverValidation?.existence.checked === false && serverValidation.existence.reasonCode !== 'LOCAL_VALIDATION_ONLY' ? (
          <p className="m-0 text-xs text-kp-overlay-text" role="status">Existence was not checked: {serverValidation.existence.reasonCode ?? 'permission unavailable'}.</p>
        ) : null}
        {validation.isError ? <p className="m-0 text-xs text-kp-red" role="alert">{messageFor(validation.error)}</p> : null}
        {save.isError ? <p className="m-0 text-xs text-kp-red" role="alert">{messageFor(save.error)}</p> : null}
        {save.isSuccess ? <p className="m-0 text-xs text-kp-green" role="status">Scope “{save.data.name}” was {editing ? 'updated' : 'saved'}.</p> : null}

        <div className="flex flex-wrap justify-end gap-2">
          {editing ? <Button variant="secondary" onClick={onCancel} disabled={save.isPending}>Cancel edit</Button> : null}
          <Button variant="secondary" onClick={() => updateRawInput('')} disabled={mode === 'all' || rawInput === ''}>Clear</Button>
          <Button variant="secondary" onClick={() => validation.mutate()} disabled={!canValidate}>
            {validation.isPending ? 'Validating…' : 'Validate with cluster'}
          </Button>
          <Button onClick={() => save.mutate()} disabled={!canSave}>
            {save.isPending ? (editing ? 'Updating…' : 'Save scope') : (editing ? 'Update scope' : 'Save scope')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

export function NamespaceScopeEditor() {
  const queryClient = useQueryClient()
  const [editingScopeId, setEditingScopeId] = useState<number | null>(null)
  const [deleteScopeId, setDeleteScopeId] = useState<number | null>(null)
  const [replacementScopeId, setReplacementScopeId] = useState<number | null>(null)
  const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000, retry: false })
  const session = useQuery({ queryKey: ['session'], queryFn: ({ signal }) => getSession(signal), staleTime: 5 * 60_000, retry: false })
  const scopes = useQuery({
    queryKey: ['namespace-scopes'],
    queryFn: ({ signal }) => getNamespaceScopes({ limit: 100 }, signal),
    enabled: status.data?.selection !== null && status.data?.selection !== undefined,
    retry: false,
  })
  const reconcile = async () => {
    setEditingScopeId(null)
    setDeleteScopeId(null)
    setReplacementScopeId(null)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['local-status'] }),
      queryClient.invalidateQueries({ queryKey: ['namespace-scopes'] }),
      queryClient.invalidateQueries({ queryKey: ['permissions'] }),
      queryClient.invalidateQueries({ queryKey: ['dashboard'] }),
    ])
    queryClient.removeQueries({ queryKey: ['session'] })
  }
  const selection = status.data?.selection ?? null
  const selectScope = useMutation({
    mutationFn: (scope: NamespaceScope) => {
      if (!selection || !session.data) throw new Error('Scope selection is not ready.')
      return selectNamespaceScope(scope.id, { expectedGeneration: selection.generation }, session.data.csrfToken)
    },
    onSuccess: reconcile,
  })
  const deleteScope = useMutation({
    mutationFn: ({ scope, replacement }: { scope: NamespaceScope; replacement: number | null }) => {
      if (!selection || !session.data) throw new Error('Scope deletion is not ready.')
      return deleteNamespaceScope(scope.id, {
        confirmed: true,
        version: scope.version,
        ...(replacement === null ? {} : { replacementScopeId: replacement }),
        expectedGeneration: selection.generation,
      }, session.data.csrfToken)
    },
    onSuccess: reconcile,
  })

  if (status.isPending) {
    return <StatePanel kind="loading" title="Loading namespace scopes">Checking the active context and generation.</StatePanel>
  }
  if (status.isError) {
    return <StatePanel kind="offline" title="Namespace scopes are offline">The local API could not be reached.</StatePanel>
  }
  if (!selection) {
    return <StatePanel kind="empty" title="Select a context first">Namespace scopes are always bound to an explicit cluster profile and context.</StatePanel>
  }

  const scopeList = scopes.data ?? []
  const editingScope = scopeList.find((scope) => scope.id === editingScopeId) ?? null
  const deleteTarget = scopeList.find((scope) => scope.id === deleteScopeId) ?? null
  const deletingActive = deleteTarget?.id === selection.scopeId
  const replacementCandidates = deletingActive ? scopeList.filter((scope) => scope.id !== deleteTarget.id) : []
  const deletionReady = deleteTarget !== null && (!deletingActive || replacementScopeId !== null)

  return (
    <div className="grid max-w-[1040px] gap-4">
      <NamespaceScopeForm
        key={editingScope ? `edit-${editingScope.id}-${editingScope.version}` : 'create'}
        selection={selection}
        csrfToken={session.data?.csrfToken ?? null}
        scope={editingScope}
        onCancel={() => setEditingScopeId(null)}
        onSaved={() => void reconcile()}
      />
      <Card aria-labelledby="saved-scopes-title">
        <CardContent className="grid gap-3 p-4">
          <CardHeader>
            <div>
              <h2 id="saved-scopes-title" className="text-xl text-kp-text">Namespace scopes</h2>
              <p className="mt-0.5 text-sm text-kp-overlay-text">Saved locally; selecting one activates its namespaces for this session.</p>
            </div>
          </CardHeader>
          {scopes.isPending ? <p className="m-0 text-sm text-kp-overlay-text" role="status">Loading saved scopes…</p> : null}
          {scopes.isError ? <p className="m-0 text-sm text-kp-red" role="status">Saved scopes are temporarily unavailable.</p> : null}
          {scopeList.length === 0 && scopes.isSuccess ? <p className="m-0 text-sm text-kp-overlay-text">No saved scopes for this local installation.</p> : null}
          {scopeList.length ? (
            <ul className="m-0 grid list-none gap-2 p-0">
              {scopeList.map((scope) => (
                <li key={scope.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-kp-overlay-0 bg-kp-surface-1 px-3 py-2.5">
                  <div className="min-w-0"><strong className="block text-sm text-kp-text">{scope.name}</strong><small className="block text-xs text-kp-overlay-text">{scope.context} · {scope.mode} · {scope.namespaces.length} namespaces</small></div>
                  <div className="flex flex-wrap items-center justify-end gap-2">
                    {selection.scopeId === scope.id ? <Badge variant="healthy">active</Badge> : (
                      <Button size="sm" disabled={!session.data || selectScope.isPending || deleteScope.isPending} onClick={() => selectScope.mutate(scope)}>Select {scope.name}</Button>
                    )}
                    <Button variant="secondary" size="sm" disabled={selectScope.isPending || deleteScope.isPending} onClick={() => {
                      setEditingScopeId(scope.id)
                      setDeleteScopeId(null)
                    }}>Edit {scope.name}</Button>
                    <Button variant="danger" size="sm" disabled={!session.data || selectScope.isPending || deleteScope.isPending} onClick={() => {
                      setDeleteScopeId(scope.id)
                      setReplacementScopeId(null)
                    }}>Delete {scope.name}</Button>
                  </div>
                </li>
              ))}
            </ul>
          ) : null}
          {selectScope.isError ? <p className="m-0 text-xs text-kp-red" role="alert">{messageFor(selectScope.error)}</p> : null}
          {deleteScope.isError ? <p className="m-0 text-xs text-kp-red" role="alert">{messageFor(deleteScope.error)}</p> : null}
          {deleteTarget ? (
            <div role="alertdialog" aria-labelledby="scope-delete-title" className="grid gap-3 rounded-lg border border-kp-red-border bg-kp-red-bg/50 p-3.5">
              <div>
                <strong id="scope-delete-title" className="block text-sm text-kp-text">Delete “{deleteTarget.name}”?</strong>
                <p className="m-0 mt-1 text-xs text-kp-subtext">This removes the local scope definition. Kubernetes resources are not modified.</p>
              </div>
              {deletingActive ? (
                <label className="grid max-w-[360px] gap-1">
                  <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Replacement scope</span>
                  <Select aria-label="Replacement scope" value={replacementScopeId ?? ''} onChange={(event) => setReplacementScopeId(event.target.value ? Number(event.target.value) : null)}>
                    <option value="">Choose before deleting the active scope</option>
                    {replacementCandidates.map((scope) => <option key={scope.id} value={scope.id}>{scope.name}</option>)}
                  </Select>
                </label>
              ) : null}
              {deletingActive && replacementCandidates.length === 0 ? <p className="m-0 text-xs text-kp-red">Create another scope before deleting the active scope.</p> : null}
              <div className="flex flex-wrap justify-end gap-2">
                <Button variant="secondary" onClick={() => {
                  setDeleteScopeId(null)
                  setReplacementScopeId(null)
                }} disabled={deleteScope.isPending}>Cancel deletion</Button>
                <Button variant="danger" disabled={!deletionReady || deleteScope.isPending} onClick={() => deleteScope.mutate({ scope: deleteTarget, replacement: replacementScopeId })}>
                  {deleteScope.isPending ? 'Deleting…' : `Confirm delete ${deleteTarget.name}`}
                </Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
