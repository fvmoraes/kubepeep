import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import { APIError, getDiagnostics, getPreferences, getStatus } from '../api/client'
import { mutatePreferences } from '../api/preferences'
import type { Preferences } from '../api/types'
import { StatePanel } from './StatePanel'
import { Button, Card, CardContent, Checkbox, Input, PageHeader, Select } from './ui'
import { ErrorBanner, SuccessBanner, WarningBanner } from './ui/Banner'

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
		<PerformanceDiagnostics />
    </div>
  )
}

function formatBytes(value: number) {
	if (value < 1_024) return `${value} B`
	if (value < 1_024 * 1_024) return `${(value / 1_024).toFixed(1)} KiB`
	return `${(value / (1_024 * 1_024)).toFixed(1)} MiB`
}

function PerformanceDiagnostics() {
	const status = useQuery({ queryKey: ['local-status'], queryFn: ({ signal }) => getStatus(signal), staleTime: 15_000 })
	const generation = status.data?.selection?.generation
	const diagnostics = useQuery({ queryKey: ['diagnostics', generation], queryFn: ({ signal }) => getDiagnostics(signal, generation), enabled: Boolean(generation), staleTime: 5_000, retry: false })
	if (!generation) return <StatePanel kind="empty" title="Diagnostics require an active scope">Select a context and default namespace scope to inspect runtime performance.</StatePanel>
	if (diagnostics.isPending) return <StatePanel kind="loading" title="Collecting diagnostics">Reading bounded process metrics and the current local resource index.</StatePanel>
	if (diagnostics.isError) return <StatePanel kind="error" title="Diagnostics unavailable">{errorMessage(diagnostics.error)}</StatePanel>
	const value = diagnostics.data
	const latency = value.performance.apiServerLatency
	return <section aria-labelledby="performance-diagnostics-title" className="grid gap-3 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-4">
		<header><p className="text-2xs uppercase tracking-widest text-kp-mauve">Diagnostics</p><h2 id="performance-diagnostics-title" className="mt-1 text-xl text-kp-text">Performance</h2><p className="mt-1 text-xs text-kp-overlay-text">Process-local metrics contain bounded resource categories only. Namespace timing is returned in this response and is not retained as telemetry.</p></header>
		{!value.complete ? <WarningBanner>Coverage is partial. Missing or forbidden cache sections stay unknown and are not displayed as zero.</WarningBanner> : null}
		<div className="grid gap-px overflow-hidden rounded-lg border border-kp-overlay-0 bg-kp-overlay-0 sm:grid-cols-3 lg:grid-cols-6">
			{[
				['API p50', `${latency.p50Milliseconds} ms`], ['API p95', `${latency.p95Milliseconds} ms`], ['API p99', `${latency.p99Milliseconds} ms`],
				['Cache hit', value.performance.cacheHitRatio === null ? 'unknown' : `${(value.performance.cacheHitRatio * 100).toFixed(1)}%`],
				['Active watches', String(value.performance.activeWatches)], ['429 responses', String(value.performance.responses429)],
			].map(([label, metric]) => <div key={label} className="bg-kp-surface-1 p-3"><small className="block text-2xs uppercase tracking-wider text-kp-overlay-text">{label}</small><strong className="mt-1 block text-lg text-kp-text">{metric}</strong></div>)}
		</div>
		<div className="grid gap-3 lg:grid-cols-2">
			<div className="overflow-auto rounded-lg border border-kp-overlay-0"><table className="w-full text-left text-xs"><caption className="p-3 text-left text-sm font-semibold text-kp-text">Resource sync latency</caption><thead className="bg-kp-surface-1 text-kp-overlay-text"><tr><th className="p-2">Resource</th><th className="p-2">p50</th><th className="p-2">p95</th><th className="p-2">p99</th></tr></thead><tbody>{value.performance.resourceSync.map((item) => <tr key={item.resource} className="border-t border-kp-overlay-0"><td className="p-2 text-kp-text">{item.resource}</td><td className="p-2">{item.latency.p50Milliseconds} ms</td><td className="p-2">{item.latency.p95Milliseconds} ms</td><td className="p-2">{item.latency.p99Milliseconds} ms</td></tr>)}</tbody></table></div>
			<div className="rounded-lg border border-kp-overlay-0 p-3"><h3 className="text-sm text-kp-text">Cluster and KubePeep</h3><dl className="mt-2 grid grid-cols-2 gap-2 text-xs"><dt className="text-kp-overlay-text">Kubernetes</dt><dd>{value.cluster.kubernetesVersion ?? 'unknown'}</dd><dt className="text-kp-overlay-text">Namespaces in scope</dt><dd>{value.cluster.namespaces}</dd><dt className="text-kp-overlay-text">Loaded totals</dt><dd>{Object.entries(value.cluster.totals).map(([kind, count]) => `${kind} ${count}`).join(' · ') || 'not synchronized'}</dd><dt className="text-kp-overlay-text">Requests/min</dt><dd>{value.performance.requestsPerMinute.toFixed(1)}</dd><dt className="text-kp-overlay-text">Resource cache</dt><dd>{value.cluster.kubepeep.resourceCacheEntries} · {formatBytes(value.cluster.kubepeep.resourceCacheBytes)}</dd><dt className="text-kp-overlay-text">Page cache</dt><dd>{value.cluster.kubepeep.collectionCacheEntries} · {formatBytes(value.cluster.kubepeep.collectionCacheBytes)}</dd><dt className="text-kp-overlay-text">Cursor memory</dt><dd>{formatBytes(value.cluster.kubepeep.cursorBytes)}</dd></dl></div>
		</div>
		<div className="overflow-auto rounded-lg border border-kp-overlay-0"><table className="w-full text-left text-xs"><caption className="p-3 text-left text-sm font-semibold text-kp-text">Namespace diagnostics · slowest measured first</caption><thead className="bg-kp-surface-1 text-kp-overlay-text"><tr><th className="p-2">Namespace</th><th className="p-2">Resources loaded</th><th className="p-2">Problems</th><th className="p-2">Restarts</th><th className="p-2">LIST latency</th></tr></thead><tbody>{value.namespaces.map((item) => <tr key={item.namespace} className="border-t border-kp-overlay-0"><td className="p-2 text-kp-text">{item.namespace}</td><td className="p-2">{Object.entries(item.resources).map(([kind,count]) => `${kind} ${count}`).join(' · ') || 'not synchronized'}</td><td className="p-2">{item.problems.critical} critical · {item.problems.warning} warning · {item.problems.info} info</td><td className="p-2">{item.restarts ?? 'unknown'}</td><td className="p-2">{item.listLatencyMilliseconds === null ? 'unknown' : `${item.listLatencyMilliseconds} ms`}</td></tr>)}</tbody></table></div>
		<p className="text-xs text-kp-overlay-text">Cache coverage: {value.cacheCoverage.map((item) => `${item.topic} ${item.complete ? item.state.toLowerCase() : 'incomplete'}`).join(' · ')}</p>
	</section>
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
        return await mutatePreferences((current) => {
          const next = structuredClone(current)
          // Settings owns UI/log/dashboard values. Filter management only
          // removes IDs explicitly deleted from this draft, preserving filters
          // that another writer appended after the form was opened.
          next.ui = structuredClone(value.ui)
          next.logs = structuredClone(value.logs)
          next.dashboard = structuredClone(value.dashboard)
          next.filters = structuredClone(current.filters)
          for (const category of Object.keys(value.filters) as Array<keyof Preferences['filters']>) {
            const desiredIDs = new Set(value.filters[category].items.map((item) => item.id))
            const removedIDs = new Set(saved.filters[category].items.filter((item) => !desiredIDs.has(item.id)).map((item) => item.id))
            if (removedIDs.size === 0) continue
            next.filters[category] = {
              ...current.filters[category],
              items: current.filters[category].items.filter((item) => !removedIDs.has(item.id)),
            }
          }
          return next
        }, { signal: controller.signal })
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
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved)

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
    <fieldset disabled={save.isPending} aria-busy={save.isPending} className="contents">
      <legend className="sr-only">Editable settings</legend>
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
    </fieldset>
    <div className="flex flex-wrap justify-end gap-2">
      <Button
        variant="secondary"
        disabled={save.isPending || !dirty}
        disabledReason={save.isPending ? 'Settings are currently being saved.' : 'There are no unsaved settings to reset.'}
        onClick={() => update(structuredClone(saved))}
      >Reset unsaved changes</Button>
      <Button
        disabled={save.isPending || !tailValid || !dirty}
        disabledReason={save.isPending ? 'Settings are currently being saved.' : !tailValid ? 'Fix the default tail-line value before saving.' : 'There are no unsaved settings to save.'}
        onClick={() => save.mutate(draft)}
      >{save.isPending ? 'Saving…' : 'Save settings'}</Button>
    </div>
    {save.isError ? <ErrorBanner>{errorMessage(save.error)}</ErrorBanner> : null}
    {save.isSuccess ? <SuccessBanner>Preferences saved transactionally.</SuccessBanner> : null}
  </>
}
