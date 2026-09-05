# ADR 0006 — Contrato de recursos cluster-scoped

- Status: aceito
- Data: 2026-09-05
- Fase: v1/F1 (V1-02, V1-03, V1-04)
- Matriz: R02 (Nodes), R23–R27/R30/R32–R34 (famílias futuras da mesma forma)

## Contexto

Todas as coleções de recursos existentes (`/workloads`, `/pods`, `/services`,
`/configmaps`, …) são namespaced: o handler exige um scope ativo
(`len(resolution.Namespaces) > 0`), o cursor mistura estado por namespace/kind
e a cobertura reporta contagens de namespaces. Recursos cluster-scoped —
começando por **Nodes** — não podem herdar esse modelo sem violar três regras
do plano v1:

1. contexto válido basta; nenhum scope de namespaces é exigido;
2. queries cluster-scoped rejeitam filtro `namespace`;
3. `meta.page.filterScope` descreve filtro/ordenação (`page`/`collection`),
   não o escopo Kubernetes — não existe `filterScope: "cluster"`.

## Decisão

### Seleção e rota

- Leitura cluster-scoped exige binding válido (profile, contexto, geração) e
  **não** exige namespaces no scope. Handler usa um leitor dedicado;
  namespaces ativos continuam disponíveis para as rotas namespaced.
- Rotas: `GET /api/v1/{collection}` e `GET /api/v1/{collection}/{name}`;
  YAML apenas por ação explícita (`GET .../{name}/yaml`) quando a família
  tiver política YAML aprovada. `HEAD` cobre as mesmas rotas via
  `allowedMethods`. Filtro `namespace` é rejeitado com `VALIDATION_FAILED`.

### Cursor

A identidade do cursor permanece `(query hash, contexto, scope local,
geração)` e o estado interno carrega a origem única cluster-scoped
(`namespace == ""`, GVR da família). Nenhum scope artificial entra na
identidade; o scope local armazenado é o mesmo do shell (nome/origem do scope
salvo), não um namespace Kubernetes.

### Cobertura (V1-03)

Listas cluster-scoped não simulam fan-out por namespace:

- `coverage.requestedNamespaces` e `coverage.completedNamespaces` são **0**;
- `coverage.deniedNamespaces` é sempre `[]`;
- falhas parciais chegam em `coverage.failed` com `namespace` **ausente**
  (nível cluster) e código sanitizado (`FORBIDDEN`,
  `AUTHORIZATION_UNAVAILABLE`, `UPSTREAM_TIMEOUT`, `CLUSTER_UNAVAILABLE`).

Estados autoritativos permanecem: `list` negado em todas as origens retorna
403 (`FORBIDDEN`); decisão unknown retorna 503
(`AUTHORIZATION_UNAVAILABLE`); nada disso vira “lista vazia”.

### Registro de família (V1-04)

Cada família cluster-scoped segue o mesmo checklist das namespaced: coleção e
GVR no catálogo `internal/services/resources`, capabilities `list`/`get` na
allowlist de `internal/services/authorization` com `ScopeCluster`,
DTO/porta em `internal/services/resources`, integração typed client em
`internal/integration/kubernetesruntime`, handler + `allowedMethods`, cliente
React e página/rota/nav. `watch` não entra sem implementação e consumo
justificados.

### Campos e YAML (V1-06)

Listas expõem apenas campos úteis e seguros; detalhes limitam condições,
capacidades e taints a contagens máximas explícitas. YAML de família
cluster-scoped existe somente quando um documento seguro for definido
explicitamente (sem objeto cru, annotations arbitrárias ou material de
autenticação), com omissões rotuladas no próprio documento. A UI só oferece
YAML que o backend serve.

## Consequências

- Nodes (F1) é a família de referência; Storage (F2), RBAC/CRDs/Classes
  (F4) reutilizam o mesmo caminho sem novo contrato.
- `filterScope` continua sem significado de escopo Kubernetes; qualquer
  indicador de escopo usa campo próprio e compatível.
- Coleções namespaced existentes não mudam de envelope nem de cursor.
