# Changelog

All notable changes to KubePeep are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versions follow [Semantic Versioning](https://semver.org/) **without the `v`
prefix** (official tags: `1.4.2`, `1.4.3`, …), and entries are generated
automatically from Conventional Commits by `.github/workflows/release.yml`.
## [0.2.1] - 2026-09-04

## Fixed
- fix(build): application icons derived from the official kubepeep-logo.svg
- fix: adjust svg

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.2.0...0.2.1

## [0.2.0] - 2026-09-04

## Added
- feat(release): publish installers with the version in the filename

## Changed
- release: 0.2.0 [skip ci]

## Fixed
- fix(release): quote versioned allowlist entries and silence SC2086 on intentional globs

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.6...0.2.0

## [0.2.0] - 2026-09-04

## Added
- feat(release): publish installers with the version in the filename

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.6...0.2.0

## [0.1.6] - 2026-09-04

## Fixed
- fix(release): smoke matches the real /releases/download/<tag> redirect route

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.5...0.1.6

## [0.1.5] - 2026-09-04

## Changed
- release: 0.1.5 [skip ci]

## Fixed
- fix(release): declare TAG in the step env of the permanent-URL smoke
- fix(release): require consecutive failed readings before aborting publish

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.4...0.1.5

## [0.1.5] - 2026-09-04

## Fixed
- fix(release): require consecutive failed readings before aborting publish

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.4...0.1.5

## [0.1.4] - 2026-09-04

## Fixed
- fix(serve): respond the SPA index at the root for plain HTTP health probes

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.3...0.1.4

## [0.1.3] - 2026-09-04

## Fixed
- fix(release): retry the permanent-URL smoke while the latest redirect propagates

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.2...0.1.3

## [0.1.2] - 2026-09-04

## Changed
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- refactor(release): build in parallel with verify; tag only after all gates

## Fixed
- fix(release): create dist/staging inside container and restore version-out for verify
- fix(release): container stages binary into dist/staging with universal permissions
- fix(release): nfpm diagnostics via error annotations (runner-only failure, invisible without auth)
- fix(release): create dist/ before nfpm and CLI archive steps (no output dir on clean runner)
- fix(release): run Linux version check inside the build container; hdiutil retry; NOTICE in CLI archives
- fix(release): track build/linux/nfpm.yaml and build/windows/icon.ico (ignored by build/ pattern)
- fix(release): version-env injection in container build, nfpm config, icon, dependabot
- fix(verify): prune go.sum entries added by 'go mod download all'
- fix(release): unblock container tidy offline and ensure NSIS on Windows

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.1...0.1.2

## [0.1.2] - 2026-09-04

## Changed
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- refactor(release): build in parallel with verify; tag only after all gates

## Fixed
- fix(release): container stages binary into dist/staging with universal permissions
- fix(release): nfpm diagnostics via error annotations (runner-only failure, invisible without auth)
- fix(release): create dist/ before nfpm and CLI archive steps (no output dir on clean runner)
- fix(release): run Linux version check inside the build container; hdiutil retry; NOTICE in CLI archives
- fix(release): track build/linux/nfpm.yaml and build/windows/icon.ico (ignored by build/ pattern)
- fix(release): version-env injection in container build, nfpm config, icon, dependabot
- fix(verify): prune go.sum entries added by 'go mod download all'
- fix(release): unblock container tidy offline and ensure NSIS on Windows

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.1...0.1.2

## [0.1.2] - 2026-09-03

## Changed
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- refactor(release): build in parallel with verify; tag only after all gates

## Fixed
- fix(release): nfpm diagnostics via error annotations (runner-only failure, invisible without auth)
- fix(release): create dist/ before nfpm and CLI archive steps (no output dir on clean runner)
- fix(release): run Linux version check inside the build container; hdiutil retry; NOTICE in CLI archives
- fix(release): track build/linux/nfpm.yaml and build/windows/icon.ico (ignored by build/ pattern)
- fix(release): version-env injection in container build, nfpm config, icon, dependabot
- fix(verify): prune go.sum entries added by 'go mod download all'
- fix(release): unblock container tidy offline and ensure NSIS on Windows

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.1...0.1.2

## [0.1.2] - 2026-09-03

## Changed
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- refactor(release): build in parallel with verify; tag only after all gates

## Fixed
- fix(release): create dist/ before nfpm and CLI archive steps (no output dir on clean runner)
- fix(release): run Linux version check inside the build container; hdiutil retry; NOTICE in CLI archives
- fix(release): track build/linux/nfpm.yaml and build/windows/icon.ico (ignored by build/ pattern)
- fix(release): version-env injection in container build, nfpm config, icon, dependabot
- fix(verify): prune go.sum entries added by 'go mod download all'
- fix(release): unblock container tidy offline and ensure NSIS on Windows

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.1...0.1.2

## [0.1.2] - 2026-09-03

## Changed
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- refactor(release): build in parallel with verify; tag only after all gates

## Fixed
- fix(release): run Linux version check inside the build container; hdiutil retry; NOTICE in CLI archives
- fix(release): track build/linux/nfpm.yaml and build/windows/icon.ico (ignored by build/ pattern)
- fix(release): version-env injection in container build, nfpm config, icon, dependabot
- fix(verify): prune go.sum entries added by 'go mod download all'
- fix(release): unblock container tidy offline and ensure NSIS on Windows

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.1...0.1.2

## [0.1.2] - 2026-09-03

## Changed
- release: 0.1.2 [skip ci]
- release: 0.1.2 [skip ci]
- refactor(release): build in parallel with verify; tag only after all gates

## Fixed
- fix(release): track build/linux/nfpm.yaml and build/windows/icon.ico (ignored by build/ pattern)
- fix(release): version-env injection in container build, nfpm config, icon, dependabot
- fix(verify): prune go.sum entries added by 'go mod download all'
- fix(release): unblock container tidy offline and ensure NSIS on Windows

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.1...0.1.2

## [0.1.2] - 2026-09-03

## Changed
- release: 0.1.2 [skip ci]
- refactor(release): build in parallel with verify; tag only after all gates

## Fixed
- fix(release): version-env injection in container build, nfpm config, icon, dependabot
- fix(verify): prune go.sum entries added by 'go mod download all'
- fix(release): unblock container tidy offline and ensure NSIS on Windows

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.1...0.1.2

## [0.1.2] - 2026-09-03

## Changed
- refactor(release): build in parallel with verify; tag only after all gates

## Fixed
- fix(verify): prune go.sum entries added by 'go mod download all'
- fix(release): unblock container tidy offline and ensure NSIS on Windows

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.1...0.1.2

## [0.1.1] - 2026-09-03

## Changed
- chore(verify): drop retired goreleaser snapshot harness

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/compare/0.1.0...0.1.1

## [0.1.0] - 2026-09-03

## Added
- feat(release): automated desktop release pipeline (Wails, all platforms)
- feat: YAML diff against the kubectl last-applied baseline (F7-02)
- feat: saved favorites with schema-bound persistence and palette group (F7-01)
- feat: global resource search in the command center (F7-04)
- feat: real exec terminal with xterm.js (F7-05)
- feat: observability follow-ups — lifecycle logs, request correlation, error sampler, doctor checks
- feat: phase 5 — observability: numeric durations, optional /metrics endpoint
- feat: phase 4 — architecture guardrails, coverage and timer race fix
- feat: complete phase 3 — dashboard navigation, namespace health and stale indicator
- feat: complete phase 2 — search parser, drawer detail, YAML highlight and logs
- feat: complete phase 1 — migrate all components to design system
- feat: phase 1 design system — tokens, brand SVGs and atomic components
- feat: desktop support and security hardening
- feat: add safe operational shortcuts
- feat: add resource list controls and harden validation
- feat: add secure keyboard command center
- feat: complete kubePeep MVP implementation
- feat: add PermissionsMatrix component and related tests
- feat: initialize kubepeep web application with React, TypeScript, and Vite

## Changed
- license: switch project license to Apache License 2.0
- docs: close G-10 with containerized desktop build, RC artifacts and build guide
- test+docs: E2E coverage for post-MVP features, API docs, drawer click-through fix
- docs: mark F7-02 done — all viable phase 7 items delivered
- docs: record phase 7 progress (F7-01/04/05 done) in the acceptance checklist
- docs: record follow-up execution results in the acceptance checklist
- test: make Playwright E2E serve the production bundle (G-09)
- refactor: consolidate pod classification primitives (F4-02)
- docs: record phase 0-6 execution results in plan
- docs: phase 6 — Compose tests and design system, RBAC, architecture docs
- docs: add comprehensive technical, functional and visual review plan
- docs: update project evidence and local tooling ignores
- docs: record full project reindex
- docs: define secure operational experience
- security: prevent sensitive data in repository history
- Close Phase 3 with native CI evidence
- Honor Windows delete sharing for secure files
- Validate canonical private Windows ACLs
- Expose sanitized Windows CI diagnostics
- Bound remaining native runtime lifecycle tests
- Validate Windows runtime DACL structurally
- Bound native runtime readiness test
- Harden Phase 3 native validation
- Complete Phase 2 specifications
- Add tests for Ginger WebSocket handling and command execution
- docs: adiciona plano de desenvolvimento do Kube Peep

## Fixed
- fix(tests): release fake SPDY mutex before blocking factories
- fix(release): satisfy shellcheck in workflow scripts
- fix: phase 0 critical fixes — flaky tests, context lifecycle and stderr isolation
- fix: harden dashboard rendering and responsive layout
- fix: align restricted Kind validation contracts
- fix: remove Windows installer fixture override
- fix: make Windows installer tests native-safe
- fix: hash Windows update candidates without module autoload
- fix: add safe Windows updater diagnostics
- fix: stabilize native Windows and Kind validation
- fix: stabilize Windows updater and Kind validation
- fix: update artifact.json commit and indexed_at timestamps; add .codebase-memory files to .gitignore

## Security
- security: prevent sensitive data in repository history

## Download

Permanently-updated links: https://github.com/fvmoraes/kubepeep/blob/main/docs/download.md

## Full Changelog
**Full Changelog**: https://github.com/fvmoraes/kubepeep/releases/tag/0.1.0

[0.1.0]: https://github.com/fvmoraes/kubepeep/releases/tag/0.1.0
[0.1.1]: https://github.com/fvmoraes/kubepeep/releases/tag/0.1.1
[0.1.2]: https://github.com/fvmoraes/kubepeep/releases/tag/0.1.2
[0.1.3]: https://github.com/fvmoraes/kubepeep/releases/tag/0.1.3
[0.1.4]: https://github.com/fvmoraes/kubepeep/releases/tag/0.1.4
[0.1.5]: https://github.com/fvmoraes/kubepeep/releases/tag/0.1.5
[0.1.6]: https://github.com/fvmoraes/kubepeep/releases/tag/0.1.6
[0.2.0]: https://github.com/fvmoraes/kubepeep/releases/tag/0.2.0
[0.2.1]: https://github.com/fvmoraes/kubepeep/releases/tag/0.2.1
