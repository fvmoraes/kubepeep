import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import { APIError, getPreferences, getSession, putPreferences } from '../api/client'
import type { Preferences } from '../api/types'
import { StatePanel } from './StatePanel'
import { Button, Card, CardContent, Input, Select } from './ui'

function errorMessage(error: unknown): string {
  return error instanceof APIError ? `${error.code}: ${error.message}` : 'Preferences could not be saved.'
}

export function SettingsPage() {
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal) })

  return (
    <div className="resource-page settings-page">
      <header className="resource-header"><div><span className="eyebrow">local preferences</span><h1>Settings</h1><p>Only allowlisted UI, log, dashboard and saved-filter preferences are stored locally.</p></div></header>
      {preferences.isPending ? <StatePanel kind="loading" title="Loading preferences">Defaults are materialized by the local service.</StatePanel>
        : preferences.isError ? <StatePanel kind="error" title="Preferences unavailable">{errorMessage(preferences.error)}</StatePanel>
          : <SettingsForm initial={preferences.data} />}
    </div>
  )
}

function SettingsForm({ initial }: { initial: Preferences }) {
  const queryClient = useQueryClient()
  const [saved, setSaved] = useState<Preferences>(initial)
  const [draft, setDraft] = useState<Preferences>(() => structuredClone(initial))
  const controllerRef = useRef<AbortController | null>(null)
  useEffect(() => () => controllerRef.current?.abort(), [])
  const save = useMutation({
    mutationFn: async (value: Preferences) => {
      controllerRef.current?.abort()
      const controller = new AbortController()
      controllerRef.current = controller
      try {
        const session = await getSession(controller.signal)
        return await putPreferences(value, session.csrfToken, controller.signal)
      } finally {
        if (controllerRef.current === controller) controllerRef.current = null
      }
    },
    onSuccess: (value) => {
      queryClient.setQueryData(['preferences'], value)
      setSaved(structuredClone(value))
      setDraft(structuredClone(value))
    },
  })

  const tailValid = Number.isInteger(draft.logs.tailLines) && draft.logs.tailLines >= 1 && draft.logs.tailLines <= 2_000

  function update(value: Preferences) {
    save.reset()
    setDraft(value)
  }

  function removeFilter(category: keyof Preferences['filters'], id: string) {
    update({
      ...draft,
      filters: {
        ...draft.filters,
        [category]: { ...draft.filters[category], items: draft.filters[category].items.filter((item) => item.id !== id) },
      },
    })
  }

  return <>
    <section className="settings-grid">
      <Card><CardContent className="grid content-start gap-3 p-4">
        <h2 className="text-lg text-kp-text">Interface</h2>
        <label>Language<Select value={draft.ui.language} onChange={(event) => update({ ...draft, ui: { language: event.target.value as Preferences['ui']['language'] } })}><option value="en">English</option><option value="pt-BR">Português (Brasil)</option></Select></label>
      </CardContent></Card>
      <Card><CardContent className="grid content-start gap-3 p-4">
        <h2 className="text-lg text-kp-text">Logs</h2>
        <label className="confirmation-check"><input type="checkbox" checked={draft.logs.wrap} onChange={(event) => update({ ...draft, logs: { ...draft.logs, wrap: event.target.checked } })} />Wrap long lines</label>
        <label className="confirmation-check"><input type="checkbox" checked={draft.logs.timestamps} onChange={(event) => update({ ...draft, logs: { ...draft.logs, timestamps: event.target.checked } })} />Show timestamps</label>
        <label>Default tail lines<Input aria-invalid={!tailValid} type="number" min="1" max="2000" value={draft.logs.tailLines} onChange={(event) => update({ ...draft, logs: { ...draft.logs, tailLines: Number(event.target.value) } })} /></label>
        {!tailValid ? <p className="field-error">Tail lines must be a whole number from 1 through 2,000.</p> : null}
      </CardContent></Card>
      <Card><CardContent className="grid content-start gap-3 p-4">
        <h2 className="text-lg text-kp-text">Dashboard</h2>
        <label>Log scan window<Select value={draft.dashboard.logScanWindow} onChange={(event) => update({ ...draft, dashboard: { ...draft.dashboard, logScanWindow: event.target.value as Preferences['dashboard']['logScanWindow'] } })}><option value="15m">15 minutes</option><option value="30m">30 minutes</option><option value="1h">1 hour</option><option value="4h">4 hours</option></Select></label>
        <fieldset><legend>Hidden sections</legend>{draft.dashboard.sectionOrder.map((section) => <label className="confirmation-check" key={section}><input type="checkbox" checked={draft.dashboard.hiddenSections.includes(section)} onChange={(event) => update({ ...draft, dashboard: { ...draft.dashboard, hiddenSections: event.target.checked ? [...draft.dashboard.hiddenSections, section] : draft.dashboard.hiddenSections.filter((value) => value !== section) } })} />{section}</label>)}</fieldset>
      </CardContent></Card>
      <Card><CardContent className="saved-filters">
        <h2 className="text-lg text-kp-text">Saved filters</h2>
        <p>Create and apply filters on Workloads, Pods, Events or Logs. Manage removal here. Only schema-limited payloads are stored; pagination cursors and limits are never persisted.</p>
        {(Object.keys(draft.filters) as Array<keyof Preferences['filters']>).map((category) => <section key={category}><h3>{category}</h3>{draft.filters[category].items.length === 0 ? <small>No saved filters.</small> : <ul>{draft.filters[category].items.map((item) => <li key={item.id}><span>{item.name}</span><Button variant="danger" size="compact" onClick={() => removeFilter(category, item.id)}>Remove</Button></li>)}</ul>}</section>)}
      </CardContent></Card>
    </section>
    <div className="form-actions"><Button variant="secondary" onClick={() => update(structuredClone(saved))}>Reset unsaved changes</Button><Button disabled={save.isPending || !tailValid} onClick={() => save.mutate(draft)}>{save.isPending ? 'Saving…' : 'Save settings'}</Button></div>
    {save.isError ? <p className="field-error">{errorMessage(save.error)}</p> : null}{save.isSuccess ? <p className="field-success" role="status">Preferences saved transactionally.</p> : null}
  </>
}
