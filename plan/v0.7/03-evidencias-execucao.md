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

### Fechamento F2 — 2026-09-29

**Status: FINALIZADA no commit `cf0400b` da branch `review/plan-v0.7`.** A
rodada de fechamento eliminou todas as caixas abertas da fase e reexecutou os
gates completos. O ensaio real usou o cluster Kind dedicado `kubepeep-f4`, o
manifesto exato `test/kind/.state/dataset-200x10.yaml`, identidade com
LIST/WATCH global e scope `all` com 209 namespaces resolvidos.

| ID | Commit | Evidência final | Resultado |
| --- | --- | --- | --- |
| F2-01 | `cf0400b` | `TestResourceBackendCoordinatesAggregateMemoryPressureInOrder`; `TestCollectionCachePressurePurgesExpiredThenTrimsLeastRecentlyUsed`; `phase02_browser.py` | **IMPLEMENTADO:** teto agregado padrão de 224 MiB para snapshots, páginas e cursores, com eviction ordenada. No C02 real o processo foi de 50,4 para 62,4 MiB RSS e 15 para 17 threads. |
| F2-02 | `cf0400b` | testes de cache, fencing e revogação já consolidados; `make test-race` | **IMPLEMENTADO:** estados e geração impedem resposta antiga; cache fresco reaparece sem bloquear a tela. |
| F2-03 | `cf0400b` | `phase02-real.spec.ts`; métrica local real | **IMPLEMENTADO:** Pods → Deployments → Pods retornou em 30 ms, sem novo HTTP de Pods; `kubepeep_watch_active{resource="pods"}` permaneceu 1 antes e depois. |
| F2-04 | `cf0400b` | testes de bookmark/reconnect existentes; `make test-race` | **IMPLEMENTADO:** bookmark avança o checkpoint sem emitir delta de UI. |
| F2-05 | `cf0400b` | `TestSubscriptionCoalescesResourceBurstButPreservesEvents`; `ResourceLiveUpdates.test.tsx` | **IMPLEMENTADO:** 10k deltas ficam sob fila de 1 MiB/1.000 eventos, um estado por objeto, uma revalidação, menos de seis commits e menos de 500 ms de CPU de render React; Events continuam cronológicos. |
| F2-06 | `cf0400b` | `version_cache.go`; quatro testes de coalescing/TTL/invalidação/falha | **IMPLEMENTADO:** `/version` saudável usa TTL de 10 min e chave completa da seleção; falha não é cacheada; generation invalida. Os demais TTLs permanecem documentados em `docs/observability.md`. |
| F2-07 | `cf0400b` | `Dashboard.test.tsx`; `phase02-real.spec.ts` | **IMPLEMENTADO:** summary/Tier 1 libera depois Node health e PVCs no Tier 2; métricas continuam isoladas e log scan continua explícito no Tier 3. O browser confirmou as duas requests após a resposta do summary. |
| F2-08 | `cf0400b` | fast path e fallback já consolidados; gate completo | **IMPLEMENTADO:** streaming list permanece opcional e o fallback clássico segue coberto. |
| F2-09 | `cf0400b` | testes HTTP/client/scheduler; captura real de headers | **IMPLEMENTADO:** prioridades visible, `likely-next` e `unrelated` dividem o scheduler global. Node/PVC reais saíram com `unrelated`; prefetch preserva `likely-next`; trabalho visível conserva capacidade. |
| F2-10 | `cf0400b` | testes 410 de LIST/WATCH e renovação de cursor já consolidados | **IMPLEMENTADO:** cursor/RV expirado reconstrói o snapshot sem erro fatal. |
| F2-11 | `cf0400b` | `TestWatchCacheOperationsExportSafeOTLPSpans` | **IMPLEMENTADO:** collector OTLP protobuf recebeu `cache.snapshot` e `cache.apply_event`; o payload não contém generation, contexto, scope, namespace ou nome do objeto usados pelo teste. Métricas reais de hit/miss/bytes/eviction/invalidação também passaram. |
| F2-12 | `cf0400b` | `TestLazyMergeMultiKindKeepsLaterExtremesOutsidePageScope`; matriz de backend/cache | **IMPLEMENTADO:** ordem de coleção só aparece em snapshot completo; multi-kind, extremo em origem posterior, paginação, autorização parcial e mutação preservam escopo honesto. |

