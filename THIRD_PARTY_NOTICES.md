# Third-party notices

Kube Peep is distributed under the MIT license in `LICENSE`. Its executable
also contains open-source dependencies. Versions are fixed by `go.mod`,
`go.sum`, and `web/package-lock.json`; the linked upstream license files remain
the authoritative terms.

## Direct Go runtime dependencies

| Component | Version | License |
| --- | --- | --- |
| [coder/websocket](https://github.com/coder/websocket) | 1.8.15 | ISC |
| [fvmoraes/ginger](https://github.com/fvmoraes/ginger) | 1.4.4 | MIT |
| [spf13/cobra](https://github.com/spf13/cobra) | 1.10.2 | Apache-2.0 |
| [golang.org/x/sys](https://cs.opensource.google/go/x/sys) | 0.46.0 | BSD-3-Clause |
| [go-yaml/yaml](https://github.com/go-yaml/yaml) | 3.0.1 | MIT |
| [kubernetes/api](https://github.com/kubernetes/api) | 0.35.7 | Apache-2.0 |
| [kubernetes/apimachinery](https://github.com/kubernetes/apimachinery) | 0.35.7 | Apache-2.0 and bundled BSD-3-Clause code |
| [kubernetes/client-go](https://github.com/kubernetes/client-go) | 0.35.7 | Apache-2.0 |
| [kubernetes/metrics](https://github.com/kubernetes/metrics) | 0.35.7 | Apache-2.0 |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | 1.54.0 | BSD-3-Clause |
| [kubernetes-sigs/yaml](https://github.com/kubernetes-sigs/yaml) | 1.6.0 | Apache-2.0 |

The linked binary also includes transitive modules listed exactly in `go.mod`
and `go.sum`. Their upstream license families were checked with
`go-licenses v1.6.0`: Apache-2.0, BSD-3-Clause, ISC, and MIT only. The
`modernc.org/mathutil` result that the classifier could not identify was
manually verified from its bundled `LICENSE` as BSD-3-Clause; the same manual
check was applied to the platform-specific modernc modules.

## Embedded frontend runtime dependencies

| Component | Version | License |
| --- | --- | --- |
| [TanStack Query for React](https://github.com/TanStack/query) | 5.101.4 | MIT |
| [TanStack Query Core](https://github.com/TanStack/query) | 5.101.4 | MIT |
| [Lucide React](https://github.com/lucide-icons/lucide) | 1.28.0 | ISC |
| [React](https://github.com/facebook/react) | 19.2.8 | MIT |
| [React DOM](https://github.com/facebook/react) | 19.2.8 | MIT |
| [Scheduler](https://github.com/facebook/react) | 0.27.0 | MIT |
| [React Router](https://github.com/remix-run/react-router) | 8.3.0 | MIT |
| [cookie-es](https://github.com/unjs/cookie-es) | 3.1.1 | MIT |

Build and test tools are development-only and are not required at runtime.
Their exact versions and licenses remain recorded by the frontend lockfile.

## Reference project provenance

The DWYT project was reviewed at commit
`a9386823272b928f2289c9020a9ae5951389e0f1` under MIT. Kube Peep reinterprets
only general local-application and visual principles; it does not copy DWYT
business logic or source. The review boundary is documented in
`docs/research/dwyt.md`.
