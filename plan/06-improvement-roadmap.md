# Roadmap de Melhorias do KubePeep

## 1. Estrutura de fases

O roadmap divide as melhorias em fases pequenas e executáveis. Cada item segue o formato:

- ID
- Título
- Problema
- Evidência
- Impacto
- Solução proposta
- Arquivos/módulos envolvidos
- Dependências
- Riscos
- Prioridade
- Complexidade
- Critérios de aceite
- Estratégia de testes
- Estratégia de rollback

---

## Fase 0 — Correções críticas

### F0-01 — Corrigir testes flaky e race conditions

- **Problema:** testes falham intermitentemente ou consistentemente com `-race`.
- **Evidência:** `TestConcurrentRefreshesShareOneLiveReview`, `TestCheckerSnapshotProviderSanitizesFailuresAndAppliesDeadline`, `TestPortForwardTerminalPathsReleaseLoopbackPortAndAdapterGoroutine`, `TestGenerationUsesActivityIdleDeadlineAndCancelsPreviousWork`.
- **Impacto:** CI instável; races podem afetar produção.
- **Solução:** corrigir deduplicação de refresh no cache de autorização; substituir `time.Sleep` por primitivas de sincronização; executar CI com `-race`.
- **Arquivos:** `internal/services/authorization/service.go`, `internal/api/snapshot.go`, `internal/services/actions/portforward.go`, `internal/adapters/kubernetes/generation.go`, testes correspondentes.
- **Dependências:** nenhuma.
- **Riscos:** alteração em lógica de cache pode afetar performance de autorização.
- **Prioridade:** P0. **Complexidade:** L.
- **Critérios de aceite:** `go test -race ./...` passa 10 vezes seguidas; testes afetados são determinísticos.
- **Testes:** unitários com race detector; repetir 10x.
- **Rollback:** revert para versão anterior do cache; validar que comportamento funcional permanece.

### F0-02 — Remover `context.Background()` de serviços de ação

- **Problema:** ações usam `context.Background()` quando `ctx == nil`, desconectando do lifecycle.
- **Evidência:** `internal/services/actions/service.go`, `exec.go`, `portforward.go`.
- **Impacto:** operações podem vazar em shutdown ou troca de geração.
- **Solução:** retornar erro quando `ctx == nil`; garantir que todos os callers passem contexto cancelável.
- **Arquivos:** `internal/services/actions/*.go`, callers em handlers.
- **Dependências:** F0-01 (para testes estáveis).
- **Riscos:** mudança de contrato interno; callers precisam ser revisados.
- **Prioridade:** P0. **Complexidade:** M.
- **Critérios de aceite:** nenhum `context.Background()` em caminhos de produção; testes de cancelamento de ações passam.
- **Testes:** unitários e integração de cancelamento.
- **Rollback:** reverter para fallback seguro com log de aviso.

### F0-03 — Eliminar mutação global de `os.Stderr`

- **Problema:** `client_factory.go` altera `os.Stderr` global durante construção do client-go.
- **Evidência:** `internal/adapters/kubernetes/client_factory.go:211`.
- **Impacto:** concorrência pode redirecionar stderr de outras goroutines.
- **Solução:** usar mecanismo thread-safe para suprimir stderr do plugin exec.
- **Arquivos:** `internal/adapters/kubernetes/client_factory.go`, `client_factory_test.go`.
- **Dependências:** nenhuma.
- **Riscos:** mudança no tratamento de erros de plugin exec.
- **Prioridade:** P1. **Complexidade:** M.
- **Critérios de aceite:** `os.Stderr` não é mutado globalmente; teste de concorrência passa; erros de plugin ainda são sanitizados.
- **Testes:** teste de concorrência de criação de clients; testes existentes de plugin exec.
- **Rollback:** reverter para mutação global se solução quebrar cross-build.

---

## Fase 1 — Consistência visual

### F1-01 — Criar tokens de design centralizados

- **Problema:** espaçamentos, raios, sombras e tipografia são hardcoded.
- **Evidência:** `web/src/styles.css` com dezenas de valores literais.
- **Impacto:** inconsistência visual e dificuldade de manutenção.
- **Solução:** definir tokens no `@theme` do Tailwind 4 e em CSS custom properties.
- **Arquivos:** `web/src/styles.css`, `web/tailwind.config.*` se existir.
- **Dependências:** nenhuma.
- **Riscos:** mudança visual indesejada se tokens não forem mapeados corretamente.
- **Prioridade:** P1. **Complexidade:** M.
- **Critérios de aceite:** todos os tokens documentados; nenhum valor hardcoded fora dos tokens (salvo exceções justificadas); screenshots comparativos.
- **Testes:** testes visuais regressão (Playwright screenshots).
- **Rollback:** reverter `styles.css`.

