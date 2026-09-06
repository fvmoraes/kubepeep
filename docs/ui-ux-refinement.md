# UI/UX Refinamento — Navegação, Contexto Global, Ações e Resource Workspace

Implementação do plano `KUBEPEEP_UI_UX_REFINEMENT_PLAN.md`. O dark theme, o roxo
KubePeep, sidebar, navegação horizontal, chips, inputs e selects foram
preservados — o objetivo foi refinar, padronizar, compactar, corrigir e tornar
funcional, sem redesign.

## 1. Tipografia e Design System

- Escala tipográfica única já existente em `web/src/tokens.css`
  (10/11/12/13/14/16/20/26/32 px) permanece a fonte de verdade.
- Referência visual dos menus/filtros (`text-sm` 12 px em itens de menu,
  `text-base` 13 px em controles) é o tamanho-base de toda a UI; tabelas,
  facts, detalhes e formulários usam `text-base`/`text-sm`; `text-2xs` (10 px)
  fica reservado a labels uppercase, cabeçalhos de tabela e badges.
- Tokens novos: `--z-workspace`, `--z-toast`, `--z-confirm`,
  `--workspace-inset`, `--content-max-width` ampliado para 1680 px.
- `web/src/styles.css` ganhou blocos de estilos do Resource Workspace,
  viewport de toasts e da barra de filtros compacta.

## 2. Proporção de filtros e conteúdo

- `ResourceListControls` foi compactado: uma linha de controles `h-7`
  (busca, filtros inline, sort, order, Apply/Refresh/Clear) + uma linha fina
  de chips com filtros aplicados. Sem labels empilhados; cada controle usa
  `aria-label`. A área de filtros fica em ~1 linha (~60-80 px) e o restante
  da página é conteúdo — bem dentro da meta 20-30% / 70-80%.
- Tabelas: `DataTable` agora é full-width (o grid de 2 colunas com drawer foi
  removido), com `stickyHeader` opcional.

## 3. Navegação (correções)

- **BUG A (corrigido)**: `AccessPages` lia o segmento `:namespace` como tab —
  ClusterRoles/ClusterRoleBindings/RoleBindings eram inalcançáveis pela
  sidebar. Agora a tab vem de `:tab`.
- **BUG B (corrigido)**: rotas de detalhe inexistentes que produziam 404:
  `/access/:tab/:name`, `/storage/:tab/:name`, `/network/:tab/:name` e
  `/service-accounts/:namespace/:name` registradas em `App.tsx`.
- `web/src/navigation/paths.ts` centraliza o path canônico de detalhe/lista de
  cada coleção (usado por tabelas, command palette, favoritos e recents —
  antes havia três mapas divergentes).
- Query keys de Access/Administration agora incluem `generation` e as chamadas
  enviam `expectedGeneration` (antes ficavam stale entre contextos).

## 4. Contexto global: Kubeconfig → Context → Scope → Namespace

- Nova barra superior: `ContextSelector` + `GlobalNamespaceSelect` + escopo.
  A hierarquia kubeconfig/context/scope/namespace fica explícita no topo.
- `web/src/context/GlobalNamespace.tsx`:
  - universo de opções = namespaces do Scope ativo
    (`GET /namespace-scopes/{id}` para single/list) ou o catálogo do cluster
    (`GET /namespaces`, RBAC-governado) quando o scope é `all`;
  - `All` = todos os namespaces permitidos pelo Scope atual, nunca além do
    RBAC;
  - troca de contexto/scope recarrega as opções e reseta para `All`
    (render-time adjustment, sem efeitos com setState).
- O namespace global é aplicado a todas as coleções namespaced
  (pods, workloads, events, services, ingresses, endpoint-slices, endpoints,
  network-policies, configmaps, secrets, leases, PVCs, quotas, limit ranges,
  HPAs, PDBs, roles, bindings, service accounts). Coleções cluster-scoped
  (nodes, PVs, StorageClasses, CSI, CRDs, classes, webhooks, ingress classes)
  o ignoram.

## 5. Ações contextuais por tipo de recurso (backend + frontend)

Novas ações no backend (`internal/services/actions`, allowlist de
capabilities, handlers e rotas), todas com confirmação, fence de geração,
idempotência quando aplicável, audit allowlisted e **revalidação SSAR
fail-closed imediatamente antes da execução** (`Guard`/`Revalidate`):

| Ação | Recursos | Rota | Capability |
|---|---|---|---|
| Restart (rollout) | Deployments, StatefulSets, DaemonSets | `POST /api/v1/workloads/{kind}/{ns}/{name}/restart` | `{kind}.restart` (patch) |
| Scale | Deployments, StatefulSets | `PUT .../scale` | `{kind}.scale` |
| Delete workload | Deployments, StatefulSets, DaemonSets, Jobs, CronJobs, ReplicaSets | `DELETE /api/v1/workloads/{kind}/{ns}/{name}` | `{kind}.delete` |
| Suspend/Resume | CronJobs | `PUT .../suspend` | `cronjobs.suspend` |
| Run now | CronJobs (cria Job com owner ref) | `POST .../trigger` | `cronjobs.runnow` (create jobs) |
| Delete Pod (existente) | Pods | `DELETE /api/v1/pods/{ns}/{name}` | `pods.delete` |
| Restart Pod | Pods (delete controlado; o controller recria) | idem delete | `pods.delete` |