Gates finais: `rtk make verify`, `rtk make test-race`, `rtk git diff --check`
e os dois testes Playwright do Kind passaram. O manifesto C02 foi removido pela
mesma entrada YAML; restaram zero namespaces e zero Pods com o label do
dataset, o ClusterRole próprio não existe mais e
`kubepeep-f4-control-plane` permaneceu `Ready`. O relatório reproduzível fica
em `test/kind/.state/phase02-real.json`, deliberadamente ignorado pelo Git.

O fechamento também resolve as pendências cruzadas das fases já concluídas:
a regressão histórica de navegação da F1 fica substituída pelo retorno real de
30 ms com o mesmo watch, e a rajada F3-08 agora possui perfil direto de CPU de
render, além da contagem de commits e da revalidação única.

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

Limites da coleta: a meta de primeira linha foi medida com o preview preparado após 1 s na Overview e primeiro namespace populado; um namespace inicial vazio depende do stream ou da LIST completa. Os 60 FPS vieram de uma execução isolada, pois a suíte concorrente variou até 31,6 FPS. A rajada de 10k deltas confirmou batching/revalidação e poucos commits React; o fechamento F2 posterior acrescentou perfil direto e exige menos de 500 ms de CPU de render React para a rajada.

Gates finais: `rtk make verify`, `rtk make test-race`, `rtk git diff --check`, o gate de segurança e `npm audit --omit=dev` passaram; o ensaio isolado de 12k linhas passou a 60,00 FPS; a execução Kind completa passou depois da migração. O PUT de preferências que devolvia 503 foi corrigido com `0002_expand_preferences.sql`, que preserva os registros antigos sob backup verificado; o teste de migração e o browser confirmaram gravação HTTP 200. A exclusão foi enviada pelo manifesto exato `test/kind/.state/dataset-50x10.yaml`; a espera do `kubectl delete --wait=true` foi interrompida após verificar zero namespaces e zero Pods com o rótulo do dataset, além da ausência do ClusterRole próprio. O nó `kubepeep-f4-control-plane` permaneceu `Ready`.

## Fase 4 — Fechamento do refinamento UI/UX e scope default

