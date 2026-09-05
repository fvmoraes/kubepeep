import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'

import {
  APIError,
  createExecTicket,
  createIdempotencyKey,
  createPortForward,
  deletePod,
  getPermissions,
  getSession,
  restartWorkload,
  scaleWorkload,
} from '../api/client'
import { streamURL } from '../api/desktop'
import { lazy, Suspense } from 'react'
import type { ExecTerminalHandle } from './ExecTerminal'

const ExecTerminal = lazy(() => import('./ExecTerminal'))
import type {
  CapabilityDecision,
  CapabilityMatrix,
  ExecTicket,
  PodDetail,
  SelectionSummary,
  WorkloadDetail,
} from '../api/types'
import { Badge, Button, Checkbox, Input, Select } from './ui'

function mutationError(error: unknown): string {
  if (error instanceof APIError) return `${error.code}: ${error.message}`
  return error instanceof Error ? error.message : 'The action could not be completed.'
}

function isAuthorizationFailure(error: unknown): boolean {
  return error instanceof APIError && error.status === 403
}

function target(selection: SelectionSummary, namespace: string, kind: 'Deployment' | 'StatefulSet' | 'Pod', name: string) {
  return {
    clusterProfileId: selection.clusterProfileId,
    context: selection.context,
    namespace,
    kind,
    name,
  } as const
}

function kindPath(kind: WorkloadDetail['kind']): string {
  const paths: Record<WorkloadDetail['kind'], string> = {
    Deployment: 'deployments',
    StatefulSet: 'statefulsets',
    DaemonSet: 'daemonsets',
    Job: 'jobs',
    CronJob: 'cronjobs',
    ReplicaSet: 'replicasets',
  }
  return paths[kind]
}

function decision(matrix: CapabilityMatrix | undefined, capabilityID: string): CapabilityDecision | 'loading' {
  if (!matrix) return 'loading'
  return matrix.decisions.find((value) => value.capabilityId === capabilityID)?.decision ?? 'unknown'
}

function CapabilityNotice({ label, value }: { label: string; value: CapabilityDecision | 'loading' }) {
  const copy = value === 'allowed'
    ? 'allowed (the backend will recheck immediately before execution)'
    : value === 'denied'
      ? 'denied by Kubernetes'
      : value === 'unknown'
        ? 'could not be verified; action disabled'
        : 'checking Kubernetes permission'
  const variant = value === 'allowed' ? 'healthy' : value === 'denied' ? 'danger' : value === 'unknown' ? 'warning' : 'unknown'
  return <li className="flex items-center gap-1.5 text-xs"><strong className="text-kp-subtext">{label}:</strong> <Badge variant={variant}>{copy}</Badge></li>
}

function useGenerationRequests(generation: string) {
  const active = useRef(new Set<AbortController>())

  useEffect(() => () => {
    for (const controller of active.current) controller.abort()
    active.current.clear()
  }, [generation])

  return useCallback(function run<T>(operation: (signal: AbortSignal) => Promise<T>): Promise<T> {
    const controller = new AbortController()
    active.current.add(controller)
    return operation(controller.signal).finally(() => active.current.delete(controller))
  }, [])
}

async function csrfForGeneration(generation: string, signal: AbortSignal): Promise<string> {
  const session = await getSession(signal)
  if (session.generation !== generation) {
    throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed before the action.' })
  }
  return session.csrfToken
}

