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
  const canSave = canContactServer && name.trim() !== '' && modeError === null

  const removeItem = (item: string) => {
    const remaining = [
      ...parsed.validation.valid.filter((candidate) => candidate !== item),
      ...parsed.validation.invalid.map((candidate) => candidate.input).filter((candidate) => candidate !== item),
    ]
    updateRawInput(remaining.join('\n'))
  }

  return (
    <section className="feature-card scope-editor" aria-labelledby="scope-editor-title">
      <div className="feature-heading">
        <div>
          <span className="eyebrow">namespace scope</span>
          <h1 id="scope-editor-title">{editing ? `Edit ${scope.name}` : 'Create a namespace scope'}</h1>
        </div>
        <span className="generation-label">{selection.generation}</span>
      </div>

      <div className="scope-origin" aria-label="Scope origin">
        <label>Profile<input aria-label="Scope cluster profile" value={String(selection.clusterProfileId)} readOnly /></label>
        <label>Context<input aria-label="Scope context" value={selection.context} readOnly /></label>
        <label>Name<input aria-label="Scope name" value={name} maxLength={120} onChange={(event) => setName(event.target.value)} placeholder="Finance workloads" /></label>
      </div>

      <fieldset className="mode-picker">
        <legend>Mode</legend>
        {(['single', 'list', 'all'] as const).map((candidate) => (
          <label key={candidate}>
            <input type="radio" name="scope-mode" value={candidate} checked={mode === candidate} onChange={() => updateMode(candidate)} />
            <span>{candidate}</span>
          </label>
        ))}
      </fieldset>

      {mode === 'all' ? (
        <div className="scope-all-note" role="note">
          All mode stores no wildcard. The backend will activate only namespaces returned by Kubernetes after confirming list permission.
        </div>
      ) : (
        <>
          <label className="textarea-field">
            <span>Namespace input</span>
            <textarea
              aria-label="Namespace input"
              value={rawInput}
              onChange={(event) => updateRawInput(event.target.value)}
              rows={8}
              placeholder={'payments, billing\n---\n- invoices'}
              spellCheck={false}
            />
          </label>
          <p className="field-help">Plain delimiters, strict JSON arrays/objects, and simple YAML sequences are accepted.</p>
        </>
      )}

      <div className="validation-counters" aria-label="Namespace validation counters">
        <div><strong>{shownValidation.validCount}</strong><span>valid</span></div>
        <div><strong>{shownValidation.duplicateCount}</strong><span>duplicates removed</span></div>
        <div><strong>{shownValidation.discardedEmptyCount}</strong><span>empty removed</span></div>
        <div><strong>{shownValidation.invalidCount}</strong><span>invalid</span></div>
      </div>

      {mode !== 'all' && (parsed.validation.valid.length > 0 || parsed.validation.invalid.length > 0) ? (
        <div className="namespace-chips" aria-label="Parsed namespaces">
          {parsed.validation.valid.map((namespace) => (
            <button key={`valid-${namespace}`} type="button" className="namespace-chip" onClick={() => removeItem(namespace)} aria-label={`Remove namespace ${namespace}`}>
              {namespace}<span aria-hidden="true">×</span>
            </button>
          ))}
          {parsed.validation.invalid.map(({ input }) => (
            <button key={`invalid-${input}`} type="button" className="namespace-chip namespace-chip--invalid" onClick={() => removeItem(input)} aria-label={`Remove invalid namespace ${input}`}>
              {input}<span aria-hidden="true">×</span>
            </button>
          ))}
        </div>
      ) : null}

      {mode !== 'all' ? (
        <label className="default-namespace">
          <span>Default namespace</span>
          <select aria-label="Default namespace" value={effectiveDefaultNamespace} onChange={(event) => setDefaultNamespace(event.target.value)} disabled={parsed.validation.valid.length === 0}>
            <option value="">Choose a valid namespace</option>
            {parsed.validation.valid.map((namespace) => <option key={namespace} value={namespace}>{namespace}</option>)}
          </select>
        </label>
      ) : null}

      {modeError ? <p className="field-error" role="alert">{modeError}</p> : null}
      {serverValidation?.existence.checked === false && serverValidation.existence.reasonCode !== 'LOCAL_VALIDATION_ONLY' ? (
        <p className="field-help" role="status">Existence was not checked: {serverValidation.existence.reasonCode ?? 'permission unavailable'}.</p>
      ) : null}
      {validation.isError ? <p className="field-error" role="alert">{messageFor(validation.error)}</p> : null}
      {save.isError ? <p className="field-error" role="alert">{messageFor(save.error)}</p> : null}
      {save.isSuccess ? <p className="field-success" role="status">Scope “{save.data.name}” was {editing ? 'updated' : 'saved'}.</p> : null}

      <div className="form-actions">
        {editing ? <button type="button" className="button button--secondary" onClick={onCancel} disabled={save.isPending}>Cancel edit</button> : null}
        <button type="button" className="button button--secondary" onClick={() => updateRawInput('')} disabled={mode === 'all' || rawInput === ''}>Clear</button>
        <button type="button" className="button button--secondary" onClick={() => validation.mutate()} disabled={!canContactServer || parsed.error !== null}>
          {validation.isPending ? 'Validating…' : 'Validate with cluster'}
        </button>
        <button type="button" className="button" onClick={() => save.mutate()} disabled={!canSave}>
          {save.isPending ? (editing ? 'Updating…' : 'Saving…') : (editing ? 'Update scope' : 'Save scope')}
        </button>
      </div>
    </section>
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
    <div className="feature-stack">
      <NamespaceScopeForm
        key={editingScope ? `edit-${editingScope.id}-${editingScope.version}` : 'create'}
        selection={selection}
        csrfToken={session.data?.csrfToken ?? null}
        scope={editingScope}
        onCancel={() => setEditingScopeId(null)}
        onSaved={() => void reconcile()}
      />
      <section className="feature-card" aria-labelledby="saved-scopes-title">
        <div className="feature-heading"><div><span className="eyebrow">saved locally</span><h2 id="saved-scopes-title">Namespace scopes</h2></div></div>
        {scopes.isPending ? <p role="status">Loading saved scopes…</p> : null}
        {scopes.isError ? <p className="field-error" role="status">Saved scopes are temporarily unavailable.</p> : null}
        {scopeList.length === 0 && scopes.isSuccess ? <p className="muted">No saved scopes for this local installation.</p> : null}
        {scopeList.length ? (
          <ul className="scope-list">
            {scopeList.map((scope) => (
              <li key={scope.id}>
                <div><strong>{scope.name}</strong><small>{scope.context} · {scope.mode} · {scope.namespaces.length} namespaces</small></div>
                <div className="scope-actions">
                  {selection.scopeId === scope.id ? <span className="status-badge status-badge--healthy">active</span> : (
                    <button type="button" className="button button--compact" disabled={!session.data || selectScope.isPending || deleteScope.isPending} onClick={() => selectScope.mutate(scope)}>Select {scope.name}</button>
                  )}
                  <button type="button" className="button button--compact button--secondary" disabled={selectScope.isPending || deleteScope.isPending} onClick={() => {
                    setEditingScopeId(scope.id)
                    setDeleteScopeId(null)
                  }}>Edit {scope.name}</button>
                  <button type="button" className="button button--compact button--danger" disabled={!session.data || selectScope.isPending || deleteScope.isPending} onClick={() => {
                    setDeleteScopeId(scope.id)
                    setReplacementScopeId(null)
                  }}>Delete {scope.name}</button>
                </div>
              </li>
            ))}
          </ul>
        ) : null}
        {selectScope.isError ? <p className="field-error" role="alert">{messageFor(selectScope.error)}</p> : null}
        {deleteScope.isError ? <p className="field-error" role="alert">{messageFor(deleteScope.error)}</p> : null}
        {deleteTarget ? (
          <div className="scope-delete-confirmation" role="alertdialog" aria-labelledby="scope-delete-title">
            <div>
              <strong id="scope-delete-title">Delete “{deleteTarget.name}”?</strong>
              <p>This removes the local scope definition. Kubernetes resources are not modified.</p>
            </div>
            {deletingActive ? (
              <label>Replacement scope
                <select aria-label="Replacement scope" value={replacementScopeId ?? ''} onChange={(event) => setReplacementScopeId(event.target.value ? Number(event.target.value) : null)}>
                  <option value="">Choose before deleting the active scope</option>
                  {replacementCandidates.map((scope) => <option key={scope.id} value={scope.id}>{scope.name}</option>)}
                </select>
              </label>
            ) : null}
            {deletingActive && replacementCandidates.length === 0 ? <p className="field-error">Create another scope before deleting the active scope.</p> : null}
            <div className="form-actions">
              <button type="button" className="button button--secondary" onClick={() => {
                setDeleteScopeId(null)
                setReplacementScopeId(null)
              }} disabled={deleteScope.isPending}>Cancel deletion</button>
              <button type="button" className="button button--danger" disabled={!deletionReady || deleteScope.isPending} onClick={() => deleteScope.mutate({ scope: deleteTarget, replacement: replacementScopeId })}>
                {deleteScope.isPending ? 'Deleting…' : `Confirm delete ${deleteTarget.name}`}
              </button>
            </div>
          </div>
        ) : null}
      </section>
    </div>
  )
}
