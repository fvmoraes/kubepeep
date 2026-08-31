# Estratégia de Testes do KubePeep

## 1. Princípios

- Testes devem acompanhar cada mudança, nunca serem adicionados como afterthought.
- Priorizar testes determinísticos; eliminar flakiness.
- Usar mocks/fakes para lógica pura; usar Kind para caminhos reais de Kubernetes.
- Manter pirâmide de testes: muitos unitários, integração moderada, E2E seletivo.
- Nunca usar credenciais reais em fixtures.

## 2. Níveis de teste

### 2.1 Unitários Go

**Escopo:**
- Parser/deduplicação/validação de namespaces.
- Classificadores de pods/workloads/events/logs.
- Redaction e detecção de padrões.
- Cache RBAC e cursor.
- DTO converters.
- Preferências allowlisted.
- Idempotência e preconditions.

**Ferramentas:**
- `go test` + `testify`.
- `goleak` para detecção de goroutines em testes de longa duração.
- Clock e filesystem injetáveis.

**Metas:**
- Cobertura ≥ 80% em `internal/services/*`.
- Cobertura ≥ 70% em `internal/adapters/*` e `internal/integration/*`.
- Zero `time.Sleep` para sincronização.

### 2.2 Integração Go

**Escopo:**
- Handlers com `httptest`.
- SQLite temporário real (WAL/journal/backup).
- Servidor HTTP Kubernetes controlado para erros, paginação, auth.
- `internal/application.Compose` com mocks.

**Ferramentas:**
- `httptest` + `pkg/testhelper`.
- Client-go fake para comportamento simples.
- kube-apiserver mock para cenários complexos.

**Metas:**
- Toda rota mutável testada com caminho permitido e negado.
- Toda rota de stream testada com cancelamento e geração antiga.

### 2.3 Testes de frontend

**Escopo:**
- Componentes atômicos (`Button`, `Input`, `Table`, etc.).
- Estados loading/vazio/offline/proibido/parcial/cancelado.
- Formulários de scope e preferências.
- Capabilities ocultas/desabilitadas.
- Filtros e navegação preservada.
- Cancelamento de query/stream.
- Nenhuma persistência browser de dados remotos.

**Ferramentas:**
- Vitest + Testing Library + jsdom.
- MSW para mock de API quando necessário.
- axe-core para acessibilidade.

**Metas:**
- Cobertura ≥ 80% em `web/src/components/ui/`.
- Todos os estados visuais testados.

### 2.4 E2E

**Escopo:**
- Jornadas no binário embutido (não dev server).
- Deep links da History API.
- Cluster Kind com identidades restritas.
- Ações mutáveis permitidas e negadas.
- Inspeção de DB/logs após execução.

**Ferramentas:**
- Playwright.
- `test/kind/harness.sh`.

**Metas:**
- Cobrir todas as rotas principais.
- Validar ausência de dados sensíveis em storage/browser/SQLite/logs.

## 3. Testes específicos por melhoria

### Fase 0 — Correções críticas

| Melhoria | Teste |
| --- | --- |
| Corrigir races | `go test -race ./...` 10x; testes afetados 100x. |
| Remover `context.Background()` | Testes de cancelamento de restart/scale/delete/exec/port-forward. |
| Eliminar mutação de `os.Stderr` | Teste de concorrência de criação de 10 clients simultâneos. |

### Fase 1 — Consistência visual

| Melhoria | Teste |
| --- | --- |
| Tokens | Playwright screenshots comparativos (baseline vs. novo). |
| Componentes atômicos | Vitest para cada componente (estados, acessibilidade). |
| SVGs da marca | Vitest verifica presença e proporção. |
| DataTable | Vitest com colunas, ordenação, paginação. |

### Fase 2 — Navegação e experiência

| Melhoria | Teste |
| --- | --- |
| Parser composto | Unitário no parser; E2E com buscas complexas. |
| Painel lateral | Playwright: clique em linha abre drawer; back funciona; filtros preservados. |
| YAML highlight | Playwright screenshot; Vitest verifica presença de highlight. |
| Logs melhorados | Vitest para pause/continue/filtro; Playwright para SSE. |

### Fase 3 — Dashboard

| Melhoria | Teste |
| --- | --- |
| Cards clicáveis | Playwright: clique navega com filtros. |
| Saúde por namespace | Integração Kind; Vitest para renderização. |
| Indicador stale | Vitest com timers falsos. |

### Fase 4 — Arquitetura

| Melhoria | Teste |
| --- | --- |
| Centralizar ports | Teste de arquitetura: handlers não importam implementações concretas. |
| Consolidar dashboard/resources | Testes de dashboard não quebram; comparação de classificadores. |
| Cobertura adapters | Novos testes unitários/integração. |

### Fase 5 — Observabilidade

| Melhoria | Teste |
| --- | --- |
| OTel | Teste de configuração; verificar que sem configuração não há tráfego. |
| Métricas internas | Testes de exposição/escopo. |

### Fase 6 — Documentação

| Melhoria | Teste |
| --- | --- |
| `application.Compose` | Testes de wiring/shutdown/cleanup. |
| Documentação | Validação de links Markdown; consistência de nomenclatura. |

## 4. CI e gates

### Pipeline de verificação

```text
format-check
lint
typecheck
go vet
go test ./...
go test -race ./internal/...
npm audit --audit-level=high
govulncheck ./...
web build
Playwright E2E (contra binário embutido)
Kind harness validate
security_check.sh HEAD
```

### Regras

- Nenhum merge com testes flaky.
- Nenhum merge com `-race` quebrado.
- Nenhum merge sem cobertura mínima nos pacotes alterados.
- Security gate deve passar antes do push.

## 5. Ferramentas recomendadas

| Ferramenta | Uso |
| --- | --- |
| `go test` / `testify` | Unitários Go. |
| `httptest` | Integração HTTP. |
| `goleak` | Detecção de goroutines. |
| `kind` / `kubectl` | E2E com cluster real. |
| `vitest` / `@testing-library/react` | Testes de frontend. |
| `playwright` | E2E de interface. |
| `axe-core` | Acessibilidade. |
| `govulncheck` | Vulnerabilidades Go. |
| `npm audit` | Vulnerabilidades npm. |
| `scripts/security_check.sh` | Segurança de conteúdo. |

## 6. Critérios de aceite transversais

- [ ] Toda mudança possui testes unitários.
- [ ] Toda mudança funcional possui teste de integração do caminho feliz.
- [ ] Toda mudança funcional possui teste do caminho negado por RBAC quando aplicável.
- [ ] Testes são determinísticos.
- [ ] `-race` passa.
- [ ] E2E passa no binário embutido.
- [ ] Security gate passa.
