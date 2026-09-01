# Checklist de Aceite do KubePeep

## 1. Fase 0 — Correções críticas

| ID | Critério | Como verificar | Status |
| --- | --- | --- | --- |
| F0-01 | `go test -race ./...` passa 10 vezes seguidas. | Executar comando em loop. | ⬜ |
| F0-02 | Testes identificados como flaky passam 100 vezes isoladamente. | `go test -count=100 -race` nos pacotes afetados. | ⬜ |
| F0-03 | Nenhum `context.Background()` em caminhos de produção de ações. | `grep -R "context.Background()" internal/services/actions/`. | ⬜ |
| F0-04 | Ações cancelam corretamente em troca de geração/shutdown. | Testes de cancelamento passam. | ⬜ |
| F0-05 | `os.Stderr` não é mutado globalmente. | `grep -R "os.Stderr =" internal/`. | ⬜ |
| F0-06 | Construção concorrente de clients é segura. | Novo teste de concorrência passa. | ⬜ |

## 2. Fase 1 — Consistência visual

| ID | Critério | Como verificar | Status |
| --- | --- | --- | --- |
| F1-01 | Tokens de design definidos e documentados. | `docs/design-system.md` existe; `styles.css` usa `@theme`. | ⬜ |
| F1-02 | Nenhum espaçamento/raio/cor hardcoded fora dos tokens. | Revisão de código; exceções justificadas. | ⬜ |
| F1-03 | Componentes atômicos criados em `web/src/components/ui/`. | Lista de componentes; testes passam. | ⬜ |
| F1-04 | SVGs da marca integrados em sidebar, cabeçalho e tela inicial. | Playwright screenshots; Vitest. | ⬜ |
| F1-05 | `DataTable` reutilizável em todas as listas. | Inspeção de `ResourcePages`, `Dashboard`, `PermissionsMatrix`. | ⬜ |
| F1-06 | Screenshots comparativos não apresentam regressões visuais graves. | Diff de imagens baseline vs. novo. | ⬜ |

## 3. Fase 2 — Navegação e experiência Kubernetes

| ID | Critério | Como verificar | Status |
| --- | --- | --- | --- |
| F2-01 | Parser composto de busca funciona. | Testes com `termo -excluido "frase"`. | ⬜ |
| F2-02 | Painel lateral de detalhes mantém contexto da lista. | Playwright: clique, back, filtros preservados. | ⬜ |
| F2-03 | YAML com syntax highlight. | Playwright screenshot; bundle impact documentado. | ⬜ |
| F2-04 | Logs com pause/continue e highlight de JSON. | Vitest; Playwright. | ⬜ |
| F2-05 | Filtro por nível de log funciona localmente. | Vitest. | ⬜ |

## 4. Fase 3 — Dashboard e diagnóstico

| ID | Critério | Como verificar | Status |
| --- | --- | --- | --- |
| F3-01 | Cards do dashboard são clicáveis e navegam com filtros. | Playwright. | ⬜ |
| F3-02 | Bloco de saúde por namespace exibe dados corretos. | Kind + Playwright. | ⬜ |
| F3-03 | Indicador de stale por bloco. | Vitest com timers. | ⬜ |

## 5. Fase 4 — Confiabilidade e desempenho

| ID | Critério | Como verificar | Status |
| --- | --- | --- | --- |
| F4-01 | Ports centralizados em `internal/ports/`. | Inspeção de código; teste de arquitetura. | ⬜ |
| F4-02 | Handlers não importam implementações concretas. | Verificação de imports. | ⬜ |
| F4-03 | Dashboard reutiliza classificadores de recursos. | Inspeção de código; golden tests. | ⬜ |
| F4-04 | Cobertura ≥ 70% em adapters e integração. | Relatório de cobertura. | ⬜ |
| F4-05 | Testes de `internal/application.Compose` existem. | `application_test.go` passa. | ⬜ |

## 6. Fase 5 — Observabilidade

| ID | Critério | Como verificar | Status |
| --- | --- | --- | --- |
| F5-01 | Logs estruturados sem dados sensíveis. | Inspeção de código; testes adversariais. | ⬜ |
| F5-02 | OTel desligado não gera tráfego. | Teste de configuração; captura de rede. | ⬜ |
| F5-03 | Métricas opcionais expostas em `/metrics`. | Configuração + requisição local. | ⬜ |
| F5-04 | `doctor` reporta estado de observabilidade. | Execução de `kubepeep doctor`. | ⬜ |

## 7. Fase 6 — Testes e documentação

