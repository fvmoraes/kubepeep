# Investigation and diagnostics

Phase 5 adds a bounded, problem-oriented investigation path on top of the
generation-scoped resource cache. It does not run a hidden cluster-wide scan
when the operator types or opens an investigation.

## Local relationship index

`ResourceCache.LocalIndex` derives secondary relationships from non-expired,
unfiltered snapshots in the active generation:

- Pods by owner, phase and bounded labels;
- Services by selector and EndpointSlices by the official Service label;
- Events by involved object;
- ConfigMaps and PersistentVolumeClaims referenced by Pods or workload Pod
  templates.

Secret names and values are excluded from this index. ConfigMap and PVC
relationships contain identity only. Event messages and Kubernetes object
payloads are not copied into the public index.

Coverage is conservative. A topic is complete only when every expected GVR is
loaded for every namespace in the active scope, or a complete global origin is
present. Selector-filtered, paginated, stale, partial and missing origins never
prove absence. The API returns topic state and loaded namespaces so the UI can
show `unknown` or `incomplete` instead of a false zero.

## Problems

The pure `internal/problems` package derives findings from observed state. It
detects CrashLoopBackOff, image pull failures, Pending Pods, OOMKilled
containers, restart thresholds, failed Jobs, unavailable replicas, Pending
PVCs, Node NotReady, failing probes and Warning Events. Findings carry
`critical`, `warning` or `info` severity and point to their observed source.

The dashboard combines authorized Pod, workload, Event and Node reads with
the PVC cache. A denied or unavailable source makes the block incomplete and
retains a sanitized reason. It never becomes “zero problems”. Inspect opens the
Resource Workspace; logs are offered for Pods and supported workloads.

## Investigation workspace

Pods and workloads have an **Investigation** tab. The tab resolves owner chain,
related Pods, Services, EndpointSlices, ConfigMaps, PVCs and Events from the
local index. Opening a related object uses the existing workspace history.
Missing relationships remain explicitly tied to local cache coverage.

## Workload logs

The Logs page supports Deployment, ReplicaSet, StatefulSet, DaemonSet and Job
targets. It discovers related, log-authorized Pods and combines lines with Pod,
container and timestamp identity.

The client enforces these bounds:

- at most five simultaneous Pod/container streams;
- 75 ms rendering batches;
- at most 1,000 lines and 1 MiB of decoded line state;
- at most 2,000 requested tail lines;
- immediate shared abort when the target changes, Stop is selected, or the
  page unmounts.

Current and previous reads, follow, container selection, text search and regular
expression filtering remain in browser memory only. Kubernetes log content is
never written to localStorage, sessionStorage, IndexedDB, SQLite or telemetry.

## Command palette

Ctrl/Cmd+K searches static routes plus identifiers from bounded resource pages
already present in the React Query cache. Filtering is local after a 30 ms
debounce and performs no request per keystroke. The palette states that absence
from loaded results does not prove absence from the cluster.

## Performance and cluster diagnostics

Settings → Diagnostics → Performance reads process-local metrics and the local
index for the active generation. It reports:

- API latency p50/p95/p99 from a 512-sample rolling window per safe metric
  series;
- cache hit ratio, active watches, requests/minute, 429 responses and watch
  reconnects;
- resource sync percentiles;
- Kubernetes version, loaded totals and bounded cache/cursor memory;
- per-namespace loaded counts, problem severities, restarts and measured LIST
  latency, ordered slowest first.

The response has the normal dashboard timeout and measures at most 200
canonical namespaces with four workers. If the scope is larger, or RBAC/cache
coverage is partial, the response is marked incomplete and explains the bounded
limit. Namespace identity is returned only in the requested response and is
never retained in telemetry labels.

## Local API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/local-index` | Generation-scoped identities, counts and coverage |
| `GET` | `/api/v1/investigation/{kind}/{namespace}/{name}` | Local relationships for a Pod or supported workload |
| `GET` | `/api/v1/diagnostics` | Performance, namespace, cluster and KubePeep state |

All responses use `Cache-Control: no-store` and a publish-time generation
fence. Investigation validates kind, DNS names and active namespace scope.
These endpoints are read-only and pass through the existing browser origin,
request ID and recovery middleware.
