# Fase 1 — Fundação backend para coleções expandidas

> **Objetivo:** estabelecer o caminho repetível para adicionar uma família de recursos Kubernetes (adapter → service → handler → capabilities → cliente TS → página) usando **Nodes como piloto ponta-a-ponta**.
> **Prioridade:** P0. **Dependências:** nenhuma. **Complexidade agregada:** L.

## Tarefas

### Backend

- [ ] **V1-01** Extrair o padrão atual de `internal/adapters/kubernetes` (pods/workloads) em um helper de listagem tipado: budgets de páginas, `continue` token, cancelamento por geração, contagem de cobertura por namespace e por recurso cluster-scoped.
- [ ] **V1-02** Definir o contrato de coleção **cluster-scoped** no envelope: `meta.page.filterScope: "cluster"`, sem parâmetro `namespace`, coverage sem `deniedNamespaces` (documentar em `docs/api.md`).
- [ ] **V1-03** Implementar `Nodes` como piloto: adapter (`ListNodes` com FieldSelector), service (DTO allowlisted: name, status conditions resumidas, roles, version, kubelet ready, age, internal IP; sem endereços internos sensíveis além do IP do cluster), handler `GET /api/v1/nodes`, `GET /api/v1/nodes/{name}`, `GET /api/v1/nodes/{name}/yaml`.
- [ ] **V1-04** Registrar capabilities da família em `internal/services/authorization/matrix.go`: `nodes.list`, `nodes.get`, `nodes.watch` (IDs e verbos do RBAC real).
- [ ] **V1-05** Atualizar `allowedMethods`/`resourceAllowedMethods` em `routes.go` para as rotas novas.
- [ ] **V1-06** Testes: unitário do adapter (fake client), wire do handler (envelope, cursor, 403 por capability, generation mismatch), teste de redaction/allowlist do DTO.

### Frontend

- [ ] **V1-07** `web/src/api/types.ts` + `client.ts`: `NodeResource`, `NodeDetail`, `getNodes`, `getNode`, `getNodeYAML` (padrão das coleções existentes).
- [ ] **V1-08** Página Nodes com o resource framework: colunas Name, Status (Ready condition → StatusBadge), Roles, Version, Age, Internal IP; detail com conditions, capacity/allocatable (CPU/memória/pods) e taints.
- [ ] **V1-09** Habilitar o item Nodes em `web/src/navigation/tree.tsx` (`path: '/nodes'`) + rota `/nodes` e `/nodes/:name`; remover item da lista de desabilitados.
- [ ] **V1-10** Command palette e favoritos reconhecem a coleção (`resourceEntryPath`).

### Documentação

- [ ] **V1-11** `docs/api.md`: seção da coleção `nodes`; `docs/architecture.md`: anotação do padrão de família; `docs/rbac-requirements.md`: capabilities novas.
- [ ] **V1-12** ADR curto (`docs/decisions/0006-…`): contrato de coleção cluster-scoped e o checklist "adicionar uma família" (o conteúdo da Fase 1 vira o guia).

## Critérios de aceite

- `GET /api/v1/nodes` respeita RBAC: sem `nodes.list` → 403 com capability no payload de erro; com RBAC parcial, coverage reflete o que foi coletado.
- Nenhum namespace scope é exigido para Nodes; a UI não mostra filtro de namespace nesta página.
- Página Nodes funcional em 1280×720 e 1920×1080; e2e cobre lista → detalhe → YAML (com `X-KubePeep-CSRF` e geração).
- `make test`, `make test-e2e`, lint, typecheck e build verdes.

## Testes e rollback

- Testes por camada como nas coleções existentes (padrão `pods`); e2e Playwright com cluster mockado.
- Rollback: cada família é um conjunto isolado de arquivos/rotas; revert do commit da família não afeta as demais.
