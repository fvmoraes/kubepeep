# Fase 2 — Estado orientado a snapshot: cache sob demanda + WATCH

**Prioridade:** P0. **Entrada:** F1. **Desbloqueia:** F3 e F5. **Matriz:** D15–D21, D01/D04 (resource cache), D06 (ordem de coleção sobre snapshot completo), T02, T03.

Mudança central da v0.7: de LIST repetitivo para **LIST → snapshot → watch → cache local**, servindo a UI a partir do estado local. O `WatchManager` atual (`internal/services/resources/watch.go`, com backoff+jitter, tratamento de `ResourceExpired` e fences de geração) é revisado e preservado; a evolução adiciona compartilhamento, cache e política de ciclo de vida. **Demand-driven:** KubePeep é dashboard desktop, não controller — nenhum informer permanente de todos os GVRs.

Contratos transversais de execução e aceite: [C01–C06](05-contratos-e-aceite.md).

## Tarefas

- [x] **F2-01 — Resource cache demand-driven.** Nova camada (ex.: `internal/services/resources/cache/`): Manager, Subscription, Store, RefCounter, Eviction. Chave base: (generation, contexto, scope, topic, GVR, namespace, selector), com resolução efetiva de origens idêntica; páginas/consultas incluem filtros, sort, ordem e paginação (C03). O coordenador limita snapshots, páginas autorizadas e cursores a 224 MiB por padrão e aplica a ordem watches inativos → cursores/páginas expirados → snapshots sem referência → páginas LRU. Apenas memória; valores de Secret **nunca** entram no cache. O ensaio Kind C02 com 209 namespaces resolvidos/2.000 Pods encerrou em 62,4 MiB RSS, abaixo do teto.
- [x] **F2-02 — stale-while-revalidate.** Estados FRESH/STALE/REFRESHING/PARTIAL/EXPIRED por entrada; abrir tela cacheada renderiza imediatamente e revalida em paralelo; invalidação explícita por generation/contexto e 403/revogação detectada, impedindo repopulação por resposta antiga (C03).
- [x] **F2-03 — Watches compartilhados.** Um watch por identidade completa de C03, sem misturar selectors, namespaces ou gerações, com ref count + idle timeout de 45 s. O ensaio real Pods → Deployments → Pods retornou em 30 ms, sem novo HTTP de Pods, e manteve exatamente um worker `kubepeep_watch_active{resource="pods"}=1` antes e depois da troca.
- [x] **F2-04 — Bookmarks.** `allowWatchBookmarks=true` quando suportado, sem depender de intervalo fixo; bookmarks servem de checkpoint para reconexão e reduzir risco de RV antigo.
- [x] **F2-05 — Backpressure level-driven.** Fila limitada a 1 MiB/1.000 eventos, com coalescing por resource key para tópicos level-driven. Events, logs e streams cronológicos preservam ordem. A rajada controlada de 10k deltas reteve um estado por objeto, causou uma revalidação e menos de seis commits React; o perfil direto limita o tempo de render da rajada a 500 ms.
- [x] **F2-06 — Freshness por tipo.** Watch para recursos core; CPU/memória em 8 s; capabilities/RBAC em 45 s; discovery e `/version` em 10 min. O cache de versão coalesce concorrência, guarda apenas respostas saudáveis e é invalidado por generation/contexto; falhas permanecem retryable. Política documentada.
- [x] **F2-07 — Overview por tiers.** Tier 1 (Pods, workloads, restarts, failed/pending, warning events) renderiza primeiro; Tier 2 inicia depois do summary e inclui CPU/memória, Node health e PVCs não Bound; Tier 3 mantém log scanning explícito fora do caminho crítico. O ensaio real confirmou Node/PVC após o término do summary.
- [x] **F2-08 — Streaming lists (fast path opcional).** `sendInitialEvents=true` + bookmarks + `resourceVersionMatch=NotOlderThan` atrás de feature flag com capability detection; fallback LIST+WATCH clássico para clusters antigos. Nunca dependência exclusiva.
- [x] **F2-09 — Scheduler adaptativo (base).** Prioridades visible/likely-next/unrelated com budget global (concorrência, QPS, retry/backoff); observação de latência, timeouts, 429 e congestionamento. O prefetch da próxima página usa `likely-next`; Node/PVC do Dashboard usam `unrelated`; listas visíveis conservam capacidade reservada. O ensaio Kind confirmou os headers e a ordem sob o dataset C02. AIMD completo permanece na F6.
- [x] **F2-10 — Recuperação de 410 Gone.** Continue token/RV expirado: invalidar cursor local, reiniciar LIST consistente, reconstruir página/cache; a UI mostra aviso discreto de snapshot renovado — nunca erro fatal. Consistência de paginação por origem (RV/continue preservados por sequência).
- [x] **F2-11 — Instrumentação real do resource cache.** Métricas reais cobrem hit/miss, bytes, eviction e invalidação. O teste de exportação OTLP executa `cache.snapshot` e `cache.apply_event`, valida ambos os spans no collector protobuf e rejeita geração, contexto, scope, namespace e nome de objeto no payload.
- [x] **F2-12 — Ordenação do snapshot.** Ordem de coleção é anunciada somente sobre snapshot completo, autorizado e dentro do orçamento; estados parciais mantêm `filterScope=page`. A matriz cobre famílias distintas, multi-kind, extremos em origens posteriores, paginação, autorização parcial e invalidação por mutação.

## Cenários obrigatórios de aceite

A auditoria aprofundada de 2026-09-29 incluiu explicitamente as cópias dos
workers no teto de F2-01: 96 MiB de resource cache, 32 MiB de snapshots de watch,
64 MiB de páginas e 32 MiB de cursores. F2-03 também cobre cleanup quando o
replay inicial é recusado e limites durante ADDED/MODIFIED, além do LIST inicial.

| Cenário | Resultado exigido |
| --- | --- |
| abrir Pods → Deployments → voltar a Pods | trocas ~instantâneas via cache; watch único compartilhado; ref count correto |
| watch saudável | sem LIST completo repetido; criação/mudança/delete chega como 1 delta |
| watch proibido por RBAC | snapshot paginado + refresh manual/polling controlado; nunca polling de 1–2 s |
| 10k eventos em rajada | fila limitada; coalescing por key; UI estável; memória sob teto |
| 410 Gone / RV expirado | recuperação automática com aviso discreto; sem erro fatal |
| selectors/namespaces distintos, troca de geração e revogação | compartilhamento só com identidade completa; cancela trabalho antigo, invalida dados afetados e impede repopulação por resposta atrasada (C03) |
| cluster antigo sem streaming lists | fallback clássico funciona; flag desligada |
| memória sob carga (cenários de 200 namespaces separados por C02) | cache e cursor dentro do orçamento; eviction observável; goroutines retornam ao baseline |

**Saída:** UI orientada a estado; primeira sincronização 1–3 s em cluster médio; troca de tela cacheada < 100 ms. **Rollback:** cache por trás de flag interna; o caminho de listagem atual continua funcional durante a fase.
