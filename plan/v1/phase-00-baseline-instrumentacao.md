# Fase 0 — Correção funcional P0, baseline e instrumentação

**Prioridade:** P0, gate de entrada e **bloqueante**. **Entrada:** base atual. **Desbloqueia:** F1–F6. **Matriz:** R01, R02, D01–D05, D07, T05, T06.

Duas frentes nesta fase. Primeiro, **correção funcional**: o usuário não pode conviver com o bug de abrir Pods e ver a tela sem nada, nem com cliques que não fazem nada — isso é revisão total e bloqueante. Segundo, **medição antes de mudar algoritmo**: baseline, instrumentação e laboratório de benchmark. O cursor opaco server-side é revisitado como ajuste completo (TTL, LRU, budget, expiração, 410) — não como entrega presumida.

## Tarefas

- [ ] **F0-01 (R01) — Corrigir o bug de Pods sem dados (P0, bloqueante).** Reproduzir o cenário de abrir Pods e a tela ficar sem nada: varrer a cadeia completa — query React (`web/src/components/ResourcePages.tsx`), transporte web/desktop (`web/src/api/`), handler (`internal/api/handlers/`), autorização, cursor/paginação (`internal/services/resources/`), cliente Kubernetes — até isolar a camada responsável. Corrigir sem mascarar: estados proibido/unknown/parcial permanecem distintos de vazio. Regressão E2E e de componentes cobre o cenário em web e desktop, com contextos, scopes e permissões variados.
- [ ] **F0-02 (R02) — Varredura total de cliques não funcionais (P0).** Inventariar **todos** os elementos clicáveis da interface — sidebar, abas horizontais, ações de linha e em massa, links de detalhes/relacionados, filtros, ordenação, colunas, paginação, refresh, paleta, confirmações — e corrigir cada clique que não executa sua função. Regra: todo clique funciona, ou o elemento está explicitamente desabilitado com tooltip explicando o motivo (permissão, condição, indisponibilidade). Registrar o inventário por tela em `docs/ui-ux-refinement.md` como base da auditoria da F4/F7.
- [ ] **F0-03 — Inventário UX baseline.** Medir a proporção filtros × conteúdo por tela, listar tamanhos/tipografia fora de tokens (`web/src/tokens.css`), filtros que não alteram a query e desvios visuais. Saída alimenta a F4.
- [ ] **F0-04 — Completar a instrumentação de leitura (D01).** Revisar e completar métricas: contador por estratégia (global × fanout), cursor store (entradas/bytes/hit/miss/expirados), watch (ativas/reconexões/410/eventos/lag), 429 e throttle client-side. Documentar em `docs/observability.md`.
- [ ] **F0-05 — Métricas de UX e budgets (D03).** time_to_first_row, time_to_page_complete, latência de filtro/sort, linhas renderizadas. Definir os budgets por tamanho de cluster (pequeno/médio/grande) conforme o contrato.
- [ ] **F0-06 — Traces OTel (D04).** Spans de `resources.list` (`.global`/`.fanout`/`.origin`), `resources.merge`, `cursor.get/put`, `watch.connect/reconnect`, `cache.*` com atributos seguros e sem alta cardinalidade sensível.
- [ ] **F0-07 — Laboratório de benchmark (D02).** Estender `test/kind/` com geração de datasets (1/10/25/50/100/200 namespaces × 10–500 pods), RBAC variado (global, namespace-only, misto, authorization unavailable, watch on/off), latência simulada (0–200 ms) e injeção de erros (429/410/timeout/reset). Script idempotente e repetível.
- [ ] **F0-08 — Baseline documentado (D02/D07).** `docs/performance-baseline.md` com TTFB, first row, full page, contagem de requests, bytes, over-fetch ratio, memória, goroutines e memória de cursor por cenário — **antes** de qualquer mudança de estratégia das fases seguintes.
- [ ] **F0-09 — Revisão completa do cursor opaco (D05).** Ajustar e testar o ciclo inteiro: TTL, LRU, budget de memória, binding de geração/consulta, expiração recuperável, 410 do continue token, purge; regressão de fan-out largo (200 namespaces × página 100) e benchmarks `BenchmarkCursorStore`.

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| Abrir Pods no cenário que exibia tela vazia | dados exibidos; se houver negativa/indisponibilidade, estado honesto e explicável — nunca vazio indevido |
| Clicar em cada item de sidebar, aba, ação, link e filtro | função executa ou elemento desabilitado com motivo; zero cliques mortos no inventário |
| Baseline registrado antes das mudanças | `docs/performance-baseline.md` reproduzível por script com números por cenário |
| 200 namespaces × página 100 | sem cursor acima do limite; `LIMIT_EXCEEDED` não reproduzível; over-fetch ratio medido |
| Métricas/traces inspecionáveis | contadores com labels previstos; nenhum atributo sensível (pod, UID, token, identidade) |
| `rtk make verify` + `rtk make test-race` | verdes no fechamento da fase |

**Saída:** zero defeitos funcionais conhecidos de leitura/cliques + baseline comparável + instrumentação estável. **Rollback:** instrumentação é aditiva; correções funcionais mantêm contratos e não removem limites de segurança.
