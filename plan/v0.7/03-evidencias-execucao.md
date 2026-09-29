# Evidências de execução — plano v0.7

Registro de fechamento das fases F0–F7. Cada linha segue a regra da [matriz](01-matriz-de-entregas.md):
`ID → commit → comando/cenário → resultado`. A coluna **Commit** identifica o
código/binário sob teste; o commit que contém esta tabela versiona o resumo
documental. Evidências contêm apenas comando, resultado resumido e commit —
nenhum dado privado, log cru ou screenshot.

Classificação usada neste registro:

- **IMPLEMENTADO:** entrega e aceite da fase possuem evidência compatível;
- **ALTERADO:** entrega funcional aceita como fechada por decisão explícita,
  com validação complementar transferida e rastreada em fase posterior;
- **PENDENTE:** entrega ainda não executada na fase à qual foi atribuída.

## Revisões documentais (não fecham fases funcionais)

- `f7cd651`: avaliação inicial, cinco links corrigidos; 34 links relativos válidos, diff-check e gate de segurança aprovados.
- Revisão C01–C06: contratos incorporados à matriz e F0–F7; 47 links relativos válidos, 74 tarefas anteriores e todos os IDs da matriz preservados; F2-11/F2-12 explicitam integrações dos requisitos existentes. Diff-check aprovado. O commit que introduz esta entrada registra a revisão; nenhuma tarefa funcional foi marcada como concluída.

## Fase 0 — Correção funcional P0, baseline e instrumentação

**Status: FINALIZADA em 2026-09-12.** Por decisão explícita do usuário, as
validações complementares de F0-01, F0-05 e F0-09 foram transferidas para
F1-11, F1-12 e F1-13, sem alegação de que já foram executadas.

Baseline numérica e protocolo: [`docs/performance-baseline.md`](../../docs/performance-baseline.md).

| ID | Classificação | Commit | Evidência | Resultado |
| --- | --- | --- | --- | --- |
| F0-01 | **ALTERADO** | `6a8eead` | `rtk make verify`; `./test/kind/harness.sh app-e2e ./dist/kubePeep` | correção e estados honestos passaram em componentes, Playwright web, transporte `desktop-bridge` simulado e black-box Kind; Wails/WebView nativo foi transferido para F1-11. |
| F0-02 | **IMPLEMENTADO** | `6a8eead` | inventário por tela em `docs/ui-ux-refinement.md`; `rtk make verify` | defeitos encontrados foram corrigidos; todo clique inventariado executa função ou fica desabilitado com motivo; nenhum clique morto conhecido permanece. |
| F0-03 | **IMPLEMENTADO** | `6a8eead` | auditoria estática e geométrica em `docs/ui-ux-refinement.md`; `rtk make verify` | proporção filtros/conteúdo medida em três viewports, exceções tipográficas inventariadas e filtros ligados ao efeito real; riscos visuais de largura estreita ficaram explícitos para F4. |
| F0-04 | **IMPLEMENTADO** | `6a8eead` | `rtk make verify`; `rtk make test-race`; inspeção de `/metrics` nos testes | estratégia LIST, cursor, watch, 429, bytes e throttle possuem séries fechadas e testadas; resource cache permanece reservado para integração real na F2 (C04). |
| F0-05 | **ALTERADO** | `6a8eead` | `web/src/observability/uxMetrics.test.ts`; `rtk make verify` | métricas por tentativa e budgets foram implementados/testados; coleta browser/desktop real foi transferida para F1-12. |
| F0-06 | **IMPLEMENTADO** | `6a8eead` | testes de `internal/observability`; `rtk make verify`; `rtk make test-race` | spans dos componentes existentes aprovados com allowlist sem identidade sensível; contrato F2 definido sem telemetria simulada. |
| F0-07 | **IMPLEMENTADO** | `6a8eead` | `rtk make benchmark`; `rtk make benchmark-matrix`; `rtk test/kind/harness.sh static` | 10 cenários representativos e 47 de matriz concluíram sem `unstable`/`unexpected_error`; dataset explícito cobre 1–200 namespaces × 10–500 Pods e não é aplicado pelos gates. |
| F0-08 | **IMPLEMENTADO** | `6a8eead` | JSONs sanitizados + `docs/performance-baseline.md` | baseline registra SHA limpo, ambiente, p50/p95, requests, bytes, over-fetch, alocação, heap, delta de goroutines, cursor e watch, separando sintético, Kind e Wails. |
| F0-09 | **ALTERADO** | `6a8eead` | benchmarks CursorStore/merge; matriz sintética C02/410 | TTL, LRU, budgets, binding, purge, 410 e cenários sintéticos passaram; cenário Kind real de 200 namespaces foi transferido para F1-13. |