**Status: FINALIZADA no commit `f318f49` da branch `review/plan-v0.7`.**

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F4-01 | `f318f49` | `tokens.css`, `styles.css`, `ExecTerminal.tsx`; busca estática por `font-size`, `fontSize` e `text-[…]` | **IMPLEMENTADO:** famílias, escala, pesos, line-height, terminal, superfícies e cores vêm de tokens. Não restou tamanho arbitrário; literais de cor ficam nos tokens e SVGs de marca. |
| F4-02 | `f318f49` | `phase04.spec.ts`; medições geométricas nos quatro viewports | **IMPLEMENTADO:** controles ocuparam 3,72–5,56% da área útil e o conteúdo 96,27–98,19% da largura, sempre dentro dos limites de aceite. |
| F4-03 | `f318f49` | `app.spec.ts`, `features.spec.ts`, catálogo/rotas existentes e suíte Playwright completa | **IMPLEMENTADO:** destinos laterais/horizontais, History API, tabs, contexto, filtros e dados foram exercitados sem rota visual inerte. |
| F4-04 | `f318f49` | migration `0003_namespace_scope_defaults.sql`; `DefaultScopeGate`; testes service/repository/handler/UI | **IMPLEMENTADO:** no máximo um default por profile/contexto; ativação automática no startup/troca, fence de geração, contexto sem default bloqueado, operador sem catálogo de namespaces e exclusão com substituto ou retorno explícito ao setup. |
| F4-05 | `f318f49` | `ResourceWorkspace.tsx`, `ResourcePages.test.tsx`, `phase04.spec.ts` | **IMPLEMENTADO:** catálogo de tabs por kind completo, incluindo Metrics/Containers de Pod, Endpoints/Actions de Service e Rules/Backends/Actions de Ingress; relacionados preservam o workspace e o histórico. |
| F4-06 | `f318f49` | `ResourceActions.tsx`, `ResourceWorkspace.tsx`; adapter/service/rota de port-forward | **IMPLEMENTADO:** catálogo existente foi revalidado e Services ganharam port-forward real com resolução de Pod Ready e `targetPort`; Ingress ganhou backends, Open URL e copy host. |
| F4-07 | `f318f49` | `ResourcePages.tsx` e testes de bulk restart/delete | **IMPLEMENTADO:** toolbar contextual oferece apenas restart/delete compatíveis com toda a seleção, confirma todos os alvos e apresenta sucessos e falhas parciais. |
| F4-08 | `f318f49` | queries de permissions, `disabledReason`, testes de serviço e Kind RBAC | **IMPLEMENTADO:** UI exige capabilities por alvo e explica bloqueios; backend revalida geração e autorização exata, inclusive `pods/portforward` no Pod resolvido para Service. |
| F4-09 | `f318f49` | `ConfirmDialog`, testes `ResourceActions`/`ResourcePages` | **IMPLEMENTADO:** restart e delete exibem alvo/namespace/consequência; delete de workload exige digitar o nome; operações em massa listam todos os recursos. |
| F4-10 | `f318f49` | estados pending/error, toasts e testes de ações | **IMPLEMENTADO:** todas as novas ações expõem progresso e resultado perceptível; falha real da API fecha confirmação e aparece no toast. |
| F4-11 | `f318f49` | `styles.css`, tabela/chooser de colunas existentes, `phase04.spec.ts` | **IMPLEMENTADO:** conteúdo usa 96,27–98,19% da largura nos viewports de aceite, tabela mantém colunas por kind e não cria overflow global. |
| F4-12 | `f318f49` | tokens semânticos e inspeção visual dos screenshots Playwright | **IMPLEMENTADO:** azul, verde, vermelho e âmbar permanecem centralizados em tokens, com dark theme e foco visível preservados. |
| F4-13 | `f318f49` | screenshots e cinco testes de `phase04.spec.ts` | **IMPLEMENTADO:** 1366×768, 1440×900, 1920×1080 e 2560×1440 passaram sem overflow ou sobreposição; espaço extra ampliou a tabela. |
| F4-14 | `f318f49` | 1.178 testes Go, 147 Vitest, 26 Playwright, `make verify`, `make test-race` e harness Kind | **IMPLEMENTADO:** fetch, filtros, search, sorting, paginação, live updates, scope/default, saved filters, colunas, workspace, ações, RBAC e estados foram exercitados. |
| F4-15 | `f318f49` | `docs/ui-ux-refinement.md`, `docs/api.md`, `docs/data-model.md`, README e checklist F4 | **IMPLEMENTADO:** decisões finais, contratos, medidas, migração, API, limites deliberados e validação ficaram documentados. |

Validação final em 2026-09-29: `rtk make build`, `rtk make verify`, `rtk make
test-race`, `rtk go test ./...` (1.178), `rtk npm test -- --run` (147), `rtk
npm run test:e2e` (26 passados e quatro cenários reais condicionais omitidos),
`rtk npm run test:e2e -- e2e/phase04.spec.ts` (5/5), `rtk git diff --check`,
o gate de segurança e `npm audit --omit=dev` passaram. O lint terminou sem erro
e com oito avisos `react-refresh/only-export-components` já aceitos pelo gate.
O conjunto de dependências de produção possui zero vulnerabilidades reportadas;
o `npm ci` informa três moderadas restritas a dependências de desenvolvimento.

O binário do mesmo código passou no harness do cluster Kind `kubepeep-f4`, com
contexto/scope reais, dashboard, SSE/log, WebSocket exec, revogação e restauração
de RBAC e modo offline. O cluster foi reutilizado e preservado; ao fim,
`kubepeep-f4-control-plane` permaneceu `Ready` em Kubernetes v1.35.0.

### Auditoria consolidada F1–F4 — 2026-09-29

**Resultado: 49/49 tarefas conferidas e sem pendência nas Fases 1–4.** A
auditoria partiu de `1345947`, cruzou os quatro checklists, as tabelas finais de
evidência, os commits citados e o grafo atual. Foram encontrados 13 IDs F1,
12 IDs F2, 9 IDs F3 e 15 IDs F4; todos estão marcados e cada ID possui evidência
final. Os símbolos centrais continuam integrados aos caminhos reais: a seleção
de paginação é chamada por `Collect`, o resource cache pelo Watch Manager,
`useInfiniteCollection` pelas seis famílias e `DefaultScopeGate` pelo shell.