### F1-02 — Criar componentes atômicos reutilizáveis

- **Problema:** botões, inputs, selects, tabelas são recriados inline.
- **Evidência:** `ResourcePages.tsx`, `NamespaceScopeEditor.tsx`, `LogsPage.tsx`.
- **Impacto:** duplicação, inconsistência, dificuldade de evolução.
- **Solução:** criar `web/src/components/ui/` com `Button`, `Input`, `Select`, `Badge`, `Card`, `Table`, `Modal`, `Drawer`, `Tabs`, `EmptyState`, `Skeleton`.
- **Arquivos:** novos em `web/src/components/ui/`; componentes existentes migrados gradualmente.
- **Dependências:** F1-01.
- **Riscos:** refatoração ampla; regressões visuais.
- **Prioridade:** P1. **Complexidade:** L.
- **Critérios de aceite:** componentes atômicos testados; 80% dos botões/inputs migrados.
- **Testes:** Vitest para componentes; Playwright para regressão visual.
- **Rollback:** manter fallback para classes antigas durante transição.

### F1-03 — Integrar SVGs oficiais da marca

- **Problema:** `src/kubepeep-logo.svg` e `src/kubepeep-name.svg` não são usados.
- **Evidência:** sidebar usa `"kp"` textual e wordmark `"kubePeep"`.
- **Impacto:** identidade visual fraca.
- **Solução:** criar componentes `BrandLogo` e `BrandWordmark`; substituir textos.
- **Arquivos:** `web/src/components/BrandLogo.tsx`, `BrandWordmark.tsx`; `web/src/App.tsx`; `web/index.html`.
- **Dependências:** nenhuma.
- **Riscos:** distorção ou fundo indesejado nos SVGs.
- **Prioridade:** P1. **Complexidade:** XS.
- **Critérios de aceite:** SVGs aparecem em sidebar, cabeçalho e tela inicial; proporção preservada; sem distorção.
- **Testes:** Vitest para renderização; Playwright screenshots.
- **Rollback:** reverter para texto.

### F1-04 — Criar `DataTable` reutilizável

- **Problema:** tabelas duplicadas e inconsistentes.
- **Evidência:** `.dashboard-table`, `.resource-table`, `.permissions-table`.
- **Impacto:** aparência inconsistente.
- **Solução:** componente `DataTable` com colunas configuráveis, ordenação, seleção.
- **Arquivos:** `web/src/components/ui/DataTable.tsx`; migrar tabelas existentes.
- **Dependências:** F1-01, F1-02.
- **Riscos:** mudança visual em todas as listas.
- **Prioridade:** P1. **Complexidade:** M.
- **Critérios de aceite:** todas as tabelas usam `DataTable`; estilos consistentes.
- **Testes:** Vitest; Playwright.
- **Rollback:** reverter tabelas individuais.

---

## Fase 2 — Navegação e experiência Kubernetes

### F2-01 — Implementar parser composto de busca

- **Problema:** busca é substring simples; não suporta termos compostos, exclusão ou multitermo.
- **Evidência:** `web/src/components/ResourceListControls.tsx`, backend em `internal/integration/kubernetesruntime/resources_sort.go`.
- **Impacto:** dificuldade de encontrar recursos em clusters grandes.
- **Solução:** parser no backend (`search`) suportando `termo -excluido "frase exata"`.
- **Arquivos:** `internal/services/resources/search.go`, handler, `ResourceListControls.tsx`.
- **Dependências:** F1-04.
- **Riscos:** mudança de contrato de busca; performance com muitos termos.
- **Prioridade:** P1. **Complexidade:** L.
- **Critérios de aceite:** busca `payment !failed` funciona; documentação atualizada; testes de casos positivos/negativos.
- **Testes:** unitários no parser; integração; E2E.
- **Rollback:** desabilitar parser composto; fallback para substring.

### F2-02 — Adicionar painel lateral de detalhes

- **Problema:** detalhe ocupa tela inteira e perde contexto da lista.
- **Evidência:** `ResourcePages.tsx` navega para rota de detalhe.
- **Impacto:** navegação lenta; perda de filtros/scroll.
- **Solução:** `Drawer` que abre sobre a lista, mantendo contexto.
- **Arquivos:** `web/src/components/ui/Drawer.tsx`, `ResourcePages.tsx`, rotas.
- **Dependências:** F1-02.
- **Riscos:** mudança de navegação; deep links de detalhe.
- **Prioridade:** P1. **Complexidade:** L.
- **Critérios de aceite:** detalhe abre em painel lateral; lista permanece visível; URL reflete recurso selecionado; back funciona.
- **Testes:** Vitest; Playwright E2E.
- **Rollback:** voltar a detalhe em tela cheia.