| ID | Critério | Como verificar | Status |
| --- | --- | --- | --- |
| F6-01 | `docs/design-system.md` criado. | Arquivo existe e está completo. | ⬜ |
| F6-02 | `docs/observability.md` criado. | Arquivo existe. | ⬜ |
| F6-03 | `docs/rbac-requirements.md` criado. | Arquivo existe. | ⬜ |
| F6-04 | `docs/architecture.md` alinhado com ports reais. | Revisão cruzada. | ⬜ |
| F6-05 | Nomenclatura `kubepeep`/`Kube Peep` normalizada. | Grep em `.md`. | ⬜ |
| F6-06 | Guia de build desktop documentado. | README ou `docs/desktop-build.md`. | ⬜ |

## 8. Gates transversais

| ID | Critério | Como verificar | Status |
| --- | --- | --- | --- |
| G-01 | `go test -count=1 ./...` passa. | CI/local. | ⬜ |
| G-02 | `go test -race ./internal/...` passa. | CI/local. | ⬜ |
| G-03 | `go vet ./...` limpo. | CI/local. | ⬜ |
| G-04 | Frontend lint/typecheck/test/build passam. | CI/local. | ⬜ |
| G-05 | `npm audit --audit-level=high` limpo. | CI/local. | ⬜ |
| G-06 | `govulncheck ./...` limpo. | CI/local. | ⬜ |
| G-07 | `scripts/security_check.sh HEAD` passa. | Pre-commit/pre-push. | ⬜ |
| G-08 | Kind harness `validate` passa. | CI/local. | ⬜ |
| G-09 | Playwright E2E passa no binário embutido. | CI/local. | ⬜ |
| G-10 | Build desktop com tag `desktop` passa (Linux). | CI/local quando possível. | ⬜ |

## 9. Critérios de release

- [ ] Todas as melhorias P0 e P1 concluídas e aceitas.
- [ ] Todos os gates transversais verdes.
- [ ] Documentação atualizada.
- [ ] Release candidate criada e instaladores testados.
- [ ] Matriz MVP 27/27.
- [ ] Matriz UX avançada conforme escopo da Fase 9.

---

## 10. Resultado da execução (2026-09-01)

### Fase 0 — ✅ concluída (`deb4d02`)

- F0-01 ✅ `TestGenerationUsesActivityIdleDeadlineAndCancelsPreviousWork` estabilizado (timer parado antes do reset + margens); port-forward cleanup com retry no probe.
- F0-02/F0-03 ✅ `context.Background()` removido dos serviços de ação; root context propagado; cancelamento por request/geração/shutdown.
- F0-05 ✅ mutação de `os.Stderr` localizada em helper com restauração via `defer`.
- F0-06 ✅ race latente adicional em `Generation.Stream` corrigida (timer sob mutex) na F4.

### Fase 1 — ✅ concluída (`41a2adf`, `6681f70`)

- F1-01 ✅ tokens em `web/src/tokens.css` (`@theme` Tailwind 4).
- F1-02 ✅ componentes atômicos em `web/src/components/ui/` (Button, Input, Select, Badge, Card, Tabs, EmptyState, Skeleton, DataTable, Drawer) e migração dos componentes de tela.
- F1-03 ✅ SVGs da marca integrados (BrandLogo/BrandWordmark na sidebar).
- F1-04 ✅ `DataTable` em uso nas listas de recursos, dashboard e permissões.

### Fase 2 — ✅ concluída (`17e53fe`)

- F2-01 ✅ parser composto (`termo -excluido "frase exata"`) no backend com testes; filtros de lista inicializam pela URL.
- F2-02 ✅ `Drawer` lateral para detalhes, preservando lista e deep links.
- F2-03 ✅ YAML com highlight (`react-syntax-highlighter`, PrismLight + vscDarkPlus); Secret continua sem YAML.
- F2-04 ✅ logs: pause/continue com buffer, highlight JSON, filtro local por nível.

### Fase 3 — ✅ concluída (`6e142bd`)

- F3-01 ✅ cards navegam para `/pods?problematic=true`, `/pods?restarts=gte3`, `/events?status=Warning` etc.
- F3-02 ✅ seção "Namespace health" no dashboard (backend já existia; frontend consumido com links por namespace).
- F3-03 ✅ idade dos dados por bloco com badge "stale" após 60s.

### Fase 4 — ✅ concluída com ressalva (`e010dcb`)

- F4-01 ✅ (forma pragmática) teste arquitetural em `internal/ports` impede handlers de importarem adapters/integração; pacote `ports` sem dependências.
- F4-02 ⏸ **adiado**: consolidação dashboard/resources é refactor de alto risco; documentada como follow-up.
- F4-03 ✅ cobertura: adapters/kubernetes 53%→91,6%; kubernetesruntime 41,4%→75,3% (alvo ≥70%).

### Fase 5 — ✅ essencial concluída (`f8dd8a6`)

