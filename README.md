# Kube Peep

Kube Peep is a local, self-contained Kubernetes dashboard. It uses the current
user's kubeconfig and Kubernetes RBAC identity, listens only on
`127.0.0.1`, and embeds its React interface in the Go binary.

The governing rule is simple: show only what the current identity may read and
enable only what it may execute. Kube Peep does not ask for separate cluster
credentials, does not impersonate users, and never displays Secret values.

## Requirements

- A working kubeconfig and any `exec` credential plugin already referenced by it.
- Kubernetes permissions for the resources that should be visible or actionable.
- No Node.js, database server, or other Kube Peep runtime dependency.

The Metrics API is optional. Its absence affects only the metrics block.

## Install a release

Always choose an explicit release version. The installers download assets only
from that immutable tag and require a matching SHA-256 entry from the same
release.

Linux or macOS:

```sh
version=0.1.0
curl --fail --location --proto '=https' --tlsv1.2 \
  "https://github.com/fvmoraes/kubepeep/releases/download/v${version}/install.sh" \
  --output /tmp/kubepeep-install.sh
sh /tmp/kubepeep-install.sh --version "$version"
```

Windows PowerShell 5.1 or newer:

```powershell
$Version = '0.1.0'
$Installer = Join-Path $env:TEMP 'kubepeep-install.ps1'
Invoke-WebRequest -UseBasicParsing \
  -Uri "https://github.com/fvmoraes/kubepeep/releases/download/v$Version/install.ps1" \
  -OutFile $Installer
& $Installer -Version $Version
```

The default destination is `~/.local/bin/kubePeep` on Unix and
`%LOCALAPPDATA%\Programs\kubePeep\kubePeep.exe` on Windows. Installation does
not require administrator privileges.

## Run

```sh
kubePeep start
```

The root command is equivalent to `start`. Useful commands and flags:

```text
kubePeep start [--kubeconfig PATHS] [--context NAME] [--namespace NAME]
               [--port 1024-65535] [--no-browser]
kubePeep status
kubePeep stop
kubePeep doctor [--json]
kubePeep version
kubePeep update --version X.Y.Z
```

Local data lives under `~/.kubePeep/` on Linux/macOS and
`%LOCALAPPDATA%\kubePeep\` on Windows. It contains configuration, SQLite data,
operational logs, cache, and short-lived runtime state—not kubeconfig contents,
Secret values, application logs, or terminal traffic.

Kube Peep does not install a ClusterRole. Grant only the Kubernetes operations
the user already needs. Read-only pages use `get`, `list`, and, when live
updates are enabled, `watch` on their exact resources. Logs require
`get pods/log`; restart requires `patch apps/deployments`; scale requires
`update apps/deployments/scale` or `update apps/statefulsets/scale`; Pod delete
requires `delete pods`; port-forward and exec require `create` on their exact
Pod subresources. A denied namespace or resource remains unavailable without
breaking other authorized sections.

Updates are explicit rather than silent. `kubePeep update --version X.Y.Z`
downloads only that immutable tag, verifies the published SHA-256 checksum and
the candidate version, then performs an atomic replacement with rollback. Stop
the running local service before updating. On Windows, the verified replacement
helper finishes after the update command exits because an executing `.exe`
cannot be replaced in place.

## Remove

The installers remove only the binary and the PATH entry they created by
default. Local data is preserved:

```sh
sh /tmp/kubepeep-install.sh --uninstall
```

```powershell
& $Installer -Uninstall
```

To also remove the canonical local data root, make the destructive scope
explicit twice:

```sh
sh /tmp/kubepeep-install.sh --uninstall --purge-data --confirm-purge
```

```powershell
& $Installer -Uninstall -PurgeData -ConfirmPurge
```

Purging local data is a separate destructive operation and requires the
installer's explicit purge confirmation flag. Neither path removes kubeconfig
files or any file outside Kube Peep's canonical data root.

## Development

Pinned toolchain versions are Go 1.25.13, Node.js 24.18.0, npm 11.16.0,
Ginger v1.4.4, and GoReleaser v2.17.1.

```sh
make web-install
make verify
make release-snapshot
```

The executable is built with `CGO_ENABLED=0`; frontend assets and SQLite
migrations are embedded. See [the executable plan](plan/README.md),
[API contract](docs/api.md), [security model](docs/security.md), and
[architecture](docs/architecture.md) for details.

## Security and limitations

- The HTTP server binds only to `127.0.0.1` and enforces exact Host, Origin,
  CSRF, request-body, and no-store policies.
- Kubernetes remains the authorization authority. UI capabilities are hints;
  mutable operations are reauthorized immediately before execution.
- Secret resources are metadata-only and have no YAML endpoint.
- Log reads and scans are bounded, redacted in memory, and never persisted.
- A kubeconfig `exec` plugin is an external runtime dependency managed by the
  user; Kube Peep reports sanitized failures when it is unavailable.
- Windows on ARM64 is published only while the native archive smoke test remains
  green.

## Troubleshooting

- Run `kubePeep doctor` (or `kubePeep doctor --json`) first. It separates local
  application, filesystem, SQLite, kubeconfig, context, cluster, and permission
  checks without printing credentials.
- Use `kubePeep status` to find the active loopback port. If startup reports a
  port conflict, omit `--port` to use the next available allowed port.
- Cluster-offline and missing Metrics API states are intentionally degraded,
  not local application failures. Restore cluster connectivity or install the
  Metrics API only if metrics are required.
- An unavailable kubeconfig `exec` credential plugin must be repaired in the
  user's Kubernetes environment; Kube Peep neither installs nor replaces it.
- A `403` after a page was already open can mean RBAC changed. Refresh the page
  or reselect the context; every mutable operation is checked again server-side.
- Stop the service before update or uninstall. On Windows, a successful update
  first reports that replacement is scheduled, then records the post-exit result
  beside the executable without changing local application data.
- Operational logs are under the local data root's `logs` directory. They are
  metadata-only and sanitized; application logs and terminal contents are never
  written there.

## License

MIT. See [LICENSE](LICENSE) and [third-party notices](THIRD_PARTY_NOTICES.md).