### Gates de fechamento executados

| Gate | Resultado |
| --- | --- |
| `rtk make format-check` | aprovado; nove warnings ESLint conhecidos, sem erro |
| `rtk make verify` | aprovado após restaurar `node_modules` pelo lockfile; lint, tipos, unidades/integração, Playwright, build e smoke verdes |
| `rtk make test-race` | aprovado |
| `rtk make build smoke` | aprovado |
| `rtk scripts/security_check.sh HEAD` | aprovado |
| `rtk npm audit --omit=dev` em `web/` | aprovado; zero vulnerabilidades de produção |
| `rtk git diff --check` | aprovado |
| `rtk test/kind/harness.sh validate` | matriz Kubernetes/RBAC real aprovada |
| `rtk test/kind/harness.sh app-e2e ./dist/kubePeep` | black-box real aprovado; cluster dedicado preservado |

## Fase 1 — Paginação estratégica e uso responsável do API Server

**Status: FINALIZADA em 2026-09-14.** Por decisão explícita do usuário, a
fase é encerrada com a regressão de UX do porte médio registrada para trabalho
posterior, sem reinterpretá-la como aprovação dos budgets.

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F1-01 | `5051bf2` | `TestSelectPaginationStrategyPreservesOrderingContract`; `rtk make benchmark[-matrix]` | **IMPLEMENTADO:** `GlobalNative`, `NamespaceSequential` e `LazyMerge` são decisão interna; cursor público permanece opaco e sort global não é anunciado. |
| F1-02 | `5051bf2` | `TestNamespaceSequentialStopsAfterFillingPage` | **IMPLEMENTADO:** 50 namespaces/página 50 consulta somente a primeira origem elegível; ordem desc/unprovada permanece page-local. |
| F1-03 | `5051bf2` | `TestLazyMergeLimitsOriginWindow`; `TestCollectPaginatesLargeFanoutWithoutGapsOrDuplicates` | **IMPLEMENTADO:** heap e chunks limitados; 50 itens com 100 origens consultam no máximo 52; dataset estável converge sem gaps/duplicatas. |
| F1-04 | `5051bf2` | `BenchmarkCollectWorkerPoolFanout` (10×, 32 origens, 1 ms) | **IMPLEMENTADO:** 4/6/8 = 9,20/7,15/4,82 ms/op nesta coleta local; default 4 preservado até medição real de pressão/429 justificar mudança. |
| F1-05 | `5051bf2` | `TestRequestCoalescerRunsIdenticalOverlapOnce`; cancelamento do último waiter | **IMPLEMENTADO:** identidade inclui seleção, resolução, coleção/GVR, filtros, sort e cursor; somente trabalho sobreposto é compartilhado. |
| F1-06 | `5051bf2` | `rtk make test-race`; testes Bridge/AbortSignal; Playwright desktop-bridge | **IMPLEMENTADO:** cancelamento web/Wails alcança o contexto do handler; race gate completo aprovado sem trabalho órfão. |
| F1-07 | `5051bf2` | `ResourceListControls.test.tsx`; cenário Playwright de filtros | **IMPLEMENTADO:** fluxo explícito Apply mantém seis mudanças de `deploy` locais e dispara uma aplicação. |
| F1-08 | `5051bf2` | `selectors_test.go`; `TestCollectPropagatesSelectorsToLister`; matriz em `docs/api.md` | **IMPLEMENTADO:** label/field selectors normalizados chegam a todos os adapters; campos fora do allowlist falham fechados e filtros sem equivalente permanecem page-local. |
| F1-09 | `5051bf2` | testes existentes de authorization cache + `discovery_cache_test.go` | **IMPLEMENTADO:** discovery tem TTL 10 min, identidade completa, coalescing, invalidação por geração/recurso e não cacheia erros. |
| F1-10 | `5051bf2` | `retry_test.go`; `rtk make benchmark[-matrix]` | **IMPLEMENTADO:** Retry-After/backoff+jitter/teto/cancelamento e redução 4→2; matriz: 29 ok + 18 erros esperados, over-fetch p95 máximo 1,6×, sem outcomes instáveis. |
| F1-11 | `5051bf2` | Wails 2.15/WebKitGTK em Wayland; inspeção AT-SPI do WebView real | **EXECUTADO:** dados = 100 linhas; vazio = `No matching resources`; parcial = valores seguros + `Partial collection errors`; negação = `FORBIDDEN · kp-denied`; fail-closed = `AUTHORIZATION_UNAVAILABLE`. O transporte simulado permanece somente regressão complementar. |
| F1-12 | `5051bf2` | Chromium real, 1 aquecimento + 5 amostras por porte; `window.__KUBEPEEP_UX_METRICS__` | **EXECUTADO COM REGRESSÃO MÉDIA:** pequeno e grande passam; no médio a página completa passa, mas primeira linha, filtro e sort excedem os budgets. Números abaixo; esta coleta não usa o runner sintético como pintura. |
| F1-13 | `5051bf2` | Kind `kubepeep-f4`, dataset 200×10, API/counters reais | **EXECUTADO:** 3.107 objetos aplicados em 95,33 s; global e restrito ≤100, rejeição >100, cursor, over-fetch e memória comprovados; cleanup exato concluído em 112,79 s, com zero namespaces/Pods do dataset e cluster preservado. |

