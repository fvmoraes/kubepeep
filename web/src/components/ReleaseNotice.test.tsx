import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { ReleaseNotice } from './ReleaseNotice'

afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals() })
it('checks in the background every two hours and links updates to the official download page', async () => {
  vi.useFakeTimers()
  const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: { currentVersion: '0.11.0', latestVersion: '0.12.0', available: true } }), { headers: { 'Content-Type': 'application/json' } }))
  // Every fetch needs a new response body.
  fetch.mockImplementation(async () => new Response(JSON.stringify({ data: { currentVersion: '0.11.0', latestVersion: '0.12.0', available: true } }), { headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', fetch)
  const client = new QueryClient()
  render(<QueryClientProvider client={client}><ReleaseNotice /></QueryClientProvider>)
  await act(async () => { await vi.advanceTimersByTimeAsync(10) })
  expect(screen.getByRole('link', { name: /0.12.0 available/ })).toHaveAttribute('href', 'https://kubepeep.online/download')
  expect(fetch).toHaveBeenCalledTimes(1)
  await act(async () => { await vi.advanceTimersByTimeAsync(2 * 60 * 60 * 1000 - 20) })
  expect(fetch).toHaveBeenCalledTimes(1)
  await act(async () => { await vi.advanceTimersByTimeAsync(20) })
  expect(fetch).toHaveBeenCalledTimes(2)
  client.clear()
})

it.each([false, 'offline'])('keeps the footer quiet when no update is available (%s)', async available => {
  const fetch = vi.fn(async () => { if (available === 'offline') throw new Error('offline'); return new Response(JSON.stringify({ data: { available: false } }), { headers: { 'Content-Type': 'application/json' } }) })
  vi.stubGlobal('fetch', fetch)
  const client = new QueryClient()
  render(<QueryClientProvider client={client}><ReleaseNotice /></QueryClientProvider>)
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 20)) })
  expect(screen.queryByRole('contentinfo')).not.toBeInTheDocument()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(fetch).toHaveBeenCalledTimes(1)
  client.clear()
})

it('opens the download page in the system browser from the desktop app', () => {
  const BrowserOpenURL = vi.fn()
  vi.stubGlobal('runtime', { BrowserOpenURL })
  const client = new QueryClient()
  client.setQueryData(['release-update'], { currentVersion: '0.11.0', latestVersion: '0.12.0', available: true })
  render(<QueryClientProvider client={client}><ReleaseNotice /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('link', { name: /0.12.0 available/ }))
  expect(BrowserOpenURL).toHaveBeenCalledWith('https://kubepeep.online/download')
  client.clear()
})
