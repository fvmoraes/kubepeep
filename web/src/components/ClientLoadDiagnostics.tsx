import { useState } from 'react'
import { snapshotUXMetrics } from '../observability/uxMetrics'
import { Button } from './ui'

export function ClientLoadDiagnostics() {
  const [samples, setSamples] = useState(snapshotUXMetrics)
  const rows = [...new Set(samples.map((sample) => sample.view))].flatMap((view) => {
    const requests = samples.filter((sample) => sample.view === view && sample.name === 'time_to_page_complete').map((sample) => sample.value).sort((a, b) => a - b)
    if (!requests.length) return []
    const visible = samples.filter((sample) => sample.view === view && sample.name === 'time_to_first_visible_row').at(-1)?.value
    return [{ view, count: requests.length, p50: requests[Math.ceil(requests.length * 0.5) - 1], p95: requests[Math.ceil(requests.length * 0.95) - 1], visible }]
  })
  return <section id="performance" aria-label="List loading performance" className="grid gap-3 rounded-xl border border-kp-overlay-0 bg-kp-surface-0 p-4">
    <header className="flex flex-wrap items-center justify-between gap-2"><h2 className="m-0 text-heading font-bold">List loading performance</h2><Button variant="secondary" onClick={() => setSamples(snapshotUXMetrics())}>Refresh measurements</Button></header>
    <p className="m-0 text-content text-kp-overlay-text">Measurements from this session. Request time includes the local API and cluster; visible row time includes rendering. Open a resource list, then return here to compare. No resource names or values are recorded.</p>
    {rows.length ? <div className="overflow-auto"><table className="w-full text-left text-content"><thead><tr>{['Screen', 'Requests', 'Request p50', 'Request p95', 'Last visible row'].map((label) => <th className="p-2 text-kp-overlay-text" key={label}>{label}</th>)}</tr></thead><tbody>{rows.map((row) => <tr key={row.view} className="border-t border-kp-divider"><td className="p-2">{row.view}</td><td className="p-2">{row.count}</td>{[row.p50, row.p95, row.visible].map((value, index) => <td className="p-2" key={index}>{value === undefined ? 'No sample' : `${Math.round(value)} ms`}</td>)}</tr>)}</tbody></table></div> : <p className="m-0 text-content text-kp-overlay-text">No list measurements yet.</p>}
  </section>
}
