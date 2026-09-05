import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import { APIError, getPreferences, getSession, putPreferences } from '../api/client'
import type { Preferences } from '../api/types'
import { StatePanel } from './StatePanel'
import { Button, Card, CardContent, Checkbox, Input, PageHeader, Select } from './ui'
import { ErrorBanner, SuccessBanner } from './ui/Banner'

function errorMessage(error: unknown): string {
  return error instanceof APIError ? error.message : 'Preferences could not be saved.'
}

export function SettingsPage() {
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal) })

  return (
    <div className="flex w-full min-w-0 flex-col gap-4">
      <PageHeader title="Settings" description="Only allowlisted UI, log, dashboard and saved-filter preferences are stored locally." />
      {preferences.isPending ? <StatePanel kind="loading" title="Loading preferences">Defaults are materialized by the local service.</StatePanel>
        : preferences.isError ? <StatePanel kind="error" title="Preferences unavailable" details={errorMessage(preferences.error)}>{errorMessage(preferences.error)}</StatePanel>
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
    <section className="grid gap-3 md:grid-cols-2">
      <Card><CardContent className="grid content-start gap-3 p-4">
        <h2 className="m-0 text-xl text-kp-text">Interface</h2>
        <label className="grid gap-1"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Language</span><Select value={draft.ui.language} onChange={(event) => update({ ...draft, ui: { language: event.target.value as Preferences['ui']['language'] } })}><option value="en">English</option><option value="pt-BR">Português (Brasil)</option></Select></label>
      </CardContent></Card>
      <Card><CardContent className="grid content-start gap-2.5 p-4">
        <h2 className="m-0 text-xl text-kp-text">Logs</h2>
        <Checkbox checked={draft.logs.wrap} onChange={(event) => update({ ...draft, logs: { ...draft.logs, wrap: event.target.checked } })}>Wrap long lines</Checkbox>
        <Checkbox checked={draft.logs.timestamps} onChange={(event) => update({ ...draft, logs: { ...draft.logs, timestamps: event.target.checked } })}>Show timestamps</Checkbox>
        <label className="grid gap-1"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Default tail lines</span><Input aria-invalid={!tailValid} type="number" min="1" max="2000" value={draft.logs.tailLines} onChange={(event) => update({ ...draft, logs: { ...draft.logs, tailLines: Number(event.target.value) } })} /></label>
        {!tailValid ? <p className="m-0 text-xs text-kp-red">Tail lines must be a whole number from 1 through 2,000.</p> : null}
      </CardContent></Card>
      <Card><CardContent className="grid content-start gap-2.5 p-4">
        <h2 className="m-0 text-xl text-kp-text">Dashboard</h2>
        <label className="grid gap-1"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Log scan window</span><Select value={draft.dashboard.logScanWindow} onChange={(event) => update({ ...draft, dashboard: { ...draft.dashboard, logScanWindow: event.target.value as Preferences['dashboard']['logScanWindow'] } })}><option value="15m">15 minutes</option><option value="30m">30 minutes</option><option value="1h">1 hour</option><option value="4h">4 hours</option></Select></label>
        <fieldset className="m-0 grid gap-1.5 rounded-lg border border-kp-overlay-0 p-3"><legend className="px-1 text-2xs uppercase tracking-wider text-kp-overlay-text">Hidden sections</legend>{draft.dashboard.sectionOrder.map((section) => <Checkbox key={section} checked={draft.dashboard.hiddenSections.includes(section)} onChange={(event) => update({ ...draft, dashboard: { ...draft.dashboard, hiddenSections: event.target.checked ? [...draft.dashboard.hiddenSections, section] : draft.dashboard.hiddenSections.filter((value) => value !== section) } })}>{section}</Checkbox>)}</fieldset>
      </CardContent></Card>
      <Card><CardContent className="grid content-start gap-2.5 p-4">
        <h2 className="m-0 text-xl text-kp-text">Saved filters</h2>
        <p className="m-0 text-xs leading-relaxed text-kp-overlay-text">Create and apply filters on Workloads, Pods, Events or Logs. Manage removal here. Only schema-limited payloads are stored; pagination cursors and limits are never persisted.</p>
        {(Object.keys(draft.filters) as Array<keyof Preferences['filters']>).map((category) => (
          <section key={category} className="grid gap-1.5 border-t border-kp-overlay-0 pt-2.5">
            <h3 className="m-0 text-xs capitalize text-kp-subtext">{category}</h3>
            {draft.filters[category].items.length === 0 ? <small className="text-xs text-kp-overlay-text">No saved filters.</small> : (
              <ul className="m-0 grid list-none gap-1.5 p-0">
                {draft.filters[category].items.map((item) => (
                  <li key={item.id} className="flex items-center justify-between gap-2 text-xs text-kp-subtext">
                    <span className="truncate">{item.name}</span>
                    <Button variant="danger" size="sm" onClick={() => removeFilter(category, item.id)}>Remove</Button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        ))}
      </CardContent></Card>
    </section>
    <div className="flex flex-wrap justify-end gap-2">
      <Button variant="secondary" onClick={() => update(structuredClone(saved))}>Reset unsaved changes</Button>
      <Button disabled={save.isPending || !tailValid} onClick={() => save.mutate(draft)}>{save.isPending ? 'Saving…' : 'Save settings'}</Button>
    </div>
    {save.isError ? <ErrorBanner>{errorMessage(save.error)}</ErrorBanner> : null}
    {save.isSuccess ? <SuccessBanner>Preferences saved transactionally.</SuccessBanner> : null}
  </>
}