### F2-03 — Syntax highlighting para YAML

- **Problema:** YAML exibido em `<pre>` sem highlight.
- **Evidência:** `ResourcePages.tsx`.
- **Impacto:** dificuldade de leitura.
- **Solução:** integrar `react-syntax-highlighter` ou Shiki para YAML.
- **Arquivos:** `web/src/components/YamlViewer.tsx`; `ResourcePages.tsx`.
- **Dependências:** nenhuma.
- **Riscos:** aumento de bundle; Secret nunca deve passar pelo viewer (já recusado no backend).
- **Prioridade:** P1. **Complexidade:** S.
- **Critérios de aceite:** YAML com highlight; bundle impact documentado; Secret continua sem rota YAML.
- **Testes:** Vitest; Playwright; inspeção de rotas.
- **Rollback:** remover biblioteca; voltar a `<pre>`.

### F2-04 — Melhorar experiência de logs

- **Problema:** não há pause no follow, highlight de JSON/níveis, filtro por nível.
- **Evidência:** `LogsPage.tsx`.
- **Impacto:** logs densos difíceis de escanear.
- **Solução:** pause/continue no follow; destaque de JSON; filtro local por nível (`error`, `warn`, `info`).
- **Arquivos:** `web/src/components/LogsPage.tsx`, `LogControls.tsx`, `LogOutput.tsx`.
- **Dependências:** F1-02.
- **Riscos:** performance com muitas linhas.
- **Prioridade:** P1. **Complexidade:** M.
- **Critérios de aceite:** botão pause/continue; JSON destacado; filtro por nível funciona; follow continua respeitando geração.
- **Testes:** Vitest; Playwright.
- **Rollback:** desabilitar melhorias de logs.

---

## Fase 3 — Dashboard e diagnóstico

### F3-01 — Navegação contextual nos cards do dashboard

- **Problema:** cards do dashboard não navegam para lista filtrada.
- **Evidência:** `Dashboard.tsx`.
- **Impacto:** dashboard perde valor como ponto de partida.
- **Solução:** cada card clicável leva à rota correspondente com filtros.
- **Arquivos:** `web/src/components/Dashboard.tsx`, rotas.
- **Dependências:** F2-01.
- **Riscos:** mudança de navegação.
- **Prioridade:** P1. **Complexidade:** S.
- **Critérios de aceite:** clique em "Pods problemáticos" abre `/pods?problematic=true`; clique em restarts abre `/pods?restarts=gte3`; filtros aplicados.
- **Testes:** Vitest; Playwright.
- **Rollback:** remover links dos cards.

### F3-02 — Resumo de saúde por namespace

- **Problema:** dashboard não mostra saúde por namespace.
- **Evidência:** `Dashboard.tsx`, `internal/services/dashboard/summary.go`.
- **Impacto:** dificuldade de identificar namespace problemático.
- **Solução:** adicionar bloco/tabela de saúde por namespace no dashboard.
- **Arquivos:** backend (`summary.go`, `pods.go`, `workloads.go`); frontend (`Dashboard.tsx`).
- **Dependências:** F3-01.
- **Riscos:** aumento de carga no cluster.
- **Prioridade:** P2. **Complexidade:** M.
- **Critérios de aceite:** bloco mostra pods problemáticos/restarts/workloads degradados por namespace; clique navega para lista filtrada.
- **Testes:** unitários backend; Vitest; E2E Kind.
- **Rollback:** ocultar bloco.

### F3-03 — Indicador de stale por bloco

- **Problema:** blocos do dashboard não indicam idade dos dados.
- **Evidência:** `Dashboard.tsx`.
- **Impacto:** usuário pode tomar decisões com dados desatualizados.
- **Solução:** exibir `collectedAt` e indicador visual de stale (> 60s).
- **Arquivos:** `web/src/components/Dashboard.tsx`.
- **Dependências:** nenhuma.
- **Riscos:** nenhum.
- **Prioridade:** P2. **Complexidade:** XS.
- **Critérios de aceite:** cada bloco mostra idade; stale indica cor/ícone.
- **Testes:** Vitest.
- **Rollback:** remover indicador.

---

## Fase 4 — Confiabilidade e desempenho

### F4-01 — Centralizar ports em `internal/ports/`

