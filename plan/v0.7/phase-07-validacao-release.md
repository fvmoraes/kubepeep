# Fase 7 — Validação comparativa e preparação da release

**Prioridade:** gate final. **Entrada:** F0–F6. **Matriz:** D35, R01–R03 (regressão), X15/X16, T01–T06; critérios consolidados das referências (§77–79 de performance, §30 de UI/UX).

A v0.7 só fecha com a prova comparativa de que a experiência mudou — e com zero regressão funcional, incluindo os bugs corrigidos na F0.

## Tarefas

- [ ] **F7-01 — Benchmark comparativo antes/depois.** Rodar a matriz completa da F0 (namespaces 1–200, pods, page sizes 25/50/100, RBAC variado, latência 0–200 ms, erros 429/410/timeout/reset) registrando TTFB, first row, full page, contagem de requests, bytes, over-fetch ratio, memória, goroutines, memória de cursor/cache, 429, cache hit e reconexões. Comparativo publicado em `docs/performance-baseline.md` (seção after).
- [ ] **F7-02 — Testes de regressão fundamentais.** Paginação com `limit=37` sobre 1000 objetos em 10 namespaces (asc/desc por namespace+name e age; sem duplicatas/gaps); mutações durante a paginação (contrato de snapshot); cursor expirado/TTL (erro recuperável, sem stack trace); memory bound (200 namespaces; goroutines retornam ao baseline); `-race` na suíte.
- [ ] **F7-03 — Critérios consolidados.** Performance: P0 (cursor sem DTOs; `LIMIT_EXCEEDED` irreproduzível; TTL; memory bound; generation fence; timeout normalizado), P1 (página 1 sem varrer tudo; lazy merge; over-fetch medido; cancelamento; coalescing; 429 sem storm; 410 recuperável), P2 (watch saudável sem polling; cache limitado; virtualização; infinite query; refresh sem flicker; Metrics API não bloqueia lista; progressividade). UX: checklist §30 da referência. Regressão funcional: R01/R02 não reproduzem.
- [ ] **F7-04 — Verificação integrada.** `rtk make verify`, `rtk make test`, `rtk make test-e2e`, `rtk make test-race`, `rtk make format-check lint typecheck`, `rtk make build smoke`, `rtk make build-desktop` (dependências nativas instaladas).
- [ ] **F7-05 — Auditoria UX final.** Percorrida nas 4 resoluções; catálogo de ações por kind; RBAC na UI; cliques reauditados contra o inventário da F0; docs atualizados (`docs/api.md`, `docs/observability.md`, `docs/ui-ux-refinement.md`, baseline).
- [ ] **F7-06 — Segurança.** `rtk scripts/security_check.sh HEAD` em todos os commits; nenhuma sentinela de dados internos em logs/persistência/artefatos; Secret continua metadata-only; cursor/cache nunca em disco.
- [ ] **F7-07 — Preparação da release.** Candidate, notas e publicação são decisão explícita do usuário — apenas commit; nunca push autônomo. Pendências de ambiente/CI ficam registradas em [`03-evidencias-execucao.md`](03-evidencias-execucao.md).

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| comparativo antes/depois por cenário | ganhos consistentes com os budgets; regressões explicadas ou corrigidas |
| bug de Pods sem dados (R01) | não reproduz em web/desktop; regressão E2E cobre |
| varredura de cliques (R02) | zero cliques mortos no inventário final |
| scope default (R03) | carregamento obrigatório na abertura e troca de contexto comprovado |
| `LIMIT_EXCEEDED` / cursor pesado | não reproduzível no cenário original de fan-out largo |
| 429 sustentado / 410 Gone | sem amplificação; recuperação automática com estados honestos |
| suite completa local | verify/test/test-e2e/test-race/build/smoke verdes |

**Saída:** v0.7 pronta com evidência comparativa e sem defeitos funcionais conhecidos. **Rollback:** candidato descartável; publicação só por decisão explícita do usuário.
