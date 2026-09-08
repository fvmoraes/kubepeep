# Fase 1 — Paginação estratégica e uso responsável do API Server

**Prioridade:** P0. **Entrada:** F0. **Desbloqueia:** F2 e F3. **Matriz:** D06–D14, T02, T03.

Reduzir trabalho antes de paralelizar: a página 1 não pode exigir varrer todos os namespaces, e o sort global não pode reordenar buffers gigantes a cada refill. Evoluir `internal/services/resources/list.go` e `internal/integration/kubernetesruntime/resources_backend.go` sem quebrar o fast path de LIST global (`PreferGlobal`/`globalListDecision`) e sem expor estado interno de paginação ao frontend — o cursor continua opaco.

## Tarefas

- [ ] **F1-01 — Interface de estratégia de paginação.** `PaginationStrategy` com `Next(ctx, state) (Page, state, error)` e implementações `GlobalNative` (fast path atual), `NamespaceSequential` e `LazyMerge`; seleção por ordenação e autorização. A estratégia é decisão do backend; o frontend só recebe itens + cursor opaco.
- [ ] **F1-02 — NamespaceSequentialPaginator (namespace+name).** Para ordenação namespace+name: percorrer namespaces em ordem, um por vez, e **parar ao completar a página**; descendente inverte a lista de namespaces; estado mínimo no cursor server-side. Página 1 não consulta todos os namespaces.
- [ ] **F1-03 — LazyMergePaginator (sorts globais).** Para age/created/restarts/status: lazy k-way merge com `container/heap`, chunk pequeno por origem (~10 itens), refill somente quando o buffer da origem esgota; nunca reordenar todo o buffer por refill.
- [ ] **F1-04 — Worker pool bounded.** Concorrência limitada e explícita (default 4; medir 4/6/8 com o benchmark da F0); nunca fan-out ilimitado em scopes grandes.
- [ ] **F1-05 — Request coalescing (singleflight).** Consultas idênticas (generation + contexto + scope + GVR + filtros + sort + página) executam uma única vez; refresh, re-mount e múltiplos componentes não duplicam trabalho.
- [ ] **F1-06 — Cancelamento ponta a ponta.** `AbortSignal` do React Query → transporte web/desktop → ctx do handler → cliente Kubernetes; trocar tela/filtro/ordenação cancela goroutines e streams antigos. Sem trabalho órfão (verificar com testes de race).
- [ ] **F1-07 — Debounce de busca e filtros.** ~200–300 ms quando o filtro depende do backend; menor para filtro local já carregado. Digitar "deploy" deve gerar 1–2 consultas, não 6.
- [ ] **F1-08 — Pushdown de selectors.** Matriz por recurso de suporte a `labelSelector`/`fieldSelector`; aplicar no servidor quando suportado, com fallback local documentado. Filtro aplicável no servidor não baixa tudo para filtrar localmente.
- [ ] **F1-09 — Cache de autorização e discovery.** Autorização por (generation, namespace, apiGroup, resource, verb) com TTL curto e invalidação por generation — nunca compartilhada entre contextos/identidades. Discovery de API por contexto/generation, invalidada em troca de contexto/geração ou erro de recurso.
- [ ] **F1-10 — 429 e pressão do API Server.** Respeitar `Retry-After`, backoff exponencial + jitter, reduzir fan-out após 429 repetidos, cancelar retries quando a geração mudou. `AUTHORIZATION_UNAVAILABLE` continua distinto de `Forbidden` (sem retry agressivo; refresh manual e diagnóstico).

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| namespace+name asc/desc com 50 namespaces e página de 50 | página 1 consulta apenas os namespaces necessários; requests ∝ página, não ∝ cluster |
| sort global (age) com 100 origens | heap em uso; sem duplicatas/gaps; falha parcial de origem marcada; requests mínimos |
| duas consultas idênticas simultâneas | uma única execução no backend |
| usuário troca de tela durante fan-out | trabalho antigo cancelado; goroutines retornam ao baseline (race tests) |
| digitar "deploy" na busca | 1–2 consultas backend, não uma por tecla |
| 429 sustentado | backoff com jitter; fan-out reduzido; sem amplificação de carga |
| RBAC misto + authorization unavailable | resultados autorizados + coverage honesto; sem degradar para vazio; SAR não repetido para a mesma capability na navegação |

**Saída:** página 1 sublinear em relação ao tamanho do cluster; over-fetch ratio ≤ ~3× nos cenários-alvo do benchmark. **Rollback:** estratégias selecionáveis por configuração interna; `GlobalNative` permanece o caminho preferencial quando autorizado.