### Coleta real F1-11–F1-13 — 2026-09-14

Ambiente: Linux/amd64, Kind v1.35.0 de nó único, Wails 2.15 com
WebKitGTK/GTK sobre Wayland e Chromium Playwright sem throttle artificial. A
coleta UX descartou uma execução de aquecimento e usa o maior valor das cinco
amostras seguintes como p95 conservador:

| Porte real | Escopo/itens | primeira linha p95 | página completa p95 | filtro p95 | sort p95 | linhas | Resultado contra `docs/observability.md` |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| pequeno | 1 namespace / 10 Pods | 22,5 ms | 13,5 ms | 13,3 ms | 31,4 ms | 10 | passa |
| médio | 50 namespaces / 500 Pods | 826,6 ms | 811,8 ms | 875,6 ms | 1.126,5 ms | 100 | página/linhas passam; primeira linha, filtro e sort falham |
| grande | `all`, 209 namespaces resolvidos / 2.023 Pods | 39,7 ms | 30,3 ms | 18,9 ms | 23,7 ms | 100 | passa |

No C02 real, a primeira página global retornou 100 itens, cursor opaco e
`complete=false` em 13,6 ms. O percurso completo produziu 2.023 itens únicos
em 21 páginas, sem duplicatas; duas páginas medidas registraram 200 itens
recebidos e 200 retornados (over-fetch 1,0×), `cursor_hits_total=1`, duas
entradas/515 bytes. O processo permaneceu em aproximadamente 58–59 MiB RSS e
17 threads. O escopo restrito com 50 namespaces devolveu página de 100; uma
LIST em escopo de 101 namespaces falhou fechada com HTTP 429/
`LIMIT_EXCEEDED`. Dos 2.000 Pods do dataset, 93 ficaram `Running` e 1.907
`Pending` pelo limite de scheduling do nó único; todos existiam no API server,
que é o objeto desta medição. A limpeza usou exatamente
`test/kind/.state/dataset-200x10.yaml`: zero namespaces e zero Pods do dataset
restaram, e `kubepeep-f4-control-plane` permaneceu `Ready`.

