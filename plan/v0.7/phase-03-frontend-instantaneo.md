# Fase 3 — Frontend progressivo, virtualizado e inicialização instantânea

**Prioridade:** P0/P1. **Entrada:** F1 (cursor opaco); efeito completo com F2 (cache/watch). Itens de listagem podem avançar junto com F1. **Desbloqueia:** F5. **Matriz:** D20 (tiers na UI), D22–D29, T04.

Coordenar com a F4 os arquivos compartilhados (`ResourcePages.tsx`, `DataTable`, controles de lista). Nunca spinner global esperando todos os namespaces: a primeira informação útil deve aparecer o mais cedo possível.

Contratos transversais de execução e aceite: [C01–C06](05-contratos-e-aceite.md).

## Tarefas

- [ ] **F3-01 — Infinite query.** `useInfiniteQuery` com cursor opaco, `fetchNextPage`/`hasNextPage` e `maxPages` (~5 páginas no cache da tela) para crescimento limitado de memória. **Em andamento:** Pods é o piloto com até cinco páginas retidas, linhas acumuladas, reset por identidade de seleção, recuperação de 410 e 403 bloqueando dados antigos; falta migrar as demais coleções e medir memória/scroll reais.
- [ ] **F3-02 — Virtualização.** Adicionar `@tanstack/react-virtual` às tabelas: renderizar apenas viewport + overscan, integrado às colunas/sticky headers existentes. Meta: 50k linhas sintéticas com DOM real ≈ viewport e scroll a 60 FPS. **Em andamento:** `DataTable` virtualiza automaticamente listas com mais de 100 linhas, mantém cabeçalho sticky e ARIA de contagem/índice; teste de componente com 50k e Playwright com 12k confirmam DOM limitado durante scroll. Falta medir 60 FPS e validar todas as famílias/tamanhos de linha.
- [ ] **F3-03 — Resultados progressivos.** Eventos incrementais (SSE existente/bridge Wails) preenchem a tabela e os contadores por namespace (✓ 12/20) conforme chegam; primeira linha < 500 ms em cluster médio; sem spinner global.
- [ ] **F3-04 — Sem flicker.** `placeholderData`/`previousData` em mudança de sort, página e refresh: dados anteriores da mesma seleção autorizada permanecem visíveis + indicador sutil de "Refreshing…". Contexto/scope/namespace/geração incompatíveis ou revogação removem os dados anteriores imediatamente (C03). **Em andamento:** Pods mantém linhas apenas com identidade de seleção igual; 403/generation_changed ocultam dados. Demais famílias e teste completo de troca de contexto/scope ainda pendentes.
- [ ] **F3-05 — Prefetch da próxima página.** Ao aproximar do fim (70–80% do scroll), prefetch de no máximo 1 página à frente, com prioridade baixa no scheduler (nunca compete com a requisição visível).
- [ ] **F3-06 — Startup instantâneo.** Shell/navegação utilizáveis antes de kubeconfig/discovery/RBAC/sync (tudo assíncrono); contexto anterior visível com dados progressivos. Janela < 500 ms; shell < 800 ms.
- [ ] **F3-07 — Error boundaries por painel.** Overview, métricas, logs e painéis isolados: falha em um não derruba a página; retry individual; estados partial/stale/forbidden/cancelled legíveis.
- [ ] **F3-08 — Batching e freshness no frontend.** Atualizações de watch/logs agrupadas em lotes de 50–100 ms; `staleTime`/`gcTime` do TanStack Query alinhados à política de freshness da F2-06. **Em andamento:** Pods usa 30 s de `staleTime` e 120 s de `gcTime`; retorno à tela usa a página fresca sem novo HTTP. Batching de 50–100 ms sem LIST em rajada, logs e demais queries ainda pendentes.
- [ ] **F3-09 — Revisão de render.** Memoização/selectors, evitar re-render de contexto, lazy components (editor YAML, syntax highlight, charts), debounce de busca local.

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
