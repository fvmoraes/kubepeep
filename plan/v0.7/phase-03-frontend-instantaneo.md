# Fase 3 — Frontend progressivo, virtualizado e inicialização instantânea

**Prioridade:** P0/P1. **Entrada:** F1 (cursor opaco); efeito completo com F2 (cache/watch). Itens de listagem podem avançar junto com F1. **Desbloqueia:** F5. **Matriz:** D20 (tiers na UI), D22–D29, T04.

Coordenar com a F4 os arquivos compartilhados (`ResourcePages.tsx`, `DataTable`, controles de lista). Nunca spinner global esperando todos os namespaces: a primeira informação útil deve aparecer o mais cedo possível.

Contratos transversais de execução e aceite: [C01–C06](05-contratos-e-aceite.md).

## Tarefas

- [x] **F3-01 — Infinite query.** `useInfiniteQuery` com cursor opaco, `fetchNextPage`/`hasNextPage` e `maxPages=5` em Pods, Workloads, Events, Network, Config e Nodes. Cinco páginas reais do escopo Kind 50×10 devolveram 500 Pods sem repetição; crescimento do heap JS de 1,6–4,6 MiB nas amostras e cursor final encerrado.
- [x] **F3-02 — Virtualização.** `@tanstack/react-virtual` mantém apenas viewport + overscan nas tabelas, cabeçalho sticky e ARIA. Teste de componente com 50k linhas, scroll browser contínuo com 12k Pods e Events com alturas variáveis passaram; amostra isolada do scroll ficou em 60 FPS com DOM abaixo de 60 linhas.
- [x] **F3-03 — Resultados progressivos.** SSE envia páginas parciais autorizadas e contadores por namespace; um preview de até 20 Pods do primeiro namespace autorizado é preparado enquanto a Overview já está utilizável e substituído pela LIST completa. No Kind 50×10, cinco processos frios após 1 s de Overview tiveram primeira linha em 75–97 ms (p95 conservador 97 ms) e página de 100 itens em 966–1.012 ms (p95 1.012 ms) após o clique; o binário final marcou 99 ms/992 ms. Preview é não selecionável e 403 o remove.
- [x] **F3-04 — Sem flicker.** `placeholderData` mantém linhas apenas na mesma identidade autorizada e mostra “Refreshing…”. Testes cobrem sort/refresh, namespace local, contexto, scope, geração, respostas tardias e 403/503; seleção incompatível ou revogação esconde linhas imediatamente.
- [x] **F3-05 — Prefetch da próxima página.** Scroll a 75% agenda no máximo uma próxima página no idle com `likely-next`; scheduler preserva a LIST visível e pode responder `PREFETCH_DEFERRED`. Browser e backend cobrem 429, concorrência, cursor e ação manual; percurso real de cinco páginas concluiu sem concorrência duplicada.
- [x] **F3-06 — Startup instantâneo.** Shell e navegação carregam antes de discovery/RBAC/sync; Dashboard e painéis pesados são lazy. Wails nativo em perfil isolado: cinco amostras após aquecimento, janela p95 234 ms e shell p95 653 ms.
- [x] **F3-07 — Error boundaries por painel.** Overview, métricas, logs e painéis isolam falhas, com retry individual e estados partial/stale/forbidden/cancelled; testes cobrem Metrics API ausente e recuperação sem derrubar o restante.
- [x] **F3-08 — Batching e freshness no frontend.** Watch e logs agrupam em 75 ms; 10k deltas de watch geram uma revalidação HTTP e menos de seis commits React no teste de rajada. Coleções usam 30 s de `staleTime`/120 s de `gcTime`; métricas usam 8 s, capabilities 45 s e preferências 60 s de `staleTime`.
- [x] **F3-09 — Revisão de render.** Selectors/memoização e debounce de 100 ms na busca local; workspace, YAML, ações e Dashboard carregados sob demanda. Bundle inicial de produção: cerca de 340 kB (105 kB gzip), abaixo dos 496 kB (151 kB gzip) anteriores.

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| 12k Pods sintéticos com scroll contínuo | 60 FPS alvo; DOM real ≈ viewport; sem travamento |
| abrir Pods em cluster médio | primeira linha < 500 ms; página completa < 1,5 s; contadores progressivos |
| mudar sort / refresh na mesma seleção autorizada | tabela anterior permanece + indicador sutil; sem flicker; troca de seleção/revogação remove dados incompatíveis (C03) |
| abrir o aplicativo | janela < 500 ms; shell < 800 ms; dados progressivos < 1–2 s |
| painel de métricas falha (API ausente) | restante do dashboard útil; retry individual; estado honesto |
| watch emitindo em rajada | atualizações agrupadas 50–100 ms; CPU estável |

**Saída:** performance percebida — abriu, viu, investigou. **Rollback:** virtualização habilitável por tabela; a listagem atual continua válida enquanto a fase avança.
