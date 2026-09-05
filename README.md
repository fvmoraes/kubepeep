# KubePeep

[![Latest Release](https://img.shields.io/github/v/release/fvmoraes/kubepeep?sort=semver&label=release&logo=github)](https://github.com/fvmoraes/kubepeep/releases/latest)
[![Release pipeline](https://github.com/fvmoraes/kubepeep/actions/workflows/release.yml/badge.svg)](https://github.com/fvmoraes/kubepeep/actions/workflows/release.yml)

KubePeep is a local, self-contained Kubernetes dashboard. It uses the current
user's kubeconfig and Kubernetes RBAC identity, and embeds its React interface
in the Go binary. It runs as a native desktop application (Wails) or as a
loopback web server (`serve`); both modes share the same backend core, and no
external browser is ever opened by the desktop build.

The governing rule is simple: show only what the current identity may read and
enable only what it may execute. KubePeep does not ask for separate cluster
credentials, does not impersonate users, and never displays Secret values.

## Requirements

- A working kubeconfig and any `exec` credential plugin already referenced by it.
- Kubernetes permissions for the resources that should be visible or actionable.
- No Node.js, database server, or other KubePeep runtime dependency.

The Metrics API is optional. Its absence affects only the metrics block.

## Download

Official desktop packages and CLI archives are published on GitHub Releases.
The links below are permanent: they always deliver the latest stable release.

| Platform | Architecture | Download |
|----------|--------------|----------|
| Windows 10/11 | x86_64 | [kubepeep-windows-amd64-setup.exe](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-windows-amd64-setup.exe) |
| Linux DEB (Debian/Ubuntu) | x86_64 | [kubepeep-linux-amd64.deb](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.deb) |
| Linux RPM (Fedora/RHEL) | x86_64 | [kubepeep-linux-amd64.rpm](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-amd64.rpm) |
| Linux DEB (Debian/Ubuntu) | ARM64 | [kubepeep-linux-arm64.deb](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.deb) |
| Linux RPM (Fedora/RHEL) | ARM64 | [kubepeep-linux-arm64.rpm](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-linux-arm64.rpm) |
| macOS (Intel) | x86_64 | [kubepeep-darwin-amd64.dmg](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-amd64.dmg) |
| macOS (Apple Silicon) | ARM64 | [kubepeep-darwin-arm64.dmg](https://github.com/fvmoraes/kubepeep/releases/latest/download/kubepeep-darwin-arm64.dmg) |

Latest release page: <https://github.com/fvmoraes/kubepeep/releases/latest> ·
Per-platform details and verification notes:
[docs/download.md](docs/download.md).

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
kubepeep            # desktop window (desktop builds)
kubepeep serve      # web server on 127.0.0.1 (always available)
```

Useful commands and flags:

```text
kubepeep [--kubeconfig PATHS] [--context NAME] [--namespace NAME]
         [--port 1024-65535] [--no-browser]
kubepeep serve [--kubeconfig PATHS] [--context NAME] [--namespace NAME]
               [--port 1024-65535] [--no-browser]
kubepeep status
kubepeep stop
kubepeep doctor [--json]
kubepeep version
kubepeep update --version X.Y.Z
```

Local data lives under `~/.kubePeep/` on Linux/macOS and
`%LOCALAPPDATA%\kubePeep\` on Windows. It contains configuration, SQLite data,
operational logs, cache, and short-lived runtime state—not kubeconfig contents,
Secret values, application logs, or terminal traffic.

KubePeep does not install a ClusterRole. Grant only the Kubernetes operations
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
files or any file outside KubePeep's canonical data root.

## Development

Pinned toolchain versions are Go 1.26.7, Node.js 24.18.0, npm 11.16.0,
Ginger v1.4.4, Wails v2.15.0, and GoReleaser v2.17.1.

```sh
make web-install
make verify
make release-snapshot
```

### Desktop (Wails)

Native Wails dependencies are required before building the desktop binary:

- Linux: `sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev` (Ubuntu 24.04
  may need `libwebkit2gtk-4.1-dev` plus `-tags webkit2_41`);
- macOS: Xcode command line tools (`xcode-select --install`);
- Windows: WebView2 runtime (no CGO required).

Install the Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0`),
then:

```sh
make dev                    # hot reload: wails dev (React + Go rebuild)
make build-desktop          # native desktop binary (dist/desktop/)
make build-desktop-linux    # linux/amd64
make build-desktop-windows  # windows/amd64
make build-desktop-darwin   # darwin/amd64
```

The desktop build compiles with `-tags desktop`; the plain `make build` target
produces the web binary (`kubepeep serve`). Bindings: JSON API calls travel
through the in-process Wails bridge (`Bridge.Invoke` with a strict path
allowlist); SSE streams and the exec WebSocket use an internal loopback
listener because the Wails AssetServer cannot carry them. See
[desktop architecture](docs/desktop-architecture.md) for the full design.

Sensitive data is never allowed in Git history. Enable the repository's
pre-commit and pre-push gates once per clone, and run the same check explicitly
whenever needed:

```sh
git config --local core.hooksPath .githooks
./scripts/security_check.sh HEAD
```

Commit authors and committers must use an approved GitHub noreply address. See the
[repository security premise](docs/security.md#11-repositório-e-cadeia-de-desenvolvimento)
before adding fixtures, logs, kubeconfigs, environment files, or diagnostics.

The executable is built with `CGO_ENABLED=0`; frontend assets and SQLite
migrations are embedded. See [the executable plan](plan/README.md),
[API contract](docs/api.md), [security model](docs/security.md), and
[architecture](docs/architecture.md) for details.

## Security and limitations

- The HTTP server binds only to `127.0.0.1` and enforces exact Host, Origin,
  CSRF, request-body, and no-store policies.
- In desktop builds the web server binds only to `127.0.0.1` and is used
  exclusively by streaming transports (SSE and the exec WebSocket); all other
  API calls travel through the in-process Wails bridge without opening a
  port. The WebView origins allowed by the desktop security profile
  (`wails://wails`, `http(s)://wails.localhost`, `null`) are never accepted by
  web builds.
- Kubernetes remains the authorization authority. UI capabilities are hints;
  mutable operations are reauthorized immediately before execution.
- Secret resources are metadata-only and have no YAML endpoint.
- Log reads and scans are bounded, redacted in memory, and never persisted.
- A kubeconfig `exec` plugin is an external runtime dependency managed by the
  user; KubePeep reports sanitized failures when it is unavailable.
- Windows on ARM64 is published only while the native archive smoke test remains
  green.

## Troubleshooting

- Desktop does not start? The binary must be built with `-tags desktop` and
  the platform's native Wails dependencies installed (Linux:
  `libgtk-3-dev` + `libwebkit2gtk-4.0-dev`; macOS: Xcode CLT; Windows:
  WebView2). Without the tag, `kubepeep` falls back to the web runtime — use
  `kubepeep serve` and open `http://127.0.0.1:<port>` manually.
- "A window does not appear and there is no error" on Linux usually means the
  WebKit2GTK runtime is missing; install the packages listed above and retry.
- Live updates or log follow stuck on "connecting" inside the desktop window
  indicates the streaming transport is unavailable in the WebView; the
  interface automatically falls back to bounded manual refresh.
- Run `kubepeep doctor` (or `kubepeep doctor --json`) first. It separates local
  application, filesystem, SQLite, kubeconfig, context, cluster, and permission
  checks without printing credentials.
- Use `kubePeep status` to find the active loopback port. If startup reports a
  port conflict, omit `--port` to use the next available allowed port.
- Cluster-offline and missing Metrics API states are intentionally degraded,
  not local application failures. Restore cluster connectivity or install the
  Metrics API only if metrics are required.
- An unavailable kubeconfig `exec` credential plugin must be repaired in the
  user's Kubernetes environment; KubePeep neither installs nor replaces it.
- A `403` after a page was already open can mean RBAC changed. Refresh the page
  or reselect the context; every mutable operation is checked again server-side.
- Stop the service before update or uninstall. On Windows, a successful update
  first reports that replacement is scheduled, then records the post-exit result
  beside the executable without changing local application data.
- Operational logs are under the local data root's `logs` directory. They are
  metadata-only and sanitized; application logs and terminal contents are never
  written there.

## License

Apache License, Version 2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE) and
[third-party notices](THIRD_PARTY_NOTICES.md).
