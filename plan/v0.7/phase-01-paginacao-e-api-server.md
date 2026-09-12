# Fase 1 — Paginação estratégica e uso responsável do API Server

**Prioridade:** P0. **Entrada:** F0. **Desbloqueia:** F2 e F3. **Matriz:** D06–D14, T02, T03.

Reduzir trabalho antes de paralelizar: a página 1 dos caminhos elegíveis de C01 evita varrer todos os namespaces, e o merge da janela limitada evita reordenar buffers gigantes a cada refill. Evoluir `internal/services/resources/list.go` e `internal/integration/kubernetesruntime/resources_backend.go` sem quebrar o fast path de LIST global (`PreferGlobal`/`globalListDecision`) e sem expor estado interno de paginação ao frontend — o cursor continua opaco.

Contratos transversais de execução e aceite: [C01–C06](05-contratos-e-aceite.md).

## Tarefas

- [ ] **F1-01 — Interface de estratégia de paginação.** `PaginationStrategy` com `Next(ctx, state) (Page, state, error)` e implementações `GlobalNative` (fast path atual), `NamespaceSequential` e `LazyMerge`; seleção por ordenação, autorização e pré-condições de C01; `GlobalNative` não implica sort global. A estratégia é decisão do backend; o frontend só recebe itens + cursor opaco.
- [ ] **F1-02 — NamespaceSequentialPaginator (namespace+name).** Para ordenação namespace+name: percorrer namespaces em ordem, um por vez, e **parar ao completar a página**; descendente exige comprovar a ordem também de nomes/desempates dentro de cada namespace, não apenas inverter namespaces. Usar o caminho antecipado somente quando a sequência completa for monotônica; caso contrário preservar o caminho limitado com `filterScope=page` (C01). Estado mínimo no cursor server-side; página 1 consulta apenas origens necessárias nos caminhos elegíveis.
- [ ] **F1-03 — LazyMergePaginator (janela limitada).** Para age/created/restarts/status: otimizar o merge com `container/heap`, chunk pequeno por origem (~10 itens) e refill limitado, preservando `filterScope=page`. Não anunciar sort global com chunks ordenados isoladamente; sequência global exige as pré-condições de C01. A ordenação de coleção sobre snapshot completo é integrada na F2.
- [ ] **F1-04 — Worker pool bounded.** Concorrência limitada e explícita (default 4; medir 4/6/8 com o benchmark da F0); nunca fan-out ilimitado em scopes grandes.
- [ ] **F1-05 — Request coalescing (singleflight).** Consultas idênticas (generation + contexto + scope + GVR + filtros + sort + página) executam uma única vez; refresh, re-mount e múltiplos componentes não duplicam trabalho.
- [ ] **F1-06 — Cancelamento ponta a ponta.** `AbortSignal` do React Query → transporte web/desktop → ctx do handler → cliente Kubernetes; trocar tela/filtro/ordenação cancela goroutines e streams antigos. Sem trabalho órfão (verificar com testes de race).
- [ ] **F1-07 — Debounce de busca e filtros.** ~200–300 ms quando o filtro depende do backend; menor para filtro local já carregado. Digitar "deploy" deve gerar 1–2 consultas, não 6.
- [ ] **F1-08 — Pushdown de selectors.** Matriz por recurso de suporte a `labelSelector`/`fieldSelector`; aplicar no servidor quando suportado, com fallback local documentado. Filtro aplicável no servidor não baixa tudo para filtrar localmente.
- [ ] **F1-09 — Cache de autorização e discovery.** Reutilizar autorização por (generation, namespace, apiGroup, resource, subresource, verb, resourceName) com TTL curto e invalidação por generation e 403/revogação detectada (C03) — nunca compartilhada entre contextos/identidades. Discovery de API por contexto/generation, invalidada em troca de contexto/geração ou erro de recurso.
- [ ] **F1-10 — 429 e pressão do API Server.** Respeitar `Retry-After`, backoff exponencial + jitter, reduzir fan-out após 429 repetidos, cancelar retries quando a geração mudou. `AUTHORIZATION_UNAVAILABLE` continua distinto de `Forbidden` (sem retry agressivo; refresh manual e diagnóstico).
- [ ] **F1-11 — Validação Wails nativa transferida da F0.** Executar o cenário de Pods dentro do runtime Wails/WebView real, com dados, vazio, parcial, forbidden e authorization unavailable; não substituir por `window.go` simulado. Registrar ambiente e resultado sem reabrir a F0.
- [ ] **F1-12 — Medição UX real transferida da F0.** Coletar `time_to_first_row`, página completa, filtro/sort e linhas renderizadas em browser e/ou desktop real nos portes pequeno/médio/grande; confrontar os budgets de `docs/observability.md` e separar backend sintético de pintura/interação real.
- [ ] **F1-13 — Cenário Kind real C02 transferido da F0.** Aplicar deliberadamente dataset controlado, provar 200 namespaces com LIST global autorizado e página 100, provar seleção restrita dentro do teto e rejeição acima de 100; medir over-fetch/cursor/memória e não confundir com stress sintético. Aplicação e limpeza exigem comando explícito.

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| namespace+name asc/desc com 50 namespaces e página de 50 | caminho antecipado comprova ordem completa incluindo nomes/desempates; demais casos preservam ordem por página; requests medidos por estratégia (C01) |
| sort age com 100 origens e item extremo no segundo chunk | heap preserva o contrato da janela limitada, sem anunciar ordem global; sem duplicatas/gaps em dataset estável, cobertura parcial explícita (C01) |
| duas consultas idênticas simultâneas | uma única execução no backend |
| usuário troca de tela durante fan-out | trabalho antigo cancelado; goroutines retornam ao baseline (race tests) |
| digitar "deploy" na busca | 1–2 consultas backend, não uma por tecla |
| 429 sustentado | backoff com jitter; fan-out reduzido; sem amplificação de carga |
| RBAC misto + authorization unavailable | resultados autorizados + coverage honesto; sem degradar para vazio; SAR não repetido para a mesma capability na navegação |
| Pods no runtime Wails nativo | dados/erros honestos no WebView real; transporte simulado continua apenas regressão complementar (F1-11) |
| budgets UX em runtime real | amostras por porte comparadas aos budgets, com backend e pintura/interação separados (F1-12) |
| Kind real com 200 namespaces | global autorizado, restrito dentro do teto e rejeição acima do teto comprovados separadamente (F1-13/C02) |

**Saída:** página 1 sublinear nos caminhos elegíveis de C01; over-fetch ratio ≤ ~3× nos cenários-alvo do benchmark; validações transferidas F1-11–F1-13 registradas. **Rollback:** estratégias selecionáveis por configuração interna; `GlobalNative` permanece o caminho preferencial quando autorizado.
