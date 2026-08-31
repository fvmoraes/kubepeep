# Estado Atual do KubePeep

> Captura do estado do projeto em 2026-08-31, após a revisão completa.

## 1. Identificação da revisão

| Campo | Valor |
| --- | --- |
| Branch | `main` |
| HEAD | `c7e291d` — `feat: desktop support and security hardening` |
| Revisão executada em | 2026-08-31 |
| Revisor | agente automatizado + análise humana orientada |
| Cluster de teste | Kind `kubepeep-f4` criado via `test/kind/harness.sh create` |
| Identidade de teste | `dashboard-viewer` (escopo `kp-allowed`) |

## 2. Validações executadas

### 2.1 Backend

| Validação | Comando | Resultado |
| --- | --- | --- |
| Testes Go | `go test -count=1 ./...` | **729 passaram, 1 falhou, 1 pulado em 36 pacotes** |
| Testes Go com race | `go test -race -count=3 ./internal/adapters/kubernetes/...` | **84 passaram** (pacote isolado) |
| Vet | `go vet ./...` | **limpo** |
| Build web | `go build -trimpath -o dist/kubePeep ./cmd/kubePeep` | **sucesso** |
| Vulnerabilidades | `govulncheck ./...` | **nenhuma vulnerabilidade alcançável** |

**Falha registrada:** `TestGenerationUsesActivityIdleDeadlineAndCancelsPreviousWork` (`internal/adapters/kubernetes/client_factory_test.go:88`) falhou intermitentemente com a mensagem `active stream was canceled by unary timeout`. O mesmo pacote passou com `-race -count=3`, confirmando natureza flaky. Esta falha foi elevada como **P0** no roadmap (`F0-01`).

### 2.2 Frontend

| Validação | Comando | Resultado |
| --- | --- | --- |
| Lint | `npm --prefix web run lint` | **limpo** |
| Type check | `npm --prefix web run typecheck` | **limpo** |
| Testes unitários | `npm --prefix web run test` | **79/79 passaram** |
| Build | `npm --prefix web run build` | **sucesso (419,74 kB JS gzip 121,10 kB)** |
| Audit | `npm --prefix web audit --audit-level=high` | **0 vulnerabilidades** |

### 2.3 Execução e navegação

| Validação | Resultado |
| --- | --- |
| Inicialização com kubeconfig real | `kubePeep serve --no-browser --port 2751` saudável |
| `/health` | `status: healthy`, todos os componentes OK |
| `/api/v1/session` | CSRF token e geração retornados |
| Navegação pelas 10 rotas | Screenshots capturadas em `docs/research/screenshots-review/` |
| Kind com fixtures | Cluster criado, fixtures aplicadas, validações básicas OK |

### 2.4 Segurança

| Validação | Comando | Resultado |
| --- | --- | --- |
| Security gate | `scripts/security_check.sh HEAD` | **passou** (após ajuste de allowlist para exemplos de secrets em `.agents/skills/golang-security/references/secrets.md`) |

## 3. Cobertura de testes observada

| Pacote | Cobertura aproximada | Observação |
| --- | --- | --- |
| `internal/api` | alta | testes de envelope, cursor, sessão |
| `internal/services/*` | média a alta | boa cobertura de classificadores |
| `internal/adapters/kubernetes` | 41–57% | baixa; interage diretamente com client-go |
| `internal/adapters/sqlite` | média | WAL, migrations, preferências |
| `internal/integration/kubernetesruntime` | ~41% | complexo, pouco testado unitariamente |
| `internal/application` | **0%** | sem testes de composição |
| `internal/ports` | **0%** | pacote vazio, apenas `doc.go` |
| `web/src/components` | alta via Vitest | 79 testes cobrindo principais fluxos |

## 4. Estado do plano original

| Fase | Estado | Gate restante |
| --- | --- | --- |
| Fase 1 — Descoberta | 44/44 concluída | nenhum |
| Fase 2 — Especificação | 61/61 concluída | nenhum |
| Fase 3 — Fundação | 54/54 concluída | manutenção contínua da CI |
| Fase 4 — Kubernetes e RBAC | 58/59 | **F4-49** (matriz exaustiva) |
| Fase 5 — Dashboard | 62/62 concluída | nenhum |
| Fase 6 — Recursos | 67/67 concluída | nenhum |
| Fase 7 — Ações | 47/47 concluída | nenhum |
| Fase 8 — Distribuição | 47/50 | **F8-42**, **F8-46**, **F8-48** (candidate real) |
| Fase 9 — Experiência operacional | 15/84 | 69 tarefas e matriz UX |

## 5. Funcionalidades validadas com Kind

Durante a revisão, o cluster Kind foi criado com fixtures representativas. As seguintes funcionalidades puderam ser exercitadas com a identidade `dashboard-viewer` no namespace `kp-allowed`:

- Dashboard com resumo, problemas, restarts, eventos e métricas ausentes.
- Listas de Workloads, Pods, Events, Network e Config.
- Detalhes de recursos e YAML de recursos elegíveis.
- Logs de Pod e follow via SSE.
- Matriz de permissões.
- Settings e preferências.

Ações mutáveis (restart, scale, delete, port-forward, exec) não foram exercitadas destrutivamente durante a revisão; seus caminhos felizes estão cobertos pelos testes unitários e pelo harness Kind.

## 6. Limitações conhecidas da revisão

- **Desktop Wails:** não foi compilado/executado localmente porque as dependências nativas do Linux (`libgtk-3-dev`, `libwebkit2gtk-4.0-dev`) não estão instaladas neste ambiente. A arquitetura foi revisada via código e documentação.
- **Multi-contexto:** a agregação multi-contexto ainda não está implementada; a análise é baseada em contratos e código parcial.
- **Ações destrutivas:** não foram executadas em cluster real durante a revisão para evitar efeitos colaterais.
- **Performance em grande escala:** não foi possível testar com centenas de namespaces ou milhares de pods; a análise é baseada em contratos e budgets documentados.

## 7. Evidências geradas

- Screenshots das 10 rotas principais: `docs/research/screenshots-review/01-overview.png` a `10-settings.png`.
- Logs de execução local do app e do harness Kind (não persistidos por conter dados do ambiente).
- Resultados dos comandos de validação listados na §2.

## 8. Síntese do estado

O KubePeep é um produto funcional e maduro para o escopo do MVP. A base técnica, a segurança e a cobertura de testes são pontos fortes. Os principais débitos estão na **organização do frontend**, na **centralização arquitetural dos ports**, na **consolidação dashboard/resources** e na **conclusão da Fase 9** de experiência operacional. Nenhum problema crítico de segurança foi identificado, mas testes flaky e races precisam de atenção imediata.
