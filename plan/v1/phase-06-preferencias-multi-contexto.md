# Fase 6 — Preferências, filtros avançados e multi-contexto

> **Objetivo:** estender o allowlist de preferências do backend para o estado de shell (fecha o desvio conhecido da sidebar), evoluir filtros/ordenação do servidor e especificar/executar o multi-contexto somente leitura.
> **Prioridade:** P1 (preferências) / P2 (multi-contexto — maior risco). **Dependências:** Fase 5 para colunas/recentes. **Complexidade agregada:** L.

## Tarefas

### Preferências de shell (backend)

- [ ] **V6-01** Estender o schema allowlisted de `/api/v1/preferences` (`internal/services/preferences` + validação em `internal/api/handlers`): `shell: { sidebarCompact: bool, collapsedGroups: []string (limite 32, ids do tree.tsx), columnVisibility: map<collection, []string> (catálogo da F5), recent: []ResourceRef (limite 20, LRU) }`.
- [ ] **V6-02** Regras de segurança do schema: nenhum valor livre além dos catálogos; ids/refs validados contra coleções existentes; sem namespace/label arbitrário (F9-25); versionação do schema com migração default-safe.
- [ ] **V6-03** Frontend: Sidebar e colunas passam a hidratar/salvar via preferências (substitui o estado in-memory); remover o desvio documentado em `components/Sidebar.tsx` e atualizar `web/src/security.test.ts` (a fronteira continua: **nada** de browser storage).

### Filtros e ordenação (servidor)

- [ ] **V6-04** Parser determinístico de filtros positivos/negativos e multi-termo para `search` das coleções (F9-23): ex. `payments !failed api` — contratos em `docs/api.md`; mensagens de erro acessíveis; probe de injeção (query params) coberto.
- [ ] **V6-05** Ordenação natural estável no servidor para `sort=name` (números dentro do nome) com tie-breaker por UID (F9-21) — aplicada nas coleções novas da v1 e retroalimentada em pods/workloads.

### Multi-contexto somente leitura (F7-03 / F9-66+)

- [ ] **V6-06** Especificação primeiro (ADR + product-spec): seleção de um conjunto de contextos para leitura sem alterar a seleção mutável principal; cada query carrega profile/contexto explícito; RBAC e budgets por contexto; cache por (contexto, geração).
- [ ] **V6-07** Backend: pool de clients por contexto com budgets isolados; geração estendida para (contexto, geração); capabilities resolvidas por contexto.
- [ ] **V6-08** Frontend: seletor multi-contexto (header) com escopo por contexto; páginas agregam com proveniência por contexto visível (F9-52 análogo); navegação preserva o conjunto (F9-12).
- [ ] **V6-09** Gate honesto: se o orçamento da v1 estourar, este bloco vira 1.1 — registrar decisão no `plan/README.md` (tabela de fases) e seguir para a Fase 7 sem ele.

## Critérios de aceite

- Sidebar/colunas/recentes sobrevivem a restart via preferências; `web/src/security.test.ts` continua verdecendo (nenhum browser storage).
- Parser de filtros: casos positivos/negativos/multi-termo em unit + e2e; nenhum novo parâmetro sem allowlist.
- Multi-contexto (se executado): nenhuma resposta mistura contextos sem proveniência; cancelamento troca de conjunto sem vazamento (goleak).

## Testes e rollback

- Backend: testes de schema/migração de preferências, parser, pool por contexto.
- Frontend: Vitest (hidratação de preferências), e2e do seletor.
- Rollback: preferências novas são aditivas e versionadas (defaults seguros); multi-contexto atrás de build/feature flag de produto com rollback para seleção única.
