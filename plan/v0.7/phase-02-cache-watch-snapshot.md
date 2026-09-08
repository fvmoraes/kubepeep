# Fase 2 — Estado orientado a snapshot: cache sob demanda + WATCH

**Prioridade:** P0. **Entrada:** F1. **Desbloqueia:** F3 e F5. **Matriz:** D15–D21, T02, T03.

Mudança central da v0.7: de LIST repetitivo para **LIST → snapshot → watch → cache local**, servindo a UI a partir do estado local. O `WatchManager` atual (`internal/services/resources/watch.go`, com backoff+jitter, tratamento de `ResourceExpired` e fences de geração) é revisado e preservado; a evolução adiciona compartilhamento, cache e política de ciclo de vida. **Demand-driven:** KubePeep é dashboard desktop, não controller — nenhum informer permanente de todos os GVRs.

## Tarefas

- [ ] **F2-01 — Resource cache demand-driven.** Nova camada (ex.: `internal/services/resources/cache/`): Manager, Subscription, Store, RefCounter, Eviction. Chave: (generation, contexto, scope, GVR, selector). Orçamento de memória (128–256 MiB inicial) e eviction (watches inativos → cursores expirados → cache de tela não usada → LRU por scope). Apenas em memória; Secret values **nunca** entram no cache (somente metadata autorizada).
- [ ] **F2-02 — stale-while-revalidate.** Estados FRESH/STALE/REFRESHING/PARTIAL/EXPIRED por entrada; abrir tela cacheada renderiza imediatamente e revalida em paralelo; invalidação explícita por generation/contexto.
- [ ] **F2-03 — Watches compartilhados.** Um watch por (contexto, scope, GVR) com ref count + idle timeout (30–60 s) para encerrar; navegar entre telas que consomem o mesmo recurso não derruba nem reabre watch a cada troca.
- [ ] **F2-04 — Bookmarks.** `allowWatchBookmarks=true` quando suportado, sem depender de intervalo fixo; bookmarks servem de checkpoint para reconexão e reduzir risco de RV antigo.
- [ ] **F2-05 — Backpressure level-driven.** Fila limitada + coalescing por resource key para a UI principal (entregar o estado atual, não toda transição intermediária). Events, logs e streams cronológicos preservam semântica própria — sem coalescing indiscriminado.
- [ ] **F2-06 — Freshness por tipo.** Watch para recursos core; CPU/memória com refresh 5–10 s; capabilities/RBAC 30–60 s; discovery/versão do Kubernetes 5–15 min; tudo invalidado por generation/contexto. Política documentada.
- [ ] **F2-07 — Overview por tiers.** Tier 1 (Pods, workloads, restarts, failed/pending, warning events) renderiza primeiro; Tier 2 (CPU/memória, Node health, storage) depois; Tier 3 (log scanning, correlações, análises) fora do caminho crítico de renderização.
- [ ] **F2-08 — Streaming lists (fast path opcional).** `sendInitialEvents=true` + bookmarks + `resourceVersionMatch=NotOlderThan` atrás de feature flag com capability detection; fallback LIST+WATCH clássico para clusters antigos. Nunca dependência exclusiva.
- [ ] **F2-09 — Scheduler adaptativo (base).** Prioridades visible/likely-next/unrelated com budget global (concorrência, QPS, retry/backoff); observação de latência, timeouts, 429 e congestionamento. Prefetch nunca compete com a requisição visível. AIMD completo permanece na F6.
- [ ] **F2-10 — Recuperação de 410 Gone.** Continue token/RV expirado: invalidar cursor local, reiniciar LIST consistente, reconstruir página/cache; a UI mostra aviso discreto de snapshot renovado — nunca erro fatal. Consistência de paginação por origem (RV/continue preservados por sequência).

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| abrir Pods → Deployments → voltar a Pods | trocas ~instantâneas via cache; watch único compartilhado; ref count correto |
| watch saudável | sem LIST completo repetido; criação/mudança/delete chega como 1 delta |
| watch proibido por RBAC | snapshot paginado + refresh manual/polling controlado; nunca polling de 1–2 s |
| 10k eventos em rajada | fila limitada; coalescing por key; UI estável; memória sob teto |
| 410 Gone / RV expirado | recuperação automática com aviso discreto; sem erro fatal |
| trocar contexto/geração | cancela goroutines, para watches, invalida cursores e caches associados |
| cluster antigo sem streaming lists | fallback clássico funciona; flag desligada |
| memória sob carga (200 namespaces) | cache e cursor dentro do orçamento; eviction observável; goroutines retornam ao baseline |

**Saída:** UI orientada a estado; primeira sincronização 1–3 s em cluster médio; troca de tela cacheada < 100 ms. **Rollback:** cache por trás de flag interna; o caminho de listagem atual continua funcional durante a fase.
