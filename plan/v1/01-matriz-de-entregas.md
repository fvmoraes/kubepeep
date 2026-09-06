# Matriz de entregas e aceite da v1

Fontes: [avaliação e evolução](../v1_reference/KUBEPEEP_AVALIACAO_E_PLANO_DE_EVOLUCAO.md), [performance e escalabilidade](../v1_reference/KUBEPEEP_PERFORMANCE_SCALABILITY_PLAN.md) e [refinamento UI/UX](../v1_reference/KUBEPEEP_UI_UX_REFINEMENT_PLAN.md). Famílias: **R** = correção funcional (P0), **D** = desempenho/escalabilidade, **X** = experiência/UI, **I** = investigação, **T** = transversal. **Todas as linhas são trabalho planejado** de ajuste, melhoria ou revisão — nenhuma é assumida como concluída; cada uma exige execução e evidência da sua fase: `ID | SHA | comando/cenário | resultado | limitação`.

## R — Correção funcional (P0, bloqueante)

| ID | Entrega | Fase | Aceite |
| --- | --- | --- | --- |
| R01 | Corrigir o bug de visualizar dados de Pods: a tela fica sem nada | F0 | Pods exibe dados em web e desktop, com contextos/scopes/permissões variados; causa raiz identificada na cadeia query → transporte → handler → autorização → render; regressão E2E cobre o cenário |
| R02 | Revisão total dos cliques da interface: nenhum clique morto | F0 (varredura) + F4 (fechamento) | inventário item a item (sidebar, abas horizontais, ações, links de detalhe, filtros, colunas, paleta, confirmações); cada clique executa sua função ou está explicitamente desabilitado com motivo; regressão E2E por família |
| R03 | Scope default obrigatório por contexto (regra do projeto) | F4 | um dos scopes cadastrados de cada contexto é marcado como default; abrir o KubePeep e selecionar/alternar contexto carrega esse scope **obrigatoriamente** como universo ativo; sem default definido, a UI conduz à marcação; nunca amplia RBAC nem exige `list/get namespaces` |

## D — Desempenho e escalabilidade