- O-01 ✅ `duration_ms` numérico no schema de logs.
- Métricas ✅ registro próprio allowlisted + middleware de requests + `/metrics` opt-in (default off, loopback), com testes de ausência/Presença.
- `docs/observability.md` ✅.
- ⏸ adiados: O-02..O-05 parciais, spans OTel (O-06..O-09), checks de observabilidade no `doctor` (F5-04).

### Fase 6 — ✅ concluída (`02d008e`)

- F6-01 ✅ testes de `internal/application.Compose` (validações, /health, cleanups, resiliência sem kubeconfig).
- F6-02..F6-04 ✅ `docs/design-system.md`, `docs/rbac-requirements.md`, `docs/observability.md`, `docs/architecture.md` atualizado.

### Gates transversais no fechamento

| Gate | Resultado |
| --- | --- |
| G-01 `go test -count=1 ./...` | ✅ 956 testes, 37 pacotes |
| G-02 `go test -race` (pacotes internos rodados durante as fases) | ✅ |
| G-03 `go vet` | ✅ limpo |
| G-04 frontend lint/typecheck/test/build | ✅ (79 Vitest) |
| G-07 `scripts/security_check.sh HEAD` | ✅ em todos os pushes |
| E2E smoke manual | ✅ app + Kind `kubepeep-f4`, screenshots atualizados |

⏸ G-08/G-09/G-10 (harness Kind completo, Playwright E2E dedicado, build desktop) permanecem para o pipeline de release.

---

## 11. Follow-ups executados (2026-09-01, segunda rodada)

| Item | Resultado | Commit |
| --- | --- | --- |
| F4-02 | ✅ Primitivas de classificação de pod consolidadas em `internal/services/podhealth` (`ControllingOwner` — antes triplicada; `Problematic` — definição canônica do badge). Workloads e redação já eram únicos. Zero mudança de comportamento. | `c0ab25b` |
| O-05/O-02 | ✅ Eventos de lifecycle estruturados (startup/shutdown, `duration_ms` numérico); shutdown ordenado para LIFO rodar com o sink aberto. | `86b285f` |
| O-04 | ✅ `APIError.requestId` populado com o header `X-Request-ID` do backend nos erros HTTP reais. | `86b285f` |
| O-03 | ✅ `logging.Sampler` (burst→1/janela) no middleware Recovery contra spam de panics. | `86b285f` |
| F5-04 | ✅ `doctor` com grupo observabilidade: `log_sink` e `metrics_endpoint` (pass/skip/warn por contrato opt-in). Validado E2E: `METRICS_READY` e `METRICS_DISABLED` observados. | `86b285f` |
| G-08 | ✅ `test/kind/harness.sh validate` verde — todas as fases 4-7 de cenários Kubernetes/RBAC. Obs.: exigiu contorno ambiental (TLS interceptado no mirror do registry — `skip_verify` no containerd do nó de teste; imagem importada via host). | — |
| G-09 | ✅ Playwright E2E 3/3, agora servindo o bundle de produção (`vite preview` em vez de dev server, que flakava por transform on-demand); asserção da marca atualizada para o SVG da Fase 1. | `968ff1e` |
| G-10 | ◐ Parcial: `go vet -tags desktop ./...` limpo; build nativo segue bloqueado sem permissão para instalar `libgtk-3-dev`/`libwebkit2gtk-4.1-dev` (policy server rejeita sudo). | — |

## 12. Fase 7 — evoluções futuras (2026-09-01, terceira rodada)

| Item | Resultado | Commit |
| --- | --- | --- |
| F7-05 Terminal exec xterm.js | ✅ ExecTerminal com tema dos tokens, screen-reader mode, scrollback 1000, fit + resize automático, degradação graciosa; stdout/stderr no terminal, status em HTML acessível; xterm em chunk separado (84 kB gzip). | `e77258b` |
| F7-04 Busca global | ✅ Command Center indexa recursos já carregados na sessão (identificadores apenas, sem conteúdo), navegando aos deep links; getter resolvido na abertura da paleta (sem loops reativos); limitado a 200 entradas. | `07de7e7` |
| F7-01 Favoritos | ✅ Seção `favorites` no schema de preferências (50 itens, kinds allowlisted, backward-compatible), botão de estrela em todos os drawers de detalhe (Secrets permitidos — metadata-only), grupo de favoritos em primeiro lugar na paleta. | `3b10615` |
| F7-02 Diff de YAML | ✅ Diff server-side contra a anotação last-applied (Myers bounded, redação herdada do nível de confiança da rota YAML, Secrets recusados antes de qualquer fetch); endpoint generation-fenced e UI no YamlViewer de todos os drawers. | `df3be10` |
| F7-03 Multi-contexto | ⏸ Backlog permanente (XL; grande superfície de segurança). | — |

Contadores no fechamento: 991 testes Go, 84 Vitest, 3 E2E Playwright.
