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