## Fase 2 — Estado orientado a snapshot: cache sob demanda + WATCH

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F2-01 | `WORKTREE@1935ba0` | `cache_test.go`, `collection_cache_test.go`, `TestResourceBackendReusesAuthorizedPageAndInvalidatesOnChange`; `go test -race ./internal/services/resources ./internal/integration/kubernetesruntime` | **EM ANDAMENTO:** snapshots e páginas autorizadas integrados ao backend; página completa pequena é servida de WATCH conectado sem novo LIST. Budgets separados de 128 MiB (snapshot) e 64 MiB (páginas), com fencing, LRU e Secret excluído. Falta eviction global entre cache/watch/cursor e carga real. |
| F2-02 | `WORKTREE@1935ba0` | `cache_test.go`; `web/e2e/phase02.spec.ts`; `npm run test:e2e -- e2e/phase02.spec.ts`; `TestWatchCoverageRequiresConnectedStream` | **IMPLEMENTADO:** estados explícitos, geração e 403 invalidam/fenceiam; Pods reaparece do cache enquanto a segunda request fica pendente; desconexão do watch torna snapshot stale. Teste de browser é mockado, não medição de <100 ms no cluster real. |
| F2-03 | `WORKTREE@1935ba0` | `TestWatchManagerKeepsIdleSourceAndReplaysItsCurrentSnapshot`; `TestWatchManagerDoesNotShareDifferentEffectiveOrigins`; `TestApplyStreamEventToSnapshotMaintainsLevelState` | **EM ANDAMENTO:** worker por identidade completa, origens efetivas, idle de 45 s e DELETE de Events identificado; falta medir reutilização de watch na troca de telas com cluster real. |
| F2-04 | `WORKTREE@1935ba0` | `TestWatchBookmarkAdvancesReconnectCheckpointWithoutUIEvent`; `TestWatchStreamRunProcessesAddsDeletesAndBookmarks`; `go test -race ./internal/services/resources ./internal/integration/kubernetesruntime` | **IMPLEMENTADO:** o adapter solicita bookmarks, encaminha apenas RV válido ao worker, que reconecta desse checkpoint sem renderizar evento extra; reconexão sem bookmark continua usando o RV anterior. |
| F2-05 | `WORKTREE@1935ba0` | `TestSubscriptionCoalescesResourceBurstButPreservesEvents` (10k); `ResourceLiveUpdates.test.tsx` (10k, uma revalidação); `npm test -- --run` (113 testes) | **EM ANDAMENTO:** fila 1 MiB/1000 eventos, coalescing level-driven e Events cronológicos; janela de 2 s evita LIST a cada 250 ms. Falta perfil de memória/estabilidade com rajada real. |
| F2-06 | `WORKTREE@1935ba0` | `docs/observability.md`; TTL de autorização 30–60 s; discovery da Metrics API 10 min; métricas 8 s | **EM ANDAMENTO:** política documentada e implementada para fontes indicadas; falta verificar cache de versão Kubernetes e invalidação completa por contexto. |
| F2-07 | `WORKTREE@1935ba0` | `Dashboard.test.tsx`; `Dashboard.tsx`; Playwright 14 testes antes do novo cenário | **EM ANDAMENTO:** summary antecede métricas/namespace-health e log scan é explícito; falta confirmar Node health/storage no Tier 2 e medir caminho crítico real. |
| F2-08 | `WORKTREE@1935ba0` | `TestStreamingListFastPathAndUnsupportedFallback`; `resources_watch_extra_test.go`; flag `KUBEPEEP_STREAMING_LISTS` desligada por padrão | **IMPLEMENTADO:** fast path inicial opcional, detecção de unsupported e fallback clássico; sem dependência de cluster novo. O E2E real posterior exercitou o caminho padrão, não a flag ligada. |
| F2-09 | `WORKTREE@1935ba0` | `scheduler_test.go`; scheduler visível/especulativo com concorrência e QPS separados; hooks 429/timeout/latência | **EM ANDAMENTO:** LIST de recursos usa budget global; a integração do prefetch automático de página seguinte veio na rodada F3-05 abaixo. Outros produtores e carga real seguem pendentes; AIMD segue F6. |
| F2-10 | `WORKTREE@1935ba0` | `TestWatchCreationResourceExpiredRelistsWithoutDroppingSubscription`; `TestResourceWatchPortRestartsExpiredPaginatedList`; `client.test.ts` | **IMPLEMENTADO:** 410 reinicia LIST/continuation e WATCH no mesmo worker; UI renova primeira página uma vez e avisa discretamente. |
| F2-11 | `WORKTREE@1935ba0` | `collection_cache_test.go`; `cache_test.go`; `docs/observability.md`; `cache.snapshot` e `cache.apply_event` reais | **EM ANDAMENTO:** séries de hit/miss/bytes/eviction/invalidação integradas, sem labels sensíveis; falta teste OTLP integrado dos novos spans. |
| F2-12 | `WORKTREE@1935ba0` | `TestResourceBackendReusesAuthorizedPageAndInvalidatesOnChange`; `TestMarkCollectionScopeRequiresWholeAuthorizedFirstPage` | **EM ANDAMENTO:** ordenação `collection` só na primeira página completa autorizada; página maior segue `page`. Falta matriz C01 inteira com multi-kind, extremos em chunks posteriores e mutações. |