Delete de workload/pod usa preconditions UID + ResourceVersion. Run-now gera
Job `<cronjob>-manual-<sufixo>` com OwnerReference do CronJob.

No frontend, `ResourceActions.tsx` expõe por kind:

- **Pod**: Delete, Restart (com explicação do controller responsável),
  Port-forward, Exec, copy name/namespace, logs deep-link.
- **Deployment**: Restart, Scale com `[-] n [+]`/Apply, Delete, HPA warning.
- **StatefulSet**: Restart, Scale, Delete.
- **DaemonSet**: Restart, Delete (sem Scale — não se aplica).
- **Job**: Delete.
- **CronJob**: Suspend/Resume, Run now, Delete.
- Todos os botões são habilitados por decisão da matriz de permissões
  (`GET /permissions`) com badges explicativos; negado → desabilitado.
- Rollout status é observável na aba **Rollout** (conditions + contadores).

## 6. Ações em massa

- Checkbox por linha no `DataTable` (Pods e Workloads) com select-all.
- Toolbar contextual: "N selected" + View logs (1 Pod) + Delete selected +
  Clear selection.
- Bulk delete executa N deletes autorizados sequencialmente (cada um com
  fetch de UID/RV + `confirmed: true`), com `ConfirmDialog` listando os
  recursos, consequência e resultado por toast (`deleted N · failed list`).

## 7. Resource Workspace

- `components/workspace/ResourceWorkspaceProvider.tsx`: histórico global com
  voltar/avançar (botões + Alt/⌘+←/→ + Esc fecha), aba ativa preservada por
  entrada, reset na troca de geração.
- `components/workspace/ResourceWorkspace.tsx`: overlay central com
  `inset: 9vh 9vw` (~82% da viewport), backdrop escurecido/desfocado,
  header (navegação, kind, nome, namespace, favorito, fechar) e abas por tipo:
  - Pod: Overview | Logs | YAML | Events | Actions
  - Deployment: Overview | Pods | ReplicaSets | YAML | Events | Rollout | Actions
  - StatefulSet: Overview | Pods | PVCs | YAML | Events | Actions
  - DaemonSet/Job/ReplicaSet: Overview | Pods | YAML | Events | Actions
  - CronJob: Overview | Jobs | YAML | Events | Actions
  - Service: Overview | Endpoints | YAML | Events
  - Ingress: Overview | Rules | YAML | Events
  - Node: Overview | Conditions | YAML | Events
  - ConfigMap: Overview | Data | YAML · Secret: Overview (sem YAML)
  - demais: Overview (+ YAML quando disponível) (+ Events para namespaced)
- Relacionados clicáveis dentro do workspace: Pods/ReplicaSets de Deployment
  (via `related` do backend), Pods de ReplicaSet/Job, Jobs de CronJob, owner
  de Pod, PVCs de StatefulSet (por nome), Endpoints de Service — cada clique
  entra no histórico. Deep links de URL abrem o workspace.
- O drawer estreito de 25-30% foi removido de todas as páginas de recursos.

## 8. Feedback e confirmação

- `ui/Toast.tsx`: toasts semânticos (success/error/info/warning) com
  aria-live, auto-dismiss e dismiss manual.
- `ui/ConfirmDialog.tsx`: alertdialog para ações destrutivas com lista de
  recursos, nota de consequência, checkbox "cannot be undone" e option de
  digitar o nome (para operações especialmente perigosas).
- Cores semânticas de botão padronizadas: azul (primário), verde (Resume),
  vermelho (Delete), âmbar (Restart/Suspend) — via variantes já existentes do
  `Button`.

## 9. Segurança mantida

- Nenhuma ação bypassa o backend: CSRF por geração, `expectedGeneration`,
  preconditions, SSAR fail-closed revalidado no momento da execução,
  audit events allowlisted, Secret continuam sem valores/YAML, port-forward
  só loopback.
- `scripts/security_check.sh HEAD` segue sendo obrigatório antes de commit.

## 10. Testes

- Go: `go test ./...` → 1017 testes, `go vet ./...` limpo, `gofmt` limpo.
  Novos testes: `internal/services/actions/workload_actions_test.go`
  (restart sts/ds, delete workload, suspend/resume, run-now, random suffix) e
  allowlist atualizada (97 → 107 capabilities documentadas).
- Web: `npm run test` → 86 testes; `npm run lint` → 0 erros; `tsc -b` limpo;
  `npm run build` ok; Playwright e2e → 12 testes.
- Ajustes de testes existentes refletem a nova UX (workspace em vez de
  drawer, confirmação por dialog em vez de checkbox, filtro global).

## 11. Pendências / próximos passos

- YAML continua **somente leitura**: editar/aplicar manifesto exigiria novo
  contrato de backend (update/patch arbitrário) e política própria para
  Secrets — ficou fora do escopo por decisão.
- CPU/Memória nas linhas de Pods dependem do health do Metrics API; sem
  métricas as colunas ficam vazias (honesto, não mock).
- Helm releases e Gateway API continuam desabilitados no menu (como antes).
- Validação visual em 1366/1440/1920/2560 feita via breakpoints CSS
  (1100 px/760 px) e inset responsivo do workspace; recomenda-se um smoke
  manual no desktop Wails.