| ID | Entrega | Fase | Aceite |
| --- | --- | --- | --- |
| D01 | Instrumentação de leitura completa (requests/duração/itens, estratégia global×fanout, cursor store, watch, cache, 429/throttle) | F0 | métricas documentadas em `docs/observability.md` e consultáveis |
| D02 | Laboratório de benchmark Kind + `docs/performance-baseline.md` | F0 | matriz de cenários executa por script; baseline registrado antes das mudanças |
| D03 | Budgets de performance por tamanho de cluster + métricas de UX (time_to_first_row etc.) | F0 | budgets definidos e medidos conforme metas do contrato |
| D04 | Traces OTel (list/merge/cursor/watch/cache) sem atributos sensíveis | F0 | spans documentados; sem pod/UID/token/identidade |
| D05 | Cursor opaco server-side sem DTOs no transporte: revisão completa do ciclo | F0 | TTL, LRU, budget, generation binding, expiração e 410 testados; 200 namespaces × página 100 sem cursor acima do limite; `LIMIT_EXCEEDED` não reproduzível |
| D06 | Estratégias de paginação: sequencial namespace+name e lazy k-way merge para sorts globais | F1 | página 1 de namespace+name não consulta todos os namespaces; heap em sorts globais; determinismo/duplicatas/gaps testados |
| D07 | Over-fetch ratio mensurado, meta ≤ ~3× nos cenários-alvo | F0/F1 | ratio por cenário no baseline e no comparativo |
| D08 | Coalescing de consultas idênticas (singleflight) | F1 | refresh/navegação rápida não dispara consultas duplicadas |
| D09 | Cancelamento ponta a ponta de trabalho obsoleto | F1 | trocar tela/filtro/ordenação cancela; AbortSignal propagado até o cliente Kubernetes |
| D10 | Debounce de busca e filtros | F1 | digitar "deploy" gera 1–2 consultas, não 6 |
| D11 | Selectors server-side (label/field) com matriz por recurso e fallback local | F1 | filtro aplicável no servidor não busca tudo para filtrar localmente |
| D12 | Cache de autorização por generation/namespace/verb com TTL curto | F1 | sem repetição de SAR para a mesma capability na navegação; isolado por contexto/identidade |
| D13 | Cache de discovery por contexto/generation | F1 | discovery não reexecuta ao abrir cada tela |
| D14 | 429: Retry-After, backoff+jitter, fan-out reduzido, sem retry storm | F1 | cenário de 429 no benchmark não amplifica carga |
| D15 | Resource cache demand-driven (ref count, idle timeout, eviction, orçamento de memória) | F2 | cache limitado; fechar telas libera; memória sob teto; nunca em disco |
| D16 | stale-while-revalidate (FRESH/STALE/REFRESHING/PARTIAL/EXPIRED) | F2 | tela cacheada abre < 100 ms e revalida em paralelo |
| D17 | Watch compartilhado + bookmarks + reconexão com backoff+jitter | F2 | um watch por (contexto, scope, GVR) compartilhado entre telas; BOOKMARK quando suportado |
| D18 | Backpressure level-driven (fila limitada + coalescing por key) | F2 | 10k eventos não geram 10k renders; logs/eventos preservam ordem |
| D19 | Freshness por tipo (watch; métricas 5–10 s; capabilities 30–60 s; discovery 5–15 min) | F2 | política documentada e aplicada; invalidação por generation |
| D20 | Overview por tiers (crítico → métricas → caro) | F2/F3 | Tier 1 renderiza sem esperar Tier 3; log scan fora do caminho crítico |
| D21 | Streaming lists (`sendInitialEvents`) como fast path opcional | F2 | flag interna; fallback LIST+WATCH em clusters antigos |
| D22 | `useInfiniteQuery` + `maxPages` + cursor opaco | F3 | paginação incremental sem crescimento ilimitado de memória |
| D23 | Virtualização de tabelas (`@tanstack/react-virtual`) | F3 | 50k linhas sintéticas: DOM real ≈ viewport; scroll 60 FPS |
| D24 | Resultados progressivos (primeira linha cedo; contadores por namespace) | F3 | primeira linha < 500 ms em cluster médio; sem spinner global |
| D25 | `placeholderData`/`previousData` em sort/página/refresh | F3 | tabela não pisca nem apaga durante refresh |
| D26 | Prefetch da próxima página (máx. 1) | F3 | prefetch não compete com a requisição visível |
| D27 | Startup instantâneo (shell primeiro, sync assíncrono) | F3 | janela < 500 ms; shell < 800 ms; dados progressivos |
| D28 | Error boundaries por painel com retry individual | F3 | falha de métricas não derruba o dashboard |
| D29 | Batching 50–100 ms para atualizações de watch/logs | F3 | sem render por evento |
| D30 | QPS/Burst medidos e ajustados por evidência | F6 | faixa testada documentada; sem ajuste "no escuro" |
| D31 | PartialObjectMetadata para catálogos/relações/autocomplete | F6 | ganho de payload comprovado; tabelas de status continuam completas |
| D32 | Protobuf para built-ins com fallback JSON | F6 | ganho comprovado; CRDs/aggregated continuam JSON |
| D33 | Compressão avaliada por cenário (local × remoto) | F6 | decisão documentada com medição |
| D34 | Concorrência adaptativa AIMD opcional (min 2 / default 4 / max 8) | F6 | flag; default só muda com benchmark |
| D35 | Benchmark comparativo antes/depois + critérios consolidados | F7 | comparativo por cenário em docs; critérios P0/P1/P2 da referência verdes |

## X — Experiência e UI