Validação transversal anterior: `go test ./...` passou; `go test -race` dos pacotes de recursos/integração passou; 113 testes Vitest, build TypeScript/Vite, `make format-check` (9 warnings preexistentes) e 15 testes Playwright passaram. Matriz sintética em `/tmp/kubepeep-phase02-matrix-20260921.json`: `global-200` OK, `restricted-200-rejected` erro esperado, `internal-200-origins` OK, delta final de goroutines p95=0; cenário global cursor p95=168 bytes e interno 200 origens p95=25.355 bytes. Relatório tem working tree dirty e não mede cache/watch real.

Validação Kind real após o Docker subir (2026-09-21): foi criado **somente** `kubepeep-f4`; `harness.sh validate` e `app-e2e` passaram com RBAC, revogação e offline reais. O dataset explícito `200x10` produziu 201 namespaces próprios e 2.000 Pods próprios; no escopo `all`, a API resolveu 209 namespaces e percorreu 2.023 Pods únicos em 21 páginas (`limit=100`), sem duplicatas (mediana 6,18 ms/página, máximo 76,95 ms nesta máquina). Duas assinaturas simultâneas de Pods receberam seis chunks/2.023 itens: primeira sincronização ~707 ms, segunda ~4 ms; `kubepeep_watch_active{resource="pods"}=1`, snapshot cache ~383 KiB e RSS do processo ~58,3 MiB. Esses números **não** demonstram troca de telas <100 ms nem orçamento sob rajada de 10k eventos.

O ensaio encontrou LIST repetido de página grande mesmo com WATCH global conectado (identidade de origem efetiva `""` contra catálogo resolvido). `watchCoverageSelection` corrige só a equivalência do cursor global, preserva a cerca de namespaces para fan-out, e `TestResourceBackendGlobalWatchCoversPaginatedPage` passou junto do teste de invalidação existente. A correção **ainda não foi revalidada no binário real**; os números de cache/LIST acima são anteriores a ela. A instância de teste foi encerrada; `kubectl delete -f test/kind/.state/dataset-200x10.yaml` removeu exatamente o dataset (0 namespaces/0 Pods próprios restantes). Os clusters `kubepeep-f4` e `tks-lab` permanecem preservados.