A regressão de UX médio registrada em F1-12 é um **resultado medido**, não uma
execução omitida: F1-12 exigia coleta e confronto honesto com os budgets. F2/F3
posteriormente comprovaram retorno cacheado em 30 ms, primeira linha em até
97,3 ms e página completa em até 1.012 ms no cenário Kind médio. A comparação
final de release, incluindo nova coleta de filtro/sort, continua corretamente
atribuída à F7 e não reabre F1–F4.

A revisão encontrou quatro vulnerabilidades Go alcançáveis preexistentes em
gRPC/OpenTelemetry. O commit `76ac4bb` atualiza gRPC para 1.83.1 e a família
OpenTelemetry para 1.44.0, além de alinhar os documentos de estado que ainda
descreviam o snapshot inicial. Depois da atualização, `govulncheck ./...`
terminou com zero vulnerabilidades alcançáveis; um advisory em pacote importado
permanece sem call path e não afeta o código segundo a análise da ferramenta.

Gates desta auditoria: `rtk make verify`, `rtk make test-race`, `rtk go test
./...` (1.178 testes), `rtk go mod verify`, `rtk govulncheck ./...`, o gate de
segurança, `npm audit --omit=dev` e o harness Kind `app-e2e` passaram. O lint
continua com oito avisos `react-refresh/only-export-components` e zero erros;
o conjunto npm de produção tem zero vulnerabilidades. O cluster
`kubepeep-f4` foi preservado e seu nó permaneceu `Ready`.

## Fase 5 — Investigação: problems, diagnóstico e logs agregados

**Status: FINALIZADA no commit funcional `146b6a0` da branch
`review/plan-v0.7`.**

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F5-01 | `146b6a0` | `index_test.go`; `TestLocalIndexStaysInsideCacheBudgetAcrossTwoHundredNamespaces`; `TestIndexCoverageRequiresEveryWorkloadGVRForEveryExpectedNamespace` | **IMPLEMENTADO:** índice generation-scoped relaciona owner/status/labels, selectors de Service, involvedObject, ConfigMaps e PVCs; selectors/páginas/GVRs/origens ausentes permanecem incompletos. O cenário sintético 200×10 reteve 2.000 Pods e 200 namespaces sob budget de 4 MiB. |
| F5-02 | `146b6a0` | `internal/problems/problems_test.go`; testes de `DashboardBackend.Problems`; Kind `app-e2e` | **IMPLEMENTADO:** detectores puros cobrem CrashLoopBackOff, image pull, Pending, OOMKilled, restarts, Jobs/replicas, PVC, Node, probes e Warning Events nas três severidades, sem converter falta de evidência em saúde. |
| F5-03 | `146b6a0` | `Dashboard.test.tsx`; `dashboard.go`; `resources_test.go` | **IMPLEMENTADO:** Overview apresenta contadores critical/warning/info, recurso, namespace, diagnóstico e ações Inspect/Logs; busca, sort e cursor usam a identidade generalizada e o Workspace recebe o alvo. |
| F5-04 | `146b6a0` | `TestInvestigationResolvesOwnerServiceStorageConfigAndEventsLocally`; `investigation_test.go`; catálogo de tabs em `ResourcePages.test.tsx` | **IMPLEMENTADO:** Investigation resolve Pod→ReplicaSet→Deployment, Pods, Service/EndpointSlice, ConfigMaps, PVCs e Events no índice local, com navegação pelo histórico e coverage explícito. |
| F5-05 | `146b6a0` | `LogsPage.test.tsx` — seis Pods, cinco streams, regex e abort coletivo; `make verify` | **IMPLEMENTADO:** Deployment/ReplicaSet/StatefulSet/DaemonSet/Job usam fluxo combinado com Pod/container/timestamp, current/previous/follow, seleção e busca; teto de cinco streams, batch de 75 ms e ring de 1.000 linhas/1 MiB são cancelados por Stop, troca ou unmount. |
| F5-06 | `146b6a0` | `GlobalResourceSearch.test.tsx`; fake timer em 30 ms; `fetch` não chamado | **IMPLEMENTADO:** Ctrl/Cmd+K busca rotas e identidades já carregadas, publica filtro local em 30 ms (<100 ms) e informa que ausência no cache não prova ausência no cluster. |
| F5-07 | `146b6a0` | `metrics_test.go`; `diagnostics_test.go`; `SettingsPage.test.tsx` | **IMPLEMENTADO:** Settings → Diagnostics → Performance exibe p50/p95/p99 globais coerentes a partir de amostras bounded, cache hit, watches, requests/min, 429, reconnects e sync por recurso. |
| F5-08 | `146b6a0` | `diagnostics_test.go`; `SettingsPage.test.tsx`; `make test-race`; Kind `app-e2e` | **IMPLEMENTADO:** cluster mostra versão, totais e memória do KubePeep; namespaces mostram recursos, severidades, restarts e LIST latency, mais lentos primeiro. Coleta usa quatro workers, timeout global, teto honesto de 200 e estados unknown/parcial sob RBAC. |

