import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { getPods } from '../../api/client'
import type { SelectionSummary, WorkloadDetail } from '../../api/types'
import { WorkloadLogsPanel } from './WorkloadLogsPanel'

vi.mock('../../api/client', async (original) => ({ ...await original<typeof import('../../api/client')>(), getPods: vi.fn() }))
vi.mock('./PodLogsPanel', () => ({ PodLogsPanel: ({ pods }: { pods: { name: string }[] }) => <output aria-label="Stream targets">{pods.map((pod) => pod.name).join(',')}</output> }))
afterEach(() => { cleanup(); vi.clearAllMocks() })

const detail = { kind: 'Deployment', metadata: { namespace: 'payments', name: 'api' } } as WorkloadDetail
const selection = { generation: 'gen_logs' } as SelectionSummary
function show() {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><WorkloadLogsPanel detail={detail} selection={selection} /></QueryClientProvider>)
}

it('reuses only exact related Pod references and bounds stream selection to five Pods', async () => {
  detail.related = [...Array.from({ length: 6 }, (_, index) => ({ kind: 'Pod', namespace: 'payments', name: `pod-${index}` })), { kind: 'Pod', namespace: 'another', name: 'foreign' }, { kind: 'ReplicaSet', namespace: 'payments', name: 'not-a-pod' }]
  show()
  expect(await screen.findByLabelText('Stream targets')).toHaveTextContent('pod-0,pod-1,pod-2,pod-3,pod-4')
  expect(screen.queryByText('not-a-pod')).not.toBeInTheDocument()
  expect(screen.queryByText('foreign')).not.toBeInTheDocument()
  fireEvent.click(screen.getByText('Pods · 5 selected / 6 loaded'))
  expect(screen.getByRole('checkbox', { name: 'pod-5' })).toBeDisabled()
  fireEvent.click(screen.getByRole('checkbox', { name: 'pod-0' }))
  fireEvent.click(screen.getByRole('checkbox', { name: 'pod-5' }))
  expect(screen.getByLabelText('Stream targets')).toHaveTextContent('pod-1,pod-2,pod-3,pod-4,pod-5')
  expect(screen.getByText(/this list may be incomplete/)).toBeInTheDocument()
  expect(getPods).not.toHaveBeenCalled()
})

it('does not discover targets or start streams without authorized Pod references', () => {
  detail.related = []
  show()
  expect(screen.getByText(/No authorized Pod references/)).toBeInTheDocument()
  expect(getPods).not.toHaveBeenCalled()
  expect(screen.queryByLabelText('Stream targets')).not.toBeInTheDocument()
})
