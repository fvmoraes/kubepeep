# KubePeep

[![Latest Release](https://img.shields.io/github/v/release/fvmoraes/kubepeep?sort=semver&label=release&logo=github)](https://github.com/fvmoraes/kubepeep/releases/latest)
[![Release pipeline](https://github.com/fvmoraes/kubepeep/actions/workflows/release.yml/badge.svg)](https://github.com/fvmoraes/kubepeep/actions/workflows/release.yml)

KubePeep is a local Kubernetes dashboard with a native desktop window (Wails)
and a loopback web mode. Both share a Go backend and an embedded React
interface. It uses your existing kubeconfig and Kubernetes RBAC identity.

Inspect workloads, Pods, Events, networking, ConfigMaps and Secret metadata;
read bounded logs; and use restart, scale, Pod deletion, port-forward and exec
when authorized. Secret values are never exposed. The Metrics API is optional.

The expanded resource catalog for version 1 is tracked in the
[execution plan](plan/README.md). Navigation placeholders do not represent
implemented resources; see the [current product scope](docs/product-spec.md).

## Install and run

Choose a package from [GitHub Releases](https://github.com/fvmoraes/kubepeep/releases/latest).
See [downloads, checksum verification and removal](docs/download.md) for
Windows, Linux and macOS, including installation from an explicit version.

```sh
kubePeep                     # native window in desktop builds
kubePeep serve               # web mode on 127.0.0.1
kubePeep doctor              # sanitized local and cluster diagnostics
kubePeep version
```

Linux system packages install the command as `kubepeep`; CLI archives and
scripted installs use `kubePeep`. Use the spelling provided by your package.

Common options are `--kubeconfig PATHS`, `--context NAME`, `--namespace NAME`,
`--port 1024-65535` and `--no-browser`. Use `kubePeep --help` for the full CLI.
`status` and `stop` control the web instance; closing the desktop window ends
its sessions. A plain CLI build opens web mode when run without a subcommand.

Requirements: a working kubeconfig, its configured credential plugins, and
permission to access the resources you need. Desktop packages also require
their platform's native WebView libraries. Node.js and a database server are
not runtime requirements.

## Data and security

Local configuration, SQLite preferences and operational logs live in
`~/.kubePeep/` on Unix or `%LOCALAPPDATA%\kubePeep\` on Windows. Kubeconfig
contents, Secret values, container logs and terminal traffic are not persisted
there. Updates and data removal require explicit actions.

The local API enforces Host/Origin checks, CSRF and bounded requests. Kubernetes
authorizes each operation; mutable actions are checked again immediately before
execution. Read the [security model](docs/security.md) and
[RBAC requirements](docs/rbac-requirements.md) for the precise contracts.

## Development

```sh
make web-install
make verify
make dev-desktop             # requires the Wails CLI and native dependencies
```

The [development guide](docs/development.md) documents toolchain versions,
builds, test gates, repository layout and private artifacts. Desktop details
are in the [desktop build guide](docs/desktop-build.md).

**Golden rule: commit only. Never push automatically.** Run
`./scripts/security_check.sh HEAD` before committing; use an approved GitHub
noreply identity. Publishing requires a separate, explicit user decision.

Start at the [documentation index](docs/README.md) for architecture, API and
data contracts, or the [v1 plan](plan/README.md) for implementation phases.

## Troubleshooting

- Run `kubePeep doctor --json` for sanitized diagnostics. Repair missing
  kubeconfig credential plugins in your Kubernetes environment.
- For the web server, `kubePeep status` reports the active port. An explicitly
  requested occupied port fails; omitting `--port` allows the default range.
- Cluster-offline, RBAC-denied and missing-metrics states are independent.
  Refresh permissions after an RBAC change; an unavailable optional block does
  not make the local application unhealthy.
- If the desktop window cannot open, check the platform dependencies in the
  [build guide](docs/desktop-build.md). `kubePeep serve` provides web mode.

## License

Apache License 2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE) and
[third-party notices](THIRD_PARTY_NOTICES.md).