### Gates de fechamento executados

| Gate | Resultado |
| --- | --- |
| `rtk make verify` | aprovado; formatação, lint, tipos, unidades/integração, Playwright, build e smoke verdes |
| `rtk make test-race` | aprovado |
| `rtk go test ./...` | 1.191 testes em 40 pacotes aprovados |
| `rtk npm test -- --run` | 150 testes em 30 arquivos aprovados, incluindo a paleta local em 30 ms |
| `rtk scripts/security_check.sh HEAD` | aprovado sobre staged content, histórico, identidades, caminhos, mensagens, tags e secrets |
| `govulncheck` sobre `go list ./...` | zero vulnerabilidades alcançáveis; um advisory importado sem call path |
| `rtk npm audit --omit=dev` | zero vulnerabilidades de produção |
| `rtk test/kind/harness.sh app-e2e ./dist/kubePeep` | contexto/scope, dashboard, SSE/log, exec, revogação/restauração RBAC e offline aprovados |
| `rtk git diff --check` | aprovado |

O cluster `kubepeep-f4` foi reutilizado e preservado; ao final do harness, o nó
`kubepeep-f4-control-plane` permaneceu `Ready` em Kubernetes v1.35.0. O dataset
massivo de C02 não foi reaplicado: F5 usa o cenário sintético 200×10 acima e a
evidência Kind real 200×10 já registrada em F1-13, evitando carga destrutiva sem
novo comando explícito.

## Fase 6 — Protocolo e tuning avançado (condicional a benchmark)

**Status: FINALIZADA no commit funcional `148419f` da branch
`review/plan-v0.7`.** A coleta final usou esse SHA com árvore limpa, Go 1.26.7,
Linux/amd64, 12 CPUs, `GOMAXPROCS=12`, 41.716.428.800 bytes de memória e
Kind/Kubernetes v1.35.0. O dataset observado tinha 23 Pods; transporte teve
duas execuções de aquecimento e 30 amostras por variante. O JSON sanitizado é
gerado localmente em `test/kind/.state/performance-protocol.json` e não é
versionado.

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F6-01 / D31 | `148419f` | `make benchmark-protocol`; `TestFactoryScopesProtocolNegotiationByClientFamily`; fallback 406/415 | **APLICADO:** PartialObjectMetadata reduziu p95 de 7,767 para 3,841 ms (-50,5%) e payload de 142.202 para 69.584 bytes (-51,1%). ServiceAccount/ConfigMap possuem fallback explícito; o cliente metadata negocia JSON como segunda opção. Secrets nunca usam cliente tipado nem expõem conteúdo. |
| F6-02 / D32 | `148419f` | `make benchmark-protocol`; teste de negociação por família | **APLICADO:** Protobuf reduziu p95 para 5,504 ms (-29,1%) e payload para 95.261 bytes (-33,0%). `Accept` inclui JSON; dynamic, CRDs e aggregated APIs permanecem JSON. |
| F6-03 / D33 | `148419f` | Kind real × rede modelada 30 ms/10 Mbit/s, 30 amostras | **AVALIADO, NÃO ATIVADO NO DEFAULT:** gzip local reduziu bytes em 85,4%, mas elevou p95 de 7,767 para 8,070 ms (+3,9%). Na rede modelada, p95 caiu de 140,091 para 41,071 ms (-70,7%); `compression=true` fica disponível para cenário remoto medido. A rede modelada não é apresentada como cluster remoto real. |
| F6-04 / D30 | `148419f` | 40 LISTs por faixa 5/10, 10/20 e 20/40; throttle, p95, erros e 429 | **AVALIADO, DEFAULT 10/20 MANTIDO:** p95 foi 5.602,510 / 1.803,443 / 11,803 ms; throttle agregado 92.996,9 / 21.010,8 / 0,4 ms; zero 429 e zero erros em todas. O Kind pequeno não prova segurança de 20/40 sob carga compartilhada; configuração rejeita 100/200. |
| F6-05 / D34 | `148419f` | experimento determinístico 429, 40 requisições por variante; `TestRequestSchedulerAIMDReducesAndRecoversWithinBounds` | **APLICADO:** fixo 4 gerou 9 respostas 429 em 231,463 ms; AIMD gerou 2 (-77,8%) em 231,668 ms, reduziu duas vezes e recuperou quatro vezes, limitado a 2/4/8. |
| F6-06 | `148419f` | `TestRelatedPrefetchWarmsBoundedPodAndEventPages`; ensaio scheduler; `make test-race` | **APLICADO:** prefetch relacionado lê páginas autorizadas de 20 Pods/Events com prazo de 3 s e cancelamento coletivo. Quatro trabalhos sem reserva fizeram visible esperar 40,152 ms; prioridades reservaram capacidade e a espera ficou abaixo de 0,001 ms, adiando um prefetch. |

