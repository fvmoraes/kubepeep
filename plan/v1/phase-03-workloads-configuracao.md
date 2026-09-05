# Fase 3 — Workloads complementares e Configuration

> **Objetivo:** fechar os grupos **Workloads** (ReplicaSets) e **Configuration** (HPA, PDB, ResourceQuotas, LimitRanges, ServiceAccounts).
> **Prioridade:** P0. **Dependências:** Fase 1. **Complexidade agregada:** M.

## Tarefas

### Workloads

- [ ] **V3-01** `ReplicaSets`: adapter/service/handler (`/api/v1/replicasets` — name, namespace, desired/ready/available, owner, age); capabilities `replicasets.list/get`.
- [ ] **V3-02** Relação navegável (F9-33 subset): do Deployment → ReplicaSets do owner; do ReplicaSet → Pods com `owner.name` (a listagem de Pods já suporta filtro por workload owner).

### Configuration

- [ ] **V3-03** `HorizontalPodAutoscalers`: namespaced (name, namespace, target kind/name, min/max, current/desired replicas, conditions); capabilities `horizontalpodautoscalers.list/get`.
- [ ] **V3-04** `PodDisruptionBudgets`: namespaced (name, namespace, minAvailable/maxUnavailable, currentHealthy, desiredHealthy, disruptionsAllowed, expectedPods); capabilities `poddisruptionbudgets.list/get`.
- [ ] **V3-05** `ResourceQuotas`: namespaced (name, namespace, hard/used por quota — apenas pares aprovados no DTO; nunca valor de Secret), capabilities `resourcequotas.list/get`.
- [ ] **V3-06** `LimitRanges`: namespaced (name, namespace, tipo e limites resumidos); capabilities `limitranges.list/get`.
- [ ] **V3-07** `ServiceAccounts`: namespaced (name, namespace, secrets count apenas como número, age — **nunca** tokens/annotations); capabilities `serviceaccounts.list/get`; YAML bloqueado para ServiceAccounts com dados sensíveis? **Decisão:** YAML permitido (é o contrato do objeto), mas DTO de detalhe metadata-only e sem `secrets[]` contents.
- [ ] **V3-08** Teste de segurança específico: nenhuma resposta de ServiceAccounts/ResourceQuotas contém tokens, strings base64 ou valores de annotation arbitrários (assert no allowlist do DTO).

### Frontend (comum)

- [ ] **V3-09** Páginas via framework; nav habilita ReplicaSets, HorizontalPodAutoscalers, PodDisruptionBudgets, ResourceQuotas, LimitRanges, ServiceAccounts (labels §36: "Horizontal Pod Autoscalers", "Pod Disruption Budgets").
- [ ] **V3-10** Filtro de Workloads por kind já existente continua funcionando; ReplicaSets entram no kind map de `resourceEntryPath`/favoritos.

## Critérios de aceite

- Grupos Workloads e Configuration completos conforme §36 (ReplicaSets + 5 páginas de Configuration).
- Nenhum token/valor sensível em ServiceAccounts (teste de allowlist cobre).
- E2E: ReplicaSets lista→detalhe; HPA com métricas de status em 1280×720.

## Testes e rollback

- Padrão da Fase 1; rollback por família.