- **Problema:** interfaces de port dispersas.
- **Evidência:** `internal/ports/doc.go` vazio.
- **Impacto:** acoplamento, desalinhamento documentação/código.
- **Solução:** criar interfaces em `internal/ports/`; handlers dependem delas.
- **Arquivos:** `internal/ports/*.go`, handlers, serviços.
- **Dependências:** F0-01, F0-02.
- **Riscos:** refatoração ampla.
- **Prioridade:** P1. **Complexidade:** L.
- **Critérios de aceite:** `internal/ports/` contém todas as interfaces documentadas; handlers não conhecem implementações concretas.
- **Testes:** testes de arquitetura (verificar imports); testes existentes.
- **Rollback:** manter interfaces antigas como aliases.

### F4-02 — Consolidar dashboard/resources

- **Problema:** duplicação de classificação e listagem.
- **Evidência:** `internal/services/dashboard/` e `internal/services/resources/`.
- **Impacto:** manutenção duplicada.
- **Solução:** reutilizar ports/classificadores de recursos no dashboard.
- **Arquivos:** `internal/services/dashboard/*`, `internal/services/resources/*`.
- **Dependências:** F4-01.
- **Riscos:** mudança de comportamento do dashboard.
- **Prioridade:** P1. **Complexidade:** L.
- **Critérios de aceite:** dashboard e listas usam mesmos classificadores; testes do dashboard passam.
- **Testes:** unitários; integração; E2E Kind.
- **Rollback:** reverter para implementações separadas.

### F4-03 — Cobertura de testes em adapters e integração

- **Problema:** cobertura baixa nas camadas de integração.
- **Evidência:** `internal/adapters/kubernetes` 41–57%, `internal/integration/kubernetesruntime` ~41%.
- **Impacto:** bugs de integração só detectados em E2E.
- **Solução:** adicionar testes com mocks/fakes e kube-apiserver mock.
- **Arquivos:** `internal/adapters/kubernetes/*_test.go`, `internal/integration/kubernetesruntime/*_test.go`.
- **Dependências:** nenhuma.
- **Riscos:** testes complexos e lentos.
- **Prioridade:** P1. **Complexidade:** L.
- **Critérios de aceite:** cobertura ≥ 70% nos pacotes alvo; testes passam.
- **Testes:** novos testes unitários/integração.
- **Rollback:** remover testes que tornarem suite instável.

---

## Fase 5 — Observabilidade

Ver `08-observability-plan.md`.

---

## Fase 6 — Testes e documentação

### F6-01 — Adicionar testes para `internal/application.Compose`

- **Problema:** composição do core sem testes.
- **Evidência:** `internal/application/application.go`.
- **Impacto:** erros de wiring só em runtime.
- **Solução:** testes de composição com mocks.
- **Arquivos:** `internal/application/application_test.go`.
- **Prioridade:** P1. **Complexidade:** M.

### F6-02 — Criar documentação de design system

- **Problema:** ausência de guia visual.
- **Solução:** `docs/design-system.md`.
- **Prioridade:** P1. **Complexidade:** M.

### F6-03 — Atualizar documentação arquitetural

- **Problema:** ports documentados não centralizados.
- **Solução:** atualizar `docs/architecture.md`.
- **Dependências:** F4-01.
- **Prioridade:** P1. **Complexidade:** S.

### F6-04 — Criar documentação RBAC e observabilidade

- **Solução:** `docs/rbac-requirements.md`, `docs/observability.md`.
- **Prioridade:** P2. **Complexidade:** S/M.

---

## Fase 7 — Evoluções futuras (P3)

### F7-01 — Favoritos e recentes

- Implementar favoritos/recentes com schema allowlisted.
- **Prioridade:** P3. **Complexidade:** M.

### F7-02 — Diff de YAML

- Comparar revisões/origens de recursos, recusando Secret.
- **Prioridade:** P3. **Complexidade:** L.

### F7-03 — Multi-contexto somente leitura

- Agregar recursos de múltiplos contextos com proveniência.
- **Prioridade:** P3. **Complexidade:** XL.

### F7-04 — Busca global de recursos

- Paleta/Command K indexando recursos visíveis (não conteúdo sensível).
- **Prioridade:** P3. **Complexidade:** L.

### F7-05 — Terminal exec com xterm.js

- Substituir `<pre>` por terminal profissional.
- **Prioridade:** P3. **Complexidade:** L.

---

## 2. Resumo por prioridade

| Prioridade | Quantidade | Itens principais |
| --- | --- | --- |
| P0 | 2 | Corrigir races; remover `context.Background()` de ações. |
| P1 | 11 | Tokens, componentes atômicos, SVGs, DataTable, parser de busca, painel lateral, YAML highlight, logs, ports, consolidar dashboard/resources, cobertura de testes. |
| P2 | 8 | Navegação dashboard, saúde por namespace, stale, gerenciador de port-forward, aplicação Compose, documentação. |
| P3 | 5 | Favoritos, diff, multi-contexto, busca global, xterm.js. |