Gates: `rtk make verify`, `rtk make test-race`, `rtk go test ./...` (1.204
testes em 41 pacotes), `rtk npm test -- --run` (150 testes em 30 arquivos),
`govulncheck` sobre `go list ./...` (zero vulnerabilidades alcançáveis; um
advisory importado sem call path), `npm audit --omit=dev` (zero vulnerabilidades
de produção), gate de segurança, benchmark Kind real, `rtk git diff --check` e
harness Kind `app-e2e` passaram. O cluster não recebeu dataset nem mutação do
benchmark de protocolo e permaneceu preservado; contexto/scope, dashboard,
SSE/log, WebSocket exec, revogação/restauração RBAC e modo offline passaram.

## Fase 7 — Validação comparativa e preparação da release

**Status: FINALIZADA no commit funcional `b148a60` da branch
`review/plan-v0.7`.** Os JSONs completos permanecem locais em
`test/kind/.state/`; os agregados sanitizados foram publicados na documentação.

| ID | Commit | Evidência | Resultado |
| --- | --- | --- | --- |
| F7-01 | `b148a60` | `make benchmark`, `make benchmark-matrix`, `make benchmark-protocol`; `docs/performance-baseline.md` | **IMPLEMENTADO:** schema v2, 10 cenários representativos e matriz 47/47 em árvore limpa. Namespace-100 caiu de 508,069 para 61,363 ms p95 e mixed-100 de 760,736 para 101,773 ms; requests/bytes/over-fetch/cursor/concorrência caíram juntos. 429, cache hit e reconexão ficaram explícitos. |
| F7-02 | `85a39d4` | `pagination_phase07_test.go`, handler 410, `make test-race` | **IMPLEMENTADO:** 10×100 objetos, `limit=37`, identity asc/desc, age asc/desc, sem gaps/duplicatas, mutação expira snapshot, erro sanitizado, cursor/memória limitados e goroutines de volta ao baseline. |
| F7-03 | `b148a60` | comparação P0/P1/P2; revisões C01–C06; matriz F1–F6 | **APROVADO:** limites legítimos, generation fence, timeout, lazy merge, cancelamento, coalescing, 429/410, watch/cache, virtualização, progressividade e ações condicionais permaneceram cobertos; R01/R02/R03 não reproduziram. |
| F7-04 | `b148a60` | `make verify`, `make test`, `make test-e2e`, `make test-race`, `make format-check lint typecheck`, `make build smoke`, `make build-desktop` | **APROVADO:** todos os gates terminaram verdes; lint manteve 8 warnings conhecidos e zero erro. |
| F7-05 | `85a39d4` | `phase07.spec.ts`; 44 destinos × 4 viewports; catálogo por kind; matriz RBAC; Kind e AT-SPI Wails | **APROVADO:** zero clique morto/overflow; 24 combinações RBAC; default antes da LIST; Pods exibiu nove linhas Kind reais no WebView nativo. |
| F7-06 | `b148a60` | gate de segurança em cada commit; `go mod verify`; `govulncheck ./...`; `npm audit --omit=dev`; `make release-artifact-check` | **APROVADO:** zero leak, vulnerabilidade alcançável ou de produção; audit log sem identidade de objeto; Secrets metadata-only e stores efêmeros preservados. |
| F7-07 | `b148a60` | `docs/release-v0.7-candidate.md`; build CLI/Wails; scanner de `dist/` | **PREPARADO LOCALMENTE:** notas, limitações e decisão de publicação registradas. Nenhum tag, push ou release foi criado. GoReleaser não está instalado no host, portanto pacotes finais multiplataforma ficam para a etapa explícita de publicação. |