## Fase 3 — Frontend progressivo, virtualizado e inicialização instantânea

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F3-01 | `WORKTREE@a2664f4` | `ResourcePages.tsx`, `ResourcePages.test.tsx`, `App.tsx`; 118 testes Vitest, 16 Playwright, build e format-check | **EM ANDAMENTO:** Pods usa `useInfiniteQuery` com cursor opaco, `fetchNextPage`/`hasNextPage`, `maxPages=5` e linhas acumuladas. Testes cobrem troca de namespace, limite de páginas, 410 que substitui snapshot antigo e 403 que oculta dados anteriores. Busca global lê páginas acumuladas. Demais coleções seguem pendentes. |
| F3-02 | `WORKTREE@a2664f4` | `@tanstack/react-virtual@3.14.13`, `DataTable.test.tsx` (50k linhas), `phase03.spec.ts` (12k Pods e scroll real) | **EM ANDAMENTO:** `DataTable` virtualiza listas acima de 100 linhas, com overscan 8, cabeçalho sticky e ARIA de índice/contagem. DOM ficou abaixo de 60 linhas nos ensaios. Falta medir FPS/CPU e validar alturas variáveis e todas as famílias. |
| F3-04 | `WORKTREE@a2664f4` | `ResourcePages.test.tsx`; teste de filtro com resposta pendente; `phase02.spec.ts` atualizado para cache fresco | **EM ANDAMENTO:** placeholder de Pods conserva linhas apenas na mesma identidade de seleção e exibe “Refreshing Pods…”; 403/generation_changed escondem linhas. Ainda não cobre todos os recursos nem a matriz completa de trocas de contexto/scope/revogação. |
| F3-08 | `WORKTREE@a2664f4` | `ResourcePages.tsx`; testes de retorno à tela sem novo HTTP antes do refresh manual | **EM ANDAMENTO:** Pods usa `staleTime=30s` e `gcTime=120s`; o cache fresco evita revalidação imediata ao retornar à tela. Batching 50–100 ms de watch/log e as demais queries seguem pendentes. |
| F3-01 | `WORKTREE@16da8fa` | `useInfiniteCollection.ts`, `ResourcePages.tsx`; `phase03.spec.ts` com cinco famílias além de Pods; `rtk npm run test:e2e -- e2e/phase03.spec.ts` | **EM ANDAMENTO:** todas as coleções de `ResourcePages.tsx` usam cursor opaco e `maxPages=5`; Workloads, Events, Services, Secrets e Nodes confirmaram duas páginas e cursor isolado no browser. Falta medição de memória/scroll no cluster real e matriz completa de seleção/revogação. |
| F3-05 | `WORKTREE@16da8fa` | `DataTable.tsx`; teste browser com primeira página de 100 linhas falhou antes da correção e passou depois; `rtk npm run test:e2e -- e2e/phase03.spec.ts` | **EM ANDAMENTO:** página normal de 100 itens agora tem viewport com scroll e dispara a próxima página após 75%, enquanto `isFetching` impede concorrência na mesma coleção. Falta prioridade baixa no scheduler global e validação sob carga real. |
| F3-04/F3-08 | `WORKTREE@16da8fa` | inspeção de `useInfiniteCollection.ts`, `ResourceLiveUpdates.tsx` e `LogsPage.tsx`; `rtk make verify` | **EM ANDAMENTO:** placeholder protegido por identidade e 403/generation_changed, `staleTime=30s`/`gcTime=120s` nas coleções migradas, status de watch e logs agrupados em 75 ms. Faltam matriz completa de transições/revogação, alinhamento das demais queries à freshness por tipo e medição real de CPU/renderização. |
| F3-05/F2-09 | `WORKTREE@16da8fa` | `TestVisibleListDoesNotJoinSpeculativeCollectionWork`; testes HTTP de prioridade/cursor; `client.test.ts`; cinco cenários em `phase03.spec.ts` | **EM ANDAMENTO:** prefetch de página seguinte sinaliza `likely-next` no HTTP, usa o scheduler global, preserva capacidade para listas visíveis e devolve `429/PREFETCH_DEFERRED` quando adiado. Cursor/cache mantêm a mesma identidade; a ação manual mantém prioridade visível. Testes Go, TypeScript e Playwright direcionados passaram; falta carga real e integração de outras fontes especulativas. |

Validação desta rodada: `npm run typecheck`, 118 testes Vitest, build e 16 testes Playwright passaram; `format:check` passou com os 9 avisos preexistentes. O `npm install` relatou duas vulnerabilidades moderadas no conjunto de dependências; não foi feito `audit fix --force`. Os números de DOM são sintéticos e não comprovam 60 FPS, primeira linha <500 ms nem startup da janela Wails.