export function WorkloadActions({ detail, selection }: { detail: WorkloadDetail; selection: SelectionSummary }) {
  const queryClient = useQueryClient()
  const runRequest = useGenerationRequests(selection.generation)
  const [confirmed, setConfirmed] = useState(false)
  const [replicas, setReplicas] = useState(detail.desired ?? 1)
  const supportedKind = detail.kind === 'Deployment' || detail.kind === 'StatefulSet'
  const actionTarget = detail.kind === 'Deployment' || detail.kind === 'StatefulSet'
    ? target(selection, detail.metadata.namespace, detail.kind, detail.metadata.name)
    : null
  const capabilityIDs = detail.kind === 'Deployment'
    ? ['deployments.restart', 'deployments.scale']
    : detail.kind === 'StatefulSet'
      ? ['statefulsets.scale']
      : []
  const permissions = useQuery({
    queryKey: ['action-permissions', selection.generation, detail.metadata.namespace, detail.metadata.name, ...capabilityIDs],
    queryFn: ({ signal }) => getPermissions({
      namespaces: [detail.metadata.namespace],
      capabilityIds: capabilityIDs,
      resourceNames: [detail.metadata.name],
    }, signal, selection.generation),
    enabled: supportedKind,
    staleTime: 15_000,
  })
  const restartDecision = decision(permissions.data, 'deployments.restart')
  const scaleDecision = decision(permissions.data, `${detail.kind === 'StatefulSet' ? 'statefulsets' : 'deployments'}.scale`)
  const invalidateActionPermissions = () => queryClient.invalidateQueries({ queryKey: ['action-permissions', selection.generation] })

  const restart = useMutation({
    mutationFn: () => runRequest(async (signal) => {
      if (!actionTarget || detail.kind !== 'Deployment') throw new Error('Restart is not supported for this workload kind.')
      const csrfToken = await csrfForGeneration(selection.generation, signal)
      return restartWorkload(kindPath(detail.kind), detail.metadata.namespace, detail.metadata.name, {
        confirmed: true,
        action: 'restart',
        consequenceCode: 'RECREATE_WORKLOAD_PODS',
        target: actionTarget,
        expectedGeneration: selection.generation,
        expectedResourceVersion: detail.metadata.resourceVersion,
      }, csrfToken, createIdempotencyKey(), signal)
    }),
    onSuccess: () => Promise.all([
      queryClient.invalidateQueries({ queryKey: ['resources'] }),
      invalidateActionPermissions(),
    ]),
    onError: (error) => {
      if (isAuthorizationFailure(error)) void invalidateActionPermissions()
    },
  })

  const scale = useMutation({
    mutationFn: () => runRequest(async (signal) => {
      if (!actionTarget) throw new Error('Scale is not supported for this workload kind.')
      const csrfToken = await csrfForGeneration(selection.generation, signal)
      return scaleWorkload(kindPath(detail.kind), detail.metadata.namespace, detail.metadata.name, {
        replicas,
        confirmed: true,
        action: 'scale',
        consequenceCode: 'CHANGE_REPLICA_COUNT',
        target: actionTarget,
        expectedGeneration: selection.generation,
        expectedResourceVersion: detail.metadata.resourceVersion,
      }, csrfToken, signal)
    }),
    onSuccess: () => Promise.all([
      queryClient.invalidateQueries({ queryKey: ['resources'] }),
      invalidateActionPermissions(),
    ]),
    onError: (error) => {
      if (isAuthorizationFailure(error)) void invalidateActionPermissions()
    },
  })

  if (!supportedKind) return null

  const replicasValid = Number.isInteger(replicas) && replicas >= 0 && replicas <= 10_000

  return (
    <section className="mt-3 grid gap-2.5 rounded-xl border border-kp-accent-border bg-kp-accent-bg/40 p-3" aria-label="Authorized workload actions">
      <h3 className="m-0 text-sm text-kp-text">Authorized actions</h3>
      <p className="m-0 text-xs leading-relaxed text-kp-overlay-text">These capability hints never replace the Kubernetes authorization check performed when the action runs.</p>
      <ul className="m-0 grid list-none gap-1 p-0" aria-label="Workload action permissions">
        {detail.kind === 'Deployment' ? <CapabilityNotice label="Restart" value={restartDecision} /> : null}
        <CapabilityNotice label="Scale" value={scaleDecision} />
      </ul>
      {permissions.isError ? <p className="m-0 text-xs text-kp-red">Permission check failed; actions remain disabled.</p> : null}
      <Checkbox checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)}>
        I checked {detail.kind} {detail.metadata.namespace}/{detail.metadata.name} and understand the consequence.
      </Checkbox>
      <div className="flex flex-wrap items-end gap-2">
        {detail.kind === 'Deployment' ? (
          <Button variant="danger" disabled={!confirmed || restartDecision !== 'allowed' || restart.isPending} onClick={() => restart.mutate()}>
            {restart.isPending ? 'Requesting restart…' : 'Restart Deployment'}
          </Button>
        ) : null}
        <label className="grid gap-1 w-28">
          <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Replicas</span>
          <Input aria-invalid={!replicasValid} type="number" min="0" max="10000" value={replicas} onChange={(event) => setReplicas(Number(event.target.value))} />
        </label>
        <Button variant="secondary" disabled={!confirmed || scaleDecision !== 'allowed' || scale.isPending || !replicasValid} onClick={() => scale.mutate()}>
          {scale.isPending ? 'Scaling…' : 'Scale'}
        </Button>
      </div>
      {!replicasValid ? <p className="m-0 text-xs text-kp-red">Replicas must be a whole number from 0 through 10,000.</p> : null}
      {restart.isError ? <p className="m-0 text-xs text-kp-red">{mutationError(restart.error)}</p> : null}
      {scale.isError ? <p className="m-0 text-xs text-kp-red">{mutationError(scale.error)}</p> : null}
      {restart.isSuccess ? <p className="m-0 text-xs text-kp-green" role="status">Restart accepted; rollout completion is not implied.</p> : null}
      {scale.isSuccess ? <p className="m-0 text-xs text-kp-green" role="status">Replica update accepted; Pod readiness is checked separately.</p> : null}
    </section>
  )
}