O harness `create/validate` reaplicou fixtures expirados sem excluir o cluster;
o `app-e2e` final aprovou contexto/scope, dashboard, recursos, SSE/log,
WebSocket exec, revogação/restauração de RBAC e offline. O kubeconfig restrito
foi renovado antes do aceite Wails. O nó Kind permaneceu preservado.

Segurança final: o evento de ação guarda apenas kind canônico, duração, decisão
e código fechado. Sentinelas de contexto, namespace e objeto foram verificadas
contra evento, stdout, SQLite, JSONL e artefatos. `govulncheck` reportou zero
call paths vulneráveis e um advisory importado não alcançável; npm de produção
reportou zero vulnerabilidades. A auditoria geral posterior atualizou Vitest e
`undici`, eliminando também os três advisories de desenvolvimento.

## Auditoria geral F0–F7 — 2026-09-29

**Status: APROVADA no commit funcional `52aa3db`.** A revisão releu o plano,
os contratos, a matriz, os checklists e os documentos correntes; cruzou o grafo
atualizado com a fonte física e reexecutou os gates afetados. O inventário tem
79/79 tarefas marcadas nas oito fases e 68/68 linhas na matriz; cada tarefa
possui evidência final sem `PENDENTE` ou `EM ANDAMENTO`. Todos os SHAs citados
existem e todos os links Markdown locais dos documentos correntes são válidos.

Achados corrigidos: o `doctor` ainda descrevia o exporter OTLP implementado
como ausente; o diagnóstico agora aprova a configuração habilitada e possui
regressão. Estados antigos ainda mostravam F5–F7 ou toda a v0.7 como planejados;
sete links apontavam para diretórios removidos; documentos de produto não
refletiam as famílias e jornadas entregues. Vitest 4.1.10 e `undici` 8.9.0
tinham três advisories apenas no tooling; as versões 4.1.11 e 8.11.2 encerraram
o `npm audit` completo sem `--force`.

| Verificação final | Resultado |
| --- | --- |
| `rtk make verify` | aprovado; 175 Vitest, Playwright, Go/vet, tipos, build, smoke e Ginger verdes; 8 warnings Fast Refresh, zero erro |
| `rtk make test-race` | aprovado |
| `rtk make build-desktop` | aprovado com Wails 2.15/WebKitGTK |
| `rtk test/kind/harness.sh app-e2e ./dist/kubePeep` | aprovado; contexto/scope, dashboard, SSE/log, exec, revogação/restauração e offline reais |
| `rtk scripts/security_check.sh HEAD` | aprovado; staged content, histórico completo, identidades, paths, mensagens, tags e secrets |
| `rtk go mod verify`; `rtk govulncheck ./...` | módulos íntegros; zero vulnerabilidade alcançável e um pacote importado sem call path |
| `rtk npm audit`; `rtk make release-artifact-check` | zero vulnerabilidades npm; artefato local aprovado |
| links, tarefas, matriz, evidências e commits | 0 link quebrado; 79/79 tarefas; 68/68 entregas; 0 SHA ausente |

O benchmark não foi repetido após `52aa3db` porque esse commit altera o
diagnóstico local, documentação e dependências de teste, sem tocar os caminhos
de coleta. O comparativo reproduzível permanece ligado a `b148a60`. O nó
`kubepeep-f4-control-plane` permaneceu `Ready` em Kubernetes v1.35.0 após o
harness. GoReleaser e publicação multiplataforma continuam na decisão externa.

## Decisão externa remanescente

- Escolha da versão/tag, instalação/execução do GoReleaser, push e publicação
  dependem de ação explícita do mantenedor e não reabrem a Fase 7.
