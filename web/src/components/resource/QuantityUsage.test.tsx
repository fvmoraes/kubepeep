import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import { QuantityUsage, QuotaUsage, WorkloadProgress, quantityNumber } from './QuantityUsage'

afterEach(cleanup)
it.each([['500m', 0.5], ['1Gi', 1073741824], ['512Mi', 536870912], ['1e3', 1000], ['250M', 250000000], ['-1', undefined], ['unknown', undefined]])('normalizes quantity %s without changing its displayed unit', (input, value) => expect(quantityNumber(input as string)).toBe(value))
it.each([[79, 'healthy'], [80, 'warning'], [90, 'warning'], [95, 'danger'], [120, 'danger']])('uses the Pod capacity colors at %s percent', (current, tone) => {
  render(<QuantityUsage label="Capacity" current={current as number} total={100} />)
  const meter = screen.getByRole('meter')
  expect(meter.parentElement).toHaveAttribute('data-tone', tone as string)
  expect(meter).toHaveAttribute('aria-valuetext', `${current} / 100 · ${(current as number).toFixed(1)}%`)
  expect(meter).toHaveAttribute('aria-valuenow', String(Math.min(100, current as number)))
})
it('computes quota ratios across mixed units and leaves missing usage unknown', () => {
  render(<QuotaUsage quota={{ hard: { cpu: '2', memory: '1Gi', pods: '20' }, used: { cpu: '500m', memory: '512Mi' } }} />)
  expect(screen.getByRole('meter', { name: 'Quota cpu' })).toHaveAttribute('aria-valuenow', '25')
  expect(screen.getByRole('meter', { name: 'Quota memory' })).toHaveAttribute('aria-valuenow', '50')
  expect(screen.getByRole('meter', { name: 'Quota pods' })).not.toHaveAttribute('aria-valuenow')
})
it('uses successful completions for Jobs and marks completed work green', () => {
  render(<WorkloadProgress workload={{ kind: 'Job', ready: 0, available: 3, desired: 3, status: 'Completed' }} />)
  expect(screen.getByRole('meter', { name: 'Job completions' })).toHaveAttribute('aria-valuenow', '100')
  expect(screen.getByRole('meter').parentElement).toHaveAttribute('data-tone', 'healthy')
})