interface TerminalLine {
  stream: 'status' | 'stdout' | 'stderr'
  text: string
}

type ExecSocketState = 'closed' | 'connecting' | 'ready'

interface ExecControl {
  type?: string
  nonce?: string
  code?: string
  message?: string
  exitCode?: number | null
  reason?: string
}

function appendTerminalBounded(lines: TerminalLine[], line: TerminalLine): TerminalLine[] {
  const encoder = new TextEncoder()
  const next = [...lines, line]
  let bytes = next.reduce((total, value) => total + encoder.encode(value.text).byteLength + 1, 0)
  while (next.length > 1_000 || bytes > 1 << 20) {
    const removed = next.shift()
    if (!removed) break
    bytes -= encoder.encode(removed.text).byteLength + 1
  }
  return next
}

export function PodActions({ detail, selection }: { detail: PodDetail; selection: SelectionSummary }) {
  const queryClient = useQueryClient()
  const runRequest = useGenerationRequests(selection.generation)
  const [confirmed, setConfirmed] = useState(false)
  const [remotePort, setRemotePort] = useState(8080)
  const [localPort, setLocalPort] = useState('')
  const [container, setContainer] = useState(detail.containers[0]?.spec.name ?? '')
  const [command, setCommand] = useState('/bin/sh')
  const [terminal, setTerminal] = useState<TerminalLine[]>([])
  const [socketState, setSocketState] = useState<ExecSocketState>('closed')
  const socketRef = useRef<WebSocket | null>(null)
  const execTerminalRef = useRef<ExecTerminalHandle | null>(null)
  const actionTarget = target(selection, detail.metadata.namespace, 'Pod', detail.metadata.name)
  const permissions = useQuery({
    queryKey: ['action-permissions', selection.generation, detail.metadata.namespace, detail.metadata.name, 'pod-actions'],
    queryFn: ({ signal }) => getPermissions({
      namespaces: [detail.metadata.namespace],
      capabilityIds: ['pods.delete', 'pods.exec.create', 'pods.portforward.create'],
      resourceNames: [detail.metadata.name],
    }, signal, selection.generation),
    staleTime: 15_000,
  })
  const deleteDecision = decision(permissions.data, 'pods.delete')
  const execDecision = decision(permissions.data, 'pods.exec.create')
  const portForwardDecision = decision(permissions.data, 'pods.portforward.create')
  const invalidateActionPermissions = () => queryClient.invalidateQueries({ queryKey: ['action-permissions', selection.generation] })

  useEffect(() => () => {
    const socket = socketRef.current
    socketRef.current = null
    if (socket) {
      socket.onmessage = null
      socket.onerror = null
      socket.onclose = null
      socket.close(1000, 'page_closed')
    }
  }, [selection.generation])

  function appendTerminal(line: TerminalLine) {
    setTerminal((lines) => appendTerminalBounded(lines, line))
  }

  function appendOutput(stream: 'stdout' | 'stderr', text: string) {
    if (stream === 'stderr') execTerminalRef.current?.writeStderr(text)
    else execTerminalRef.current?.writeStdout(text)
  }

  async function openExecSocket(ticket: ExecTicket) {
    const previous = socketRef.current
    if (previous) {
      previous.onmessage = null
      previous.onerror = null
      previous.onclose = null
      previous.close(1000, 'replaced')
    }
    const socket = new WebSocket(await streamURL(ticket.websocketUrl), ticket.protocols)
    socket.binaryType = 'arraybuffer'
    socketRef.current = socket
    setSocketState('connecting')
    setTerminal([{ stream: 'status', text: 'Connecting to the authorized container…' }])
    socket.onmessage = (event) => {
      if (socketRef.current !== socket) return
      if (typeof event.data === 'string') {
        let control: ExecControl | null = null
        try { control = JSON.parse(event.data) as ExecControl } catch { control = null }
        if (control?.type === 'heartbeat' && typeof control.nonce === 'string') {
          socket.send(event.data)
          return
        }
        if (control?.type === 'ready') {
          setSocketState('ready')
          appendTerminal({ stream: 'status', text: 'Exec session ready.' })
          return
        }
        if (control?.type === 'exit') {
          appendTerminal({ stream: 'status', text: `Process exited (${control.exitCode ?? 'no code'}): ${control.reason ?? 'completed'}.` })
          return
        }
        if (control?.type === 'error') {
          appendTerminal({ stream: 'status', text: `${control.code ?? 'EXEC_ERROR'}: ${control.message ?? 'The exec session ended.'}` })
          return
        }
        appendTerminal({ stream: 'status', text: 'The exec server sent an unsupported control message.' })
        return
      }
      const bytes = new Uint8Array(event.data as ArrayBuffer)
      if (bytes.length < 2) return
      if (bytes[0] !== 1 && bytes[0] !== 2) {
        appendTerminal({ stream: 'status', text: 'The exec server sent an unsupported data stream.' })
        return
      }
      appendOutput(bytes[0] === 2 ? 'stderr' : 'stdout', new TextDecoder().decode(bytes.subarray(1)))
    }
    socket.onerror = () => {
      if (socketRef.current === socket) appendTerminal({ stream: 'status', text: 'The exec stream failed.' })
    }
    socket.onclose = () => {
      if (socketRef.current !== socket) return
      socketRef.current = null
      setSocketState('closed')
      appendTerminal({ stream: 'status', text: 'Session closed.' })
    }
  }

  const remove = useMutation({
    mutationFn: () => runRequest(async (signal) => {
      const csrfToken = await csrfForGeneration(selection.generation, signal)
      return deletePod(detail.metadata.namespace, detail.metadata.name, {
        confirmed: true,
        action: 'deletePod',
        consequenceCode: 'DELETE_POD',
        target: actionTarget,
        expectedGeneration: selection.generation,
        expectedUid: detail.metadata.uid,
        expectedResourceVersion: detail.metadata.resourceVersion,
      }, csrfToken, signal)
    }),
    onSuccess: () => Promise.all([
      queryClient.invalidateQueries({ queryKey: ['resources'] }),
      invalidateActionPermissions(),
    ]),
    onError: (error) => {
      if (isAuthorizationFailure(error)) void invalidateActionPermissions()
    },
  })

  const portForward = useMutation({
    mutationFn: () => runRequest(async (signal) => {
      const csrfToken = await csrfForGeneration(selection.generation, signal)
      return createPortForward(detail.metadata.namespace, detail.metadata.name, {
        remotePort,
        localPort: localPort === '' ? null : Number(localPort),
        confirmed: true,
        action: 'portForward',
        consequenceCode: 'EXPOSE_POD_PORT_LOCALLY',
        target: actionTarget,
        expectedGeneration: selection.generation,
      }, csrfToken, createIdempotencyKey(), signal)
    }),
    onSuccess: () => Promise.all([
      queryClient.invalidateQueries({ queryKey: ['port-forwards'] }),
      invalidateActionPermissions(),
    ]),
    onError: (error) => {
      if (isAuthorizationFailure(error)) void invalidateActionPermissions()
    },
  })

  const exec = useMutation({
    mutationFn: () => runRequest(async (signal) => {
      const argv = command.split('\n').map((value) => value.trim()).filter(Boolean)
      const csrfToken = await csrfForGeneration(selection.generation, signal)
      const ticket = await createExecTicket(detail.metadata.namespace, detail.metadata.name, {
        container,
        command: argv,
        tty: true,
        stdin: true,
        confirmed: true,
        action: 'exec',
        consequenceCode: 'OPEN_INTERACTIVE_PROCESS',
        target: actionTarget,
        expectedGeneration: selection.generation,
      }, csrfToken, signal)
      if (!signal.aborted) await openExecSocket(ticket)
    }),
    onSuccess: invalidateActionPermissions,
    onError: (error) => {
      if (isAuthorizationFailure(error)) void invalidateActionPermissions()
    },
  })

  const argv = command.split('\n').map((value) => value.trim()).filter(Boolean)
  const argvBytes = argv.reduce((total, value) => total + new TextEncoder().encode(value).byteLength, 0)
  const commandValid = argv.length >= 1 && argv.length <= 64 && argvBytes <= 32 * 1024 && argv.every((value) => !value.includes('\0') && new TextEncoder().encode(value).byteLength <= 4_096)
  const remotePortValid = Number.isInteger(remotePort) && remotePort >= 1 && remotePort <= 65_535
  const parsedLocalPort = localPort === '' ? null : Number(localPort)
  const localPortValid = parsedLocalPort === null || (Number.isInteger(parsedLocalPort) && parsedLocalPort >= 1_024 && parsedLocalPort <= 65_535)

  function sendRawStdin(text: string) {
    const socket = socketRef.current
    if (!socket || socketState !== 'ready' || socket.readyState !== WebSocket.OPEN || text === '') return
    const payload = new TextEncoder().encode(text)
    const frame = new Uint8Array(payload.length + 1)
    frame[0] = 0
    frame.set(payload, 1)
    socket.send(frame)
  }

  function handleTerminalResize(nextColumns: number, nextRows: number) {
    const socket = socketRef.current
    if (!socket || socketState !== 'ready' || socket.readyState !== WebSocket.OPEN) return
    socket.send(JSON.stringify({ type: 'resize', columns: nextColumns, rows: nextRows }))
  }

  function closeExec() {
    const socket = socketRef.current
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(JSON.stringify({ type: 'close', stream: 'session' }))
  }

  return (
    <section className="mt-3 grid gap-2.5 rounded-xl border border-kp-accent-border bg-kp-accent-bg/40 p-3" aria-label="Authorized pod actions">
      <h3 className="m-0 text-sm text-kp-text">Authorized actions</h3>
      <p className="m-0 text-xs leading-relaxed text-kp-overlay-text">Delete is destructive. Port-forward binds only to loopback. Exec argv is one item per line and is never joined through a shell.</p>
      <p className="m-0 rounded-r-md border-l-2 border-kp-yellow-border bg-kp-yellow-bg px-2.5 py-1.5 text-xs text-kp-yellow">Owner: {detail.summary.owner ? `${detail.summary.owner.kind}/${detail.summary.owner.name}; its controller may recreate this Pod.` : 'standalone Pod; recreation is not promised.'}</p>
      <ul className="m-0 grid list-none gap-1 p-0" aria-label="Pod action permissions">
        <CapabilityNotice label="Delete" value={deleteDecision} />
        <CapabilityNotice label="Port-forward" value={portForwardDecision} />
        <CapabilityNotice label="Exec" value={execDecision} />
      </ul>
      {permissions.isError ? <p className="m-0 text-xs text-kp-red">Permission check failed; actions remain disabled.</p> : null}
      <Checkbox checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)}>
        I checked Pod {detail.metadata.namespace}/{detail.metadata.name} (UID {detail.metadata.uid}) and understand these consequences.
      </Checkbox>
      <div className="flex flex-wrap items-end gap-2">
        <Button variant="danger" disabled={!confirmed || deleteDecision !== 'allowed' || remove.isPending} onClick={() => remove.mutate()}>{remove.isPending ? 'Deleting…' : 'Delete Pod'}</Button>
        <label className="grid gap-1 w-28"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Remote port</span><Input aria-invalid={!remotePortValid} type="number" min="1" max="65535" value={remotePort} onChange={(event) => setRemotePort(Number(event.target.value))} /></label>
        <label className="grid gap-1 w-36"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Local port (optional)</span><Input aria-invalid={!localPortValid} inputMode="numeric" value={localPort} onChange={(event) => setLocalPort(event.target.value)} placeholder="automatic" /></label>
        <Button variant="secondary" disabled={!confirmed || portForwardDecision !== 'allowed' || portForward.isPending || !remotePortValid || !localPortValid} onClick={() => portForward.mutate()}>{portForward.isPending ? 'Starting…' : 'Start port-forward'}</Button>
      </div>
      {!remotePortValid || !localPortValid ? <p className="m-0 text-xs text-kp-red">Remote port must be 1–65,535; an explicit local port must be 1,024–65,535.</p> : null}
      {portForward.data ? <p className="m-0 text-xs text-kp-green" role="status">Loopback listener: <strong className="mono">{portForward.data.localAddress}:{portForward.data.localPort}</strong> → {portForward.data.namespace}/{portForward.data.pod}:{portForward.data.remotePort}.</p> : null}
      <div className="flex flex-wrap items-end gap-2 border-t border-kp-overlay-0 pt-2.5">
        <label className="grid gap-1 w-36"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Container</span><Select value={container} onChange={(event) => setContainer(event.target.value)}>{detail.containers.map((value) => <option key={value.spec.name}>{value.spec.name}</option>)}</Select></label>
        <label className="grid flex-1 gap-1 min-w-[220px]"><span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Command argv (one item per line)</span><textarea className="min-h-[76px] w-full rounded-md border border-kp-overlay-0 bg-kp-crust px-2.5 py-1.5 text-sm text-kp-text focus:border-kp-mauve focus:shadow-focus focus:outline-none resize-y leading-relaxed" value={command} onChange={(event) => setCommand(event.target.value)} /></label>
        <Button variant="secondary" disabled={!confirmed || execDecision !== 'allowed' || exec.isPending || container === '' || !commandValid} onClick={() => exec.mutate()}>{exec.isPending ? 'Authorizing…' : 'Open exec session'}</Button>
      </div>
      {!commandValid ? <p className="m-0 text-xs text-kp-red">Provide 1–64 argv items, at most 4 KiB each and 32 KiB total, without NUL.</p> : null}
      {terminal.length > 0 ? (
        <div className="grid gap-2 rounded-lg border border-kp-blue-border bg-kp-crust p-2.5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="mr-auto text-xs text-kp-sky" aria-live="polite">Exec: {socketState}</span>
            <Button size="sm" variant="secondary" disabled={socketState !== 'ready'} onClick={closeExec}>Close session</Button>
            <Button size="sm" variant="secondary" onClick={() => { execTerminalRef.current?.clear(); setTerminal([]) }}>Clear output</Button>
          </div>
          <ul className="m-0 grid list-none gap-0.5 p-0 text-xs" aria-label="Exec session status">
            {terminal.map((line, index) => <li key={`${index}-${line.stream}`} className="mono text-kp-sky">{line.text}</li>)}
          </ul>
          <Suspense fallback={<pre aria-label="Exec terminal output" className="min-h-32" />}>
            <ExecTerminal
              ref={execTerminalRef}
              label="Exec terminal output"
              onStdin={sendRawStdin}
              onResize={handleTerminalResize}
            />
          </Suspense>
        </div>
      ) : null}
      {[remove, portForward, exec].map((operation, index) => operation.isError ? <p className="m-0 text-xs text-kp-red" key={index}>{mutationError(operation.error)}</p> : null)}
      {remove.isSuccess ? <p className="m-0 text-xs text-kp-green" role="status">Deletion accepted; recreation depends on the owner shown above.</p> : null}
    </section>
  )
}