Validação posterior em `WORKTREE@16da8fa`: 124 testes Vitest, TypeScript, os quatro cenários de `phase03.spec.ts` e `rtk make verify` passaram; o gate completo terminou com nove avisos ESLint preexistentes. O ensaio de 12k linhas sintéticas manteve o DOM abaixo de 60 linhas, mas o FPS variou de 31,6 sob a suíte paralela a 60,0 em execução isolada; a meta de 60 FPS segue pendente de medição controlada. As asserções absolutas de FPS e primeira linha foram retiradas desse ensaio porque ele injeta 12k objetos em uma única resposta, fora do contrato HTTP de 100 itens por página, e a concorrência dos workers altera fortemente a medição. O teste mantém a verificação do DOM limitado e da chegada ao fim da lista; as amostras continuam registradas. Dois `make verify` anteriores ao ajuste falharam apenas nesse ensaio sintético (primeira linha 639,6 ms; depois scroll 31,6 FPS); o gate completo passou após separar medição diagnóstica do aceite funcional.

Validação da integração F3-05/F2-09 em 2026-09-28: `rtk make verify` passou com os mesmos nove avisos ESLint preexistentes; `rtk go test -race ./internal/services/resources ./internal/integration/kubernetesruntime ./internal/api/handlers` passou (409 testes em três pacotes); `rtk npm run test:e2e -- phase03.spec.ts` passou (cinco cenários); `rtk git diff --check` passou. O ensaio browser cobre o header de prioridade nas seis famílias, o adiamento e a ação manual. O teste de backend comprova que uma LIST visível conclui enquanto a LIST especulativa equivalente continua bloqueada. Nenhuma medição com concorrência real no Kind foi feita nesta rodada.

### Fechamento F3 — 2026-09-28

**Status: FINALIZADA no commit `33ccb66` da branch `review/plan-v0.7`.** Os ensaios abaixo usam o binário compilado do mesmo código antes do commit. O cenário de latência abre a Overview, aguarda 1 s para o preview autorizado e clica em Pods. O cluster é o Kind dedicado `kubepeep-f4`, escopo explícito com 50 namespaces e 10 Pods por namespace, identidade com RBAC de LIST/WATCH global. O tempo de primeira linha vem de `MutationObserver` no DOM; `time_to_first_visible_row` mede o clique de navegação até o commit React. `time_to_first_row` continua medindo a tentativa HTTP, por isso não representa o preview.

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F3-01 | `33ccb66` | `phase03-real.spec.ts`; cinco páginas Kind/500 Pods; `ResourcePages.test.tsx` | **IMPLEMENTADO:** seis famílias usam `useInfiniteQuery`, cursor opaco e `maxPages=5`; fim do cursor confirmado. Em amostras reais, DOM ficou em 12–15 linhas e heap JS cresceu 1,6–5,2 MiB. |
| F3-02 | `33ccb66` | `DataTable.test.tsx` (50k); `phase03.spec.ts` (12k Pods, Events variáveis) | **IMPLEMENTADO:** scroll isolado de 12k Pods a 60,00 FPS, p95 de frame 16,7 ms, 16 linhas no DOM; Events de altura variável e cabeçalho sticky passaram. |
| F3-03 | `33ccb66` | `phase03_browser.py` + `phase03-real.spec.ts`; cinco processos frios | **IMPLEMENTADO NO CENÁRIO DE ACEITE:** primeira linha 74,7–97,3 ms, p95 conservador 97,3 ms; primeira página completa 966–1.012 ms, p95 1.012 ms. Preview vem de LIST autorizada de um namespace, é parcial/não selecionável, e a LIST do escopo inteiro o substitui; SSE mostrou contadores até ✓ 50/50 e 500 itens. Um namespace inicial vazio depende do stream ou da LIST completa. |
| F3-04 | `33ccb66` | `useInfiniteCollection.test.tsx`, `ResourcePages.test.tsx`, `resources_stream_session_test.go` | **IMPLEMENTADO:** placeholder somente na identidade autorizada; sort/refresh sem flicker; contexto, scope, namespace, geração, resposta tardia e 403/503 removem dados incompatíveis. Progressos SSE são filtrados ao scope antes da entrega. |
| F3-05 | `33ccb66` | `phase03.spec.ts`, `phase03-real.spec.ts`, `TestVisibleListDoesNotJoinSpeculativeCollectionWork` | **IMPLEMENTADO:** próximo cursor disparado a 75% do scroll, uma página à frente, `likely-next` com prioridade baixa; 429 adia e clique manual prossegue. Cinco páginas reais concluíram com 500 Pods e cursor final fechado. |
| F3-06 | `33ccb66` | `rtk make build-desktop`; `phase03_desktop.py` com 1 aquecimento + 5 amostras | **IMPLEMENTADO:** janela Wails p95 234,4 ms; shell p95 652,9 ms em perfil isolado, antes de discovery/RBAC. O teste inclui a migração de preferências de um perfil novo. |
| F3-07 | `33ccb66` | `PanelErrorBoundary.test.tsx`, `Dashboard.test.tsx`, `App.test.tsx` | **IMPLEMENTADO:** falha de Metrics API e de painéis permanece isolada, com retry individual; shell e outras seções continuam úteis. |
| F3-08 | `33ccb66` | `ResourceLiveUpdates.test.tsx` (10k deltas), `LogsPage.tsx`, queries de recursos/métricas/capabilities/preferências | **IMPLEMENTADO:** batches de 75 ms, uma revalidação HTTP e menos de seis commits React na rajada; coleções 30 s de stale/120 s de GC, métricas 8 s, capabilities 45 s e preferências 60 s de stale. |
| F3-09 | `33ccb66` | build Vite, `App.tsx`, `CommandCenter.tsx`, `ResourceWorkspace.tsx` | **IMPLEMENTADO:** Dashboard, workspace, YAML e ações lazy; busca local com debounce de 100 ms. Chunk inicial 339.941 bytes (104.868 gzip), antes 496.260/151.130 bytes. |