| ID | Entrega | Fase | Aceite |
| --- | --- | --- | --- |
| X01 | Tokens globais de tipografia/espaçamento/cor; zero tamanhos arbitrários | F4 | `web/src/tokens.css` cobre a interface; auditoria sem exceções |
| X02 | Filtros ≤ ~20–30% e conteúdo ≥ ~70–80% da área útil | F4 | medição por tela; conteúdo é o protagonista |
| X03 | Navegação lateral + horizontal 100% funcional item a item | F0/F4 | checklist por destino: recurso, dados, estado visual, filtros, contexto/namespace, rota; fecha o inventário de R02 |
| X04 | Namespace global + hierarquia kubeconfig→context→scope→namespace→recurso | F4 | troca de nível superior recarrega os inferiores sem estado órfão |
| X05 | **Scope default por contexto carregado obrigatoriamente** ao abrir e ao selecionar contexto | F4 | executa R03: default marcado por contexto, carregamento automático na abertura/troca de contexto, All = universo do scope default + RBAC |
| X06 | Resource Workspace ~80% com abas por kind | F4 | abas relevantes por família; nenhum drawer estreito |
| X07 | Histórico back/forward + recursos relacionados clicáveis | F4 | retorna ao recurso anterior com aba/filtros preservados |
| X08 | Catálogo de ações rápidas por tipo de recurso (referência §8–14) | F4 | ações específicas por kind, sem ações sem sentido; restart de Pod explica recriação pelo owner |
| X09 | Ações em massa com toolbar contextual e compatibilidade por seleção | F4 | só ações válidas para todos os selecionados; confirmação clara |
| X10 | RBAC na UI: ocultar/desabilitar com tooltip; backend sempre revalida | F4 | nenhuma ação apresentada sem permissão |
| X11 | Confirmação destrutiva com alvo/namespace; digitar nome para críticas | F4 | fluxos destrutivos nunca silenciosos |
| X12 | Feedback completo (loading/toast/status; sucesso e erro real) | F4 | nenhum botão sem resultado perceptível |
| X13 | Largura total + colunas default por kind + menu Columns | F4 | colunas da referência §21 nos kinds principais |
| X14 | Cores semânticas (primário/positivo/destrutivo/atenção) | F4 | padronizadas sem exagero; dark theme preservado |
| X15 | Responsividade 1366/1440/1920/2560 | F4/F7 | sem scroll horizontal global; espaço extra vira informação |
| X16 | Validação funcional por tela (fetch/filtros/sort/paginação/refresh/live/namespace/scope/saved/columns/details/actions/RBAC/estados) | F4/F7 | checklist por tela com evidência; renderizar não basta |
| X17 | `docs/ui-ux-refinement.md` atualizado com decisões finais | F4 | documento reflete o estado entregue |

## I — Investigação

| ID | Entrega | Fase | Aceite |
| --- | --- | --- | --- |
| I01 | Índices locais (owner, status, label, selector, involvedObject, pod↔pvc/configmap) | F5 | relações e busca local sem novas chamadas ao cluster |
| I02 | Problems Engine com detectores e severidades (critical/warning/info) | F5 | contagem e lista coerentes com o estado observado; sem saúde inventada |
| I03 | Investigation View (owner chain, Service/EndpointSlice, ConfigMaps, PVCs, Events, Logs) | F5 | contexto do problema em um clique; navega pelo Workspace |
| I04 | Logs agregados por workload (multi-pod/containers, follow/previous, busca/regex, limites e cancelamento) | F5 | estilo stern leve; streams limitados; backpressure e cancelamento imediato |
| I05 | Command Palette avançada (busca no cache local, ações, navegação) | F5 | "portal" encontra Deployment/Pod/Service/ConfigMap/Ingress sem consultar o cluster |
| I06 | Performance Diagnostics (Settings → Diagnostics → Performance) | F5 | latência, p50/p95/p99, cache hit, watches, 429, sync por recurso/namespace |
| I07 | Namespace/Cluster Diagnostics | F5 | contagens, problemas por severidade e latências por namespace |

## Requisitos transversais

| ID | Critério | Fase |
| --- | --- | --- |
| T01 | Segurança: Secrets metadata-only, loopback, RBAC no backend, CSRF, cancelamento, sem conteúdo Kubernetes no navegador, cursor/cache nunca em disco | todas; gate F7 |
| T02 | API Server como recurso compartilhado: sem fan-out ilimitado, polling agressivo, retry storm ou informer global | todas |
| T03 | Estados honestos: proibido ≠ vazio ≠ unknown ≠ parcial; coverage com causa sanitizada | todas |
| T04 | Sem redesign: dark theme, roxo, sidebar + navegação horizontal e identidade atual preservados | todas; auditoria F7 |
| T05 | Telemetria sem dados sensíveis (sem pod name, UID, token, selector cru, identidade) | F0/F5 |
| T06 | Regressão: `rtk make verify`, test-race, E2E e smoke verdes a cada fechamento de fase | todas; gate F7 |

## Evidência e regra de conclusão

Ao concluir uma entrega, registrar em [`03-evidencias-execucao.md`](03-evidencias-execucao.md): `ID | SHA | teste/comando | resultado | limitação`. Não marcar teste de mock como teste de cluster real, benchmark local como cluster remoto, nem preparação local como publicação. Todas as linhas R/D/X/I/T são gate da v1; o [backlog](02-backlog-pos-v1.md) é explícito e não conta como entregue.
