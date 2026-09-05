# Fase 0 (concluída) e escopo da v1

> Executar esta fase **não** requer trabalho: serve de âncora de estado e de vocabulário comum para as fases seguintes.

## 1. O que já está entregue (commit `5ac7320`, 2026-09-04)

| Área | Entregue |
| --- | --- |
| Design tokens | `web/src/tokens.css` — superfícies neutras (`#111016`/`#18161E`), texto `#F4F1F7`→`#686271`, roxo de marca `#A78BFA` restrito a seleção/navegação/foco, semânticas azul/verde/vermelho/âmbar, escala tipográfica 10–32px, controles 28/32/36px |
| Tipografia | Inter Variable única família; monospace somente em logs, YAML, código e terminal |
| Componentes | `web/src/components/ui/`: Button (7 variantes), Badge/StatusBadge, Input, Select, Checkbox, Field, SearchInput, Card, DataTable, Drawer, Tabs, Banner×4, PageHeader, EmptyState, Skeleton |
| Shell | Sidebar com árvore Kubernetes em grupos colapsáveis + modo compacto com tooltips + versão real (`useAppVersion`); topbar 56px com contexto auto-apply, chip de namespace scope, status; command palette gerada da árvore |
| Resource framework | `web/src/components/resource/`: ResourcePage, SelectionGate/QueryState/CollectionFooter, TableLink, Facts, ResourceTabStrip, format, status, errors |
| Páginas migradas | Dashboard, Workloads, Pods, Events, Network, Config, Logs, Namespaces, Permissions, Settings |
| Navegação | `web/src/navigation/tree.tsx` — fonte única; recursos sem backend aparecem desabilitados ("available in a future release") |
| Validação | typecheck, lint, 86 unit, 6 e2e, 994 Go, build, screenshots 1280→1920 |

## 2. Suportes que a v1 encontra prontos (backend)

- Padrão de coleção: `internal/api/handlers/routes.go` (rotas + `allowedMethods`), envelope `data/meta` com `page` (cursor, `complete`, `truncated`, `filterScope`) e `coverage`.
- RBAC: matriz de capabilities (`internal/services/authorization/matrix.go`), `GET /api/v1/permissions` com SAR por namespace/recurso.
- Client-go: `internal/adapters/kubernetes` com budgets, cancelamento por geração e client por profile/contexto.
- Frontend: `useResourceList` pattern (draft/applied + cursor por geração + saved filters), `getPermissions` por ação, `web/src/navigation/tree.tsx` com itens desabilitados prontos para `path`.

## 3. Escopo da v1

1. Todas as famílias de recursos do grupo de navegação habilitadas (Cluster, Workloads complementares, Configuration, Storage, Access Control, Administration) — exceto Helm e Gateway API (pós-1.0).
2. Recursos **cluster-scoped** funcionando sem namespace scope (§13 da especificação de referência).
3. Experiência operacional fechada: port-forward panel, ações contextuais, colunas, favoritos/recentes, YAML/logs refinements.
4. Preferências de shell persistidas no allowlist do backend.
5. Release 1.0.0 com RC imutável e documentação atualizada.

## 4. Fora de escopo da v1

- Helm Releases e Gateway API (grupos já reservados na navegação).
- Edição/apply de recursos (o produto permanece read-only + ações allowlisted).
- Multi-cluster simultâneo (multi-contexto readonly segue na Fase 6 como melhor caso; se estourar o orçamento, vira 1.1).

## 5. Restrições herdadas

- `web/src/security.test.ts` proíbe localStorage/sessionStorage em produção — preferências de shell vão para `/api/v1/preferences` (allowlist no backend), não para o browser.
- Secrets nunca expõem `data`/`stringData`/YAML — vale para toda página nova.
- Nenhuma página nova cria CSS próprio; apenas tokens e componentes de `web/src/components/ui/` + `components/resource/`.