A execução final do binário após a telemetria teve `time_to_first_visible_row=75,1 ms`, primeira linha no DOM em 98,9 ms, página completa em 991,8 ms, cinco páginas/500 Pods e crescimento do heap JS de 5,14 MiB. Todas as operações de preferências observadas no percurso responderam HTTP 200.

Limites da coleta: a meta de primeira linha foi medida com o preview preparado após 1 s na Overview e primeiro namespace populado; um namespace inicial vazio depende do stream ou da LIST completa. Os 60 FPS vieram de uma execução isolada, pois a suíte concorrente variou até 31,6 FPS. A rajada de 10k deltas confirmou batching/revalidação e poucos commits React, mas não incluiu perfil direto de CPU.

Gates finais: `rtk make verify`, `rtk make test-race`, `rtk git diff --check`, o gate de segurança e `npm audit --omit=dev` passaram; o ensaio isolado de 12k linhas passou a 60,00 FPS; a execução Kind completa passou depois da migração. O PUT de preferências que devolvia 503 foi corrigido com `0002_expand_preferences.sql`, que preserva os registros antigos sob backup verificado; o teste de migração e o browser confirmaram gravação HTTP 200. A exclusão foi enviada pelo manifesto exato `test/kind/.state/dataset-50x10.yaml`; a espera do `kubectl delete --wait=true` foi interrompida após verificar zero namespaces e zero Pods com o rótulo do dataset, além da ausência do ClusterRole próprio. O nó `kubepeep-f4-control-plane` permaneceu `Ready`.

## Fase 4 — Fechamento do refinamento UI/UX e scope default

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Fase 5 — Investigação: problems, diagnóstico e logs agregados

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Fase 6 — Protocolo e tuning avançado (condicional a benchmark)

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Fase 7 — Validação comparativa e preparação da release

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
|  |  |  |  |

## Pendências futuras (não reabrem a Fase 0)

- F1-11, F1-12 e F1-13 preservam integralmente as validações transferidas.
- Métricas e spans do novo resource cache continuam atribuídos à F2 por C04;
  não são simulados para fechar D01/D04 antecipadamente.
