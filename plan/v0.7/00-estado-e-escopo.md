# Estado atual e limites da v0.7

Base de código: HEAD local `d71d488` (com release `0.6.1` no histórico); a referência local `origin/main` aponta para `598e167` (release `0.6.2`), ainda não integrada nesta branch, com o plano de expansão de recursos registrado em [`plan/v0/03-evidencias-execucao.md`](../v0/03-evidencias-execucao.md). Inventário revisado por Codebase MCP e leitura da fonte em setembro de 2026.

**Enquadramento desta v0.7:** todo o conteúdo dos três documentos em [`../v0.7_reference/`](../v0.7_reference/) é tratado como **trabalho de ajuste, melhoria e revisão a executar nas fases** — independentemente do que já exista na base. Nada é assumido como pronto: cada entrega passa pelo aceite da sua fase, com evidência própria. Os caminhos abaixo apenas localizam onde cada tema toca o código hoje; eles não substituem a execução nem o aceite.

## Onde as referências tocam a base (mapa de trabalho, não lista de concluído)

| Tema | Caminhos envolvidos | Trabalho da v0.7 |
| --- | --- | --- |
| Cursor/paginação | `internal/api/cursor_store.go`, `internal/services/resources/cursor.go`, `list.go` | revisar ciclo completo (TTL, LRU, budget, 410, expiração) e evoluir para estratégias de paginação (F0/F1) |
| Instrumentação | `internal/observability/metrics.go`, `docs/observability.md` | completar métricas, traces, baseline e budgets (F0) |
| Namespace/scope | `web/src/context/GlobalNamespace.tsx`, `internal/services/namespaces/` | **scope default obrigatório por contexto** (F4) e revisão da hierarquia |
| Workspace e ações | `web/src/components/workspace/`, `ResourceActions.tsx` | completar abas/ações por kind, massa, RBAC na UI (F4) |
| Watch/cache | `internal/services/resources/watch.go`, `internal/adapters/kubernetes/client_cache.go` | cache de snapshot sob demanda, bookmarks, backpressure (F2) |
| Frontend de listagem | `web/src/components/ResourcePages.tsx`, `ResourceLiveUpdates.tsx`, `api/client.ts` | progressividade, virtualização, infinite query, startup (F3) |
| Paleta/diagnóstico | `web/src/components/CommandCenter.tsx`, `internal/observability/` | busca local, problems, diagnostics (F5) |
| Laboratório | `test/kind/harness.sh` | matriz de benchmark de performance (F0) |

Infinite query, virtualização, coalescing de consultas, resource cache de snapshot, índices locais e Problems Engine **não existem** nesta base e serão construídos nas fases correspondentes. Usar os caminhos reais acima, sem criar duplicatas.

## Contrato da v0.7

- **Revisão funcional total (P0, bloqueante):** corrigir o bug de **visualizar dados de Pods e a tela ficar sem nada** e fazer uma **revisão completa de todos os cliques que não funcionam na interface** (sidebar, abas horizontais, ações, links, filtros, colunas, paleta). Nenhum clique morto sobrevive à release; a correção é gate da F0 e reauditada na F4/F7. [Fase 0](phase-00-baseline-instrumentacao.md).
- **Scope default obrigatório por contexto (regra do projeto):** todo contexto com namespaces cadastrados tem **um de seus scopes marcado como default**; ao abrir o KubePeep e ao selecionar/alternar contexto, esse scope default é carregado **obrigatoriamente** como universo ativo da sessão. Sem scope default definido, a UI conduz à marcação em vez de assumir All namespaces. Detalhe na [Fase 4](phase-04-refinamento-ui-ux.md).
- **Princípio único de performance:** buscar somente o necessário, reutilizar o que já foi buscado, transmitir somente o necessário e renderizar somente o que está visível.
- **Metas** (referência de performance §43/§53, cluster médio): primeira linha < 500 ms; página completa < 1,5 s; tela cacheada < 100 ms; watch → UI < 250 ms; scroll a 60 FPS; janela aberta < 500 ms e shell < 800 ms. Crescimento de tempo **sublinear** nos caminhos antecipados elegíveis; sorts sobre dados ainda não carregados respeitam C01, sem promessa global indevida.
- **UX:** refinamento, não redesign. Tokens globais, proporção filtros/conteúdo, workspace por recurso, ações contextuais, RBAC visível na UI. O estilo atual (dark, roxo, sidebar + navegação horizontal) é aprovado e preservado.
- **Investigação:** Problems Engine, Investigation View, logs agregados por workload, paleta com busca local e diagnostics — posicionamento FAST + SAFE + PROBLEM-ORIENTED + DEVELOPER-FIRST.
- **Não é escopo da v0.7:** multi-contexto simultâneo, Prometheus, diff entre origens, Helm/Gateway/CR genérico, plugins — tudo no [backlog](02-backlog-pos-v1.md).

