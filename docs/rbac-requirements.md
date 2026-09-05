# Requisitos RBAC do KubePeep

> **Fonte única de verdade:** `internal/services/authorization/allowlist.go`. Este documento reflete a allowlist imutável e os requisitos mínimos por perfil de uso.

## 1. Modelo

- O KubePeep **nunca** usa credenciais próprias: toda chamada usa a identidade do kubeconfig selecionado (com `SelfSubjectRulesReview`/`SelfSubjectAccessReview` para descobrir capacidades).
- Toda capacidade é revalidada no servidor em cada operação mutável (guard), nunca apenas cacheada no frontend.
- A allowlist é fechada: capacidades fora da lista não são consultadas nem exibidas.

## 2. Perfis de uso

### 2.1 Somente leitura (dashboard/visualização)

Requisitos mínimos por namespace:

```text
namespaces.list                                    (cluster)
pods.list, pods.get, pods.watch                    (namespace)
pods.logs.get                                      (namespace, resourceName)
events.list, events.watch                          (namespace)
deployments.list/get/watch                         (namespace)
statefulsets.list/get/watch                        (namespace)
daemonsets.list/get/watch                          (namespace)
jobs.list/get/watch                                (namespace)
cronjobs.list/get/watch                            (namespace)
services/ingresses/endpointslices/configmaps/secrets
  (list/get/watch — ver allowlist para grupos/verbos exatos)
```

Com esse perfil o usuário vê dashboard, listas, detalhes (Secret apenas metadados), logs e métricas (quando a Metrics API permitir).

### 2.2 Ações mutáveis (opcionais, por namespace/recurso)

| Capacidade | Verbo Kubernetes | Uso na UI |
| --- | --- | --- |
| `deployments.restart` | `patch` deployments | botão Restart |
| `deployments.scale` | `update` deployments/scale | campo Scale |
| `statefulsets.scale` | `update` statefulsets/scale | campo Scale |
| `pods.delete` | `delete` pods | Delete pod |
| `pods.exec.create` | `create` pods/exec | Terminal exec |
| `pods.portforward.create` | `create` pods/portforward | Port-forward |

Sem a capacidade, o controle correspondente aparece desabilitado com o motivo — nunca escondido silenciosamente.

## 3. Regras de exibição

1. **Negado ≠ zero.** Blocos parcialmente negados exibem estado `denied` distinto, sem inferir valor.
2. `resourceName` só é usado quando a política da capacidade é `ResourceNameTarget`.
3. Escopo da consulta é sempre a interseção escopo ativo ∩ filtros; a query jamais expande o escopo.
4. Toda mutação exige: geração atual válida, `ExpectedResourceVersion`/`ExpectedUID` quando aplicável, confirmação explícita e idempotency key.

## 4. Auditoria

Cada ação registra evento sanitizado (`internal/services/actions/audit.go`): timestamp, operação, contexto, namespace, recurso, duração e código de erro — nunca corpo, comando, saída ou ticket.

## 5. Recursos cluster-scoped (ADR 0006)

Capacidades cluster-scoped são próprias e nunca derivam de um scope
namespaced. `nodes.list`/`nodes.get` são os primeiros exemplos: exigem apenas
contexto válido e o verbo Kubernetes correspondente; `get` forma produto com
`resourceName` (policy `target`). Um operador sem `list nodes` continua com o
restante do produto útil, e negação de Nodes não bloqueia o fluxo namespaced.

## 6. Checklist de uma família nova

Toda família nova (namespaced ou cluster-scoped) segue o mesmo caminho —
usar Nodes (`cb86d6b`) como fonte do guia, não abstrações de planos antigos:

1. **Catálogo** — coleção/GVR em `internal/services/resources` (namespaced:
   `service.go`; cluster-scoped: `cluster.go`) + regras de sort/status em
   `options.go` + capabilities `list`/`get` em
   `internal/services/authorization/allowlist.go` (ID ≠ plural Kubernetes).
2. **DTO/porta** — DTOs de lista/detalhe fechados com limites explícitos e
   conversores testados; política de YAML definida antes de expor rota
   (`MarshalReadOnlyYAML` ou documento curado via `MarshalYAMLDocument`).
3. **Runtime** — lister/getter typed em
   `internal/integration/kubernetesruntime` (`resources_backend.go` +
   `resources_adapter.go`), reutilizando `Collect`/`clusterCollect` com o
   budget de janela configurável.
4. **Handler/wiring** — métodos em `internal/api/handlers/resources.go`
   (`handleList`/`handleClusterList`, `detail`/`clusterDetail`,
   `yaml`/`clusterYAML`), rotas em `routes.go` e `allowedMethods` completos
   (GET/HEAD; POST/PUT/DELETE separados).
5. **Cliente** — tipos em `web/src/api/types.ts` e funções em `client.ts`
   com geração esperada e AbortSignal.
6. **Página/rota/nav** — página com o resource framework (`ResourcePage`,
   `ResourceListControls`, `DataTable`, `CollectionFooter`, `Drawer`,
   `YamlViewer`), rota em `App.tsx`, item com `path` em
   `navigation/tree.tsx` e ajuste do teste de catálogo da paleta.
7. **Testes** — unitários de DTO/contrato, integração handler/runtime com
   cliente fake (403/unknown/vazio distintos) e E2E mockado da jornada
   lista → detalhe → YAML autorizado.
