import { afterEach, describe, expect, it } from 'vitest'

import {
  associateListRequestRows,
  beginListInteraction,
  beginListRequest,
  bindListInteraction,
  cancelListRequest,
  completeListRequest,
  currentUXView,
  listInteractionFor,
  recordFirstRowRendered,
  recordRenderedRowCount,
  resetUXMetrics,
  snapshotUXMetrics,
} from './uxMetrics'

afterEach(resetUXMetrics)

function completeRows(view = 'pods') {
  const requestId = beginListRequest({ view })
  const rows = [{ name: 'redacted' }]
  associateListRequestRows(requestId, rows)
  completeListRequest(requestId, true)
  recordFirstRowRendered(rows)
  return { requestId, rows }
}

describe('UX metrics', () => {
  it('records bounded request, committed-row and interaction timings without resource identity', () => {
    const interactionId = beginListInteraction('filter', 'pods')
    const requestId = beginListRequest({ view: 'pods', interactionId })
    const rows = [{ name: 'private-pod' }]
    associateListRequestRows(requestId, rows)
    completeListRequest(requestId, true)
    recordFirstRowRendered(rows)
    recordRenderedRowCount(100, 'pods')

    const snapshot = snapshotUXMetrics()
    expect(snapshot.map((sample) => sample.name)).toEqual([
      'time_to_page_complete',
      'filter_interaction_latency',
      'time_to_first_row',
      'rendered_row_count',
    ])
    expect(snapshot.every((sample) => sample.view === 'pods' && sample.value >= 0)).toBe(true)
    expect(JSON.stringify(snapshot)).not.toContain('namespace')
    expect(JSON.stringify(snapshot)).not.toContain('private-pod')
    expect(JSON.stringify(snapshot)).not.toContain(requestId)
    expect(JSON.stringify(snapshot)).not.toContain(interactionId)
  })

  it('attaches an interaction out-of-band to the exact applied query state', () => {
    const interactionId = beginListInteraction('filter', 'pods')
    const priorState = { search: '' }
    const appliedState = bindListInteraction({ search: 'running' }, interactionId)

    expect(listInteractionFor(priorState)).toBeUndefined()
    expect(listInteractionFor(appliedState)).toBe(interactionId)
    expect(JSON.stringify(appliedState)).toBe('{"search":"running"}')
  })

  it('does not create duplicate request or first-row samples when cached rows commit again', () => {
    const { rows } = completeRows()
    recordFirstRowRendered(rows)

    const names = snapshotUXMetrics().map((sample) => sample.name)
    expect(names.filter((name) => name === 'time_to_page_complete')).toHaveLength(1)
    expect(names.filter((name) => name === 'time_to_first_row')).toHaveLength(1)
  })

  it('tracks a background refetch independently even when the row count is unchanged', () => {
    completeRows()
    completeRows()

    const names = snapshotUXMetrics().map((sample) => sample.name)
    expect(names.filter((name) => name === 'time_to_page_complete')).toHaveLength(2)
    expect(names.filter((name) => name === 'time_to_first_row')).toHaveLength(2)
  })

  it('reattaches an explicit interaction token to the successful retry after a failed attempt', () => {
    const interactionId = beginListInteraction('sort', 'pods')
    const failedRequest = beginListRequest({ view: 'pods', interactionId })
    cancelListRequest(failedRequest)
    const retriedRequest = beginListRequest({ view: 'pods', interactionId })
    const rows = [{ name: 'redacted' }]
    associateListRequestRows(retriedRequest, rows)
    completeListRequest(retriedRequest, true)
    recordFirstRowRendered(rows)

    const names = snapshotUXMetrics().map((sample) => sample.name)
    expect(names.filter((name) => name === 'sort_interaction_latency')).toHaveLength(1)
    expect(names.filter((name) => name === 'time_to_page_complete')).toHaveLength(1)
    expect(names.filter((name) => name === 'time_to_first_row')).toHaveLength(1)
  })

  it('does not let an unrelated concurrent request in the same view capture the interaction', () => {
    const interactionId = beginListInteraction('filter', 'pods')
    const unrelated = beginListRequest({ view: 'pods' })
    const intended = beginListRequest({ view: 'pods', interactionId })
    const unrelatedRows = [{ ordinal: 1 }]
    const intendedRows = [{ ordinal: 2 }]
    associateListRequestRows(unrelated, unrelatedRows)
    associateListRequestRows(intended, intendedRows)

    completeListRequest(unrelated, true)
    expect(snapshotUXMetrics().filter((sample) => sample.name === 'filter_interaction_latency')).toHaveLength(0)
    completeListRequest(intended, true)
    recordFirstRowRendered(unrelatedRows)
    recordFirstRowRendered(intendedRows)

    const samples = snapshotUXMetrics()
    expect(samples.filter((sample) => sample.name === 'filter_interaction_latency')).toHaveLength(1)
    expect(samples.filter((sample) => sample.name === 'time_to_page_complete')).toHaveLength(2)
    expect(samples.filter((sample) => sample.name === 'time_to_first_row')).toHaveLength(2)
    expect(samples.every((sample) => sample.view === 'pods')).toBe(true)
  })

  it('maps paths to a closed view vocabulary and bounds retained samples', () => {
    expect(currentUXView('/')).toBe('overview')
    expect(currentUXView('/pods/private-namespace/private-pod')).toBe('pods')
    expect(currentUXView('/unknown/private')).toBe('other')
    for (let index = 0; index < 300; index += 1) recordRenderedRowCount(index, 'pods')
    expect(snapshotUXMetrics()).toHaveLength(256)
    expect(window.__KUBEPEEP_UX_METRICS__?.snapshot()).toHaveLength(256)
  })
})