## Premissas de segurança preservadas (não negociáveis)

Secrets permanecem metadata-only (sem conteúdo em cache, cursor, telemetria ou log); exec e port-forward em loopback com autorização; RBAC revalidado no backend sempre; CSRF nas operações que o exigem; cancelamento e limites em toda leitura; nenhum conteúdo Kubernetes no navegador (sem `localStorage`/`sessionStorage`/IndexedDB); cursor e cache de recursos apenas em memória, nunca em disco nem no SQLite (que segue restrito a preferências); API Server é recurso compartilhado; telemetria sem pod name, UID, token, selector cru ou identidade.

## Decisões que evitam regressões

1. **Não aumentar** limite de cursor, goroutines, QPS/Burst ou timeout "no escuro". Tuning apenas por benchmark (anti-patterns da referência de performance §80).
2. Cache de recursos em memória com orçamento e eviction. SQLite permanece apenas para preferências/configurações.
3. Watch é **demand-driven** (ref count + idle timeout). Nenhum informer permanente de todos os GVRs; KubePeep é dashboard desktop, não controller.
4. WATCH saudável elimina polling completo. Fallback é refresh manual ou polling controlado com jitter — nunca LIST completo a cada 1–2 s.
5. Streaming lists, Protobuf, PartialObjectMetadata, compressão e concorrência adaptativa são **opt-in com flag**, só ativados com ganho comprovado; clusters antigos continuam suportados pelo fallback clássico.
6. `meta.page.filterScope` continua descrevendo filtro/ordenação. As estratégias de paginação não expõem estado interno ao frontend; o cursor permanece opaco. Aplicar [C01–C06](05-contratos-e-aceite.md) a ordenação, limites, identidade, instrumentação, gates condicionais e cobertura.
7. `AUTHORIZATION_UNAVAILABLE` ≠ `Forbidden`; proibição autoritativa nunca degrada para lista vazia; cobertura parcial continua com causa sanitizada.
8. A severidade do Problems Engine é diagnóstico derivado de estado observado — nunca inventa saúde nem infere permissão a partir de Role.
9. Prefetch nunca compete com a requisição visível; prioridade visible > likely-next > unrelated.
10. Nenhum redesign: preservar identidade visual, componentes e tokens existentes; F4 audita, não substitui.
11. O scope default é uma escolha **local** do usuário (preferência por contexto); nunca implica `list/get namespaces`, criação de objetos Namespace ou ampliação de RBAC. O carregamento obrigatório do scope default não contorna autorização por recurso.

## Evidência inicial a registrar na primeira execução

- SHA de partida, `rtk make verify` verde como referência de regressão e worktree limpo.
- Reprodução do bug de Pods sem dados: modo (web/desktop), contexto, scope, permissões e camada responsável — antes de qualquer correção.
- Inventário completo de cliques não funcionais por tela, com camada responsável de cada um.
- Baseline de performance **antes** de qualquer mudança de estratégia (F0), com o script do laboratório Kind executável.
- Bloqueios de ambiente (Docker ausente, cluster indisponível) registrados como pendentes, nunca como aprovados.
