# Revisão de Arquitetura do KubePeep

## 1. Visão geral

O KubePeep é uma aplicação Go 1.26 com frontend React, organizada em camadas:

```text
cmd/kubePeep              → entrypoint
internal/cli              → Cobra, comandos
internal/runtime          → lifecycle local (lock, listener, estado, controle)
internal/app              → aplicação HTTP Ginger, roteamento, embed
internal/application      → composição do core (sem dependência de Wails)
internal/api              → handlers, middlewares, DTOs, sessão, cursor
internal/services         → domínio: autorização, seleção, dashboard, recursos, ações
internal/adapters         → Kubernetes, SQLite, browser, userdirs
internal/integration      → client-go/SPDY concretos
internal/desktop          → bridge Wails e loopback interno
internal/migrations       → schema SQLite embutido
internal/web              → assets Vite embutidos
web/src                   → React/TypeScript
```

## 2. Pontos fortes

- **Segurança:** Host/Origin/CSRF, tokens de controle, loopback, Secret metadata-only, redaction.
- **Cancelamento hierárquico:** geração monotônica propaga cancelamento.
- **Separação web/desktop:** `internal/application` e `internal/desktop` permitem builds com e sem tag `desktop`.
- **SQLite seguro:** WAL, foreign keys, busy timeout, migrations com checksum.
- **CLI bem modelada:** exit codes, injeção de dependências, doctor.

## 3. Problemas arquiteturais

### 3.1 Ports não centralizados

**Evidência:**
- `internal/ports/` contém apenas `doc.go`.
- Interfaces de port estão dispersas em handlers (`handlers.ResourceService`, `handlers.DashboardService`) e serviços (`resources.SecretMetadataPort`, `dashboard.PodPort`).

**Impacto:**
- Desconformidade com `docs/architecture.md` §4–5.
- Acoplamento implícito entre handlers e implementações.
- Dificuldade de navegação e auditoria.

**Prioridade:** P1. **Complexidade:** L.

**Recomendação:**
- Criar interfaces em `internal/ports/` conforme a tabela de `docs/architecture.md` §5.
- Fazer handlers dependerem das interfaces de port.
- Manter serviços implementando os ports.

**Critério de aceite:**
- Nenhum handler conhece implementação concreta de serviço.
- `internal/ports/` contém todas as interfaces documentadas.

---

### 3.2 Duplicação dashboard/resources

**Evidência:**
- `internal/services/dashboard/` e `internal/services/resources/` possuem classificadores e listagens sobrepostos.
- `DashboardPodService` e `PodService`/`ResourceService` fazem trabalho similar.
- `dashboard.PodPort` vs `resources.LogPort`.

**Impacto:**
- Manutenção duplicada.
- Risco de inconsistência entre dashboard e listas.

**Prioridade:** P1. **Complexidade:** L.

**Recomendação:**
- Refatorar `DashboardService` para coordenar ports de recursos já existentes.
- Reutilizar classificadores de pods/workloads/logs entre as camadas.
- Mover lógica de ranking/ordenação para pacotes compartilhados.

**Critério de aceite:**
- Dashboard e listas usam os mesmos classificadores.
- Não há duplicação de conversão client-go → DTO.

---

### 3.3 Testes flaky e race conditions

**Evidência:**
- `TestCheckerSnapshotProviderSanitizesFailuresAndAppliesDeadline` (`internal/api`).
- `TestPortForwardTerminalPathsReleaseLoopbackPortAndAdapterGoroutine/absolute_expiry` (`internal/services/actions`).
- `TestGenerationUsesActivityIdleDeadlineAndCancelsPreviousWork` (`internal/adapters/kubernetes`).
- `TestConcurrentRefreshesShareOneLiveReview` (`internal/services/authorization`) — falha consistentemente com `-race`.

**Impacto:**
- Races na implementação podem causar comportamento não determinístico em produção.
- CI instável reduz confiança.

**Prioridade:** P0. **Complexidade:** L.

**Recomendação:**
- Corrigir deduplicação de `Refresh` no cache de autorização.
- Substituir `time.Sleep` em testes por canais/WaitGroups.
- Executar CI com `-race` e múltiplas repetições.

**Critério de aceite:**
- `go test -race ./...` passa de forma determinística.
- Testes afetados passam 10 vezes seguidas.

---

### 3.4 Uso de `context.Background()` em ações

**Evidência:**
- `internal/services/actions/service.go` (Restart, Scale, DeletePod).
- `internal/services/actions/exec.go` (CreateTicket, AuthorizeUpgrade, Start).
- `internal/services/actions/portforward.go` (Create, Close).

**Impacto:**
- Desconecta operações do lifecycle do request/processamento.
- Em shutdown ou troca de geração, operações podem continuar sem cancelamento.

**Prioridade:** P0. **Complexidade:** M.

**Recomendação:**
- Retornar erro quando `ctx == nil` em vez de criar `context.Background()`.
- Garantir que todo request mutável passe contexto cancelável.

**Critério de aceite:**
- Nenhum `context.Background()` em caminhos de produção.
- Testes de cancelamento de ações passam.

---

### 3.5 `os.Stderr` global mutável

**Evidência:**
- `internal/adapters/kubernetes/client_factory.go:211` altera `os.Stderr` global durante construção do client-go.

**Impacto:**
- Sob concorrência, stderr de outras goroutines pode ser redirecionado.
- Risco de perder logs importantes.

**Prioridade:** P1. **Complexidade:** M.

**Recomendação:**
- Usar mecanismo thread-safe para suprimir stderr do plugin exec.
- Alternativa: isolar construção em subprocesso ou usar hooks de log do client-go quando disponíveis.

**Critério de aceite:**
- Teste de concorrência de criação de clients não falha.
- `os.Stderr` não é mais mutado globalmente.

---

### 3.6 `internal/application.Compose` sem testes

**Evidência:**
- `internal/application/application.go` tem 315 linhas de wiring e zero testes.

**Impacto:**
- Erros de composição só aparecem em runtime ou testes de integração.

**Prioridade:** P1. **Complexidade:** M.

**Recomendação:**
- Adicionar testes de composição com dependências mockadas.
- Testar shutdown e cleanup.

**Critério de aceite:**
- `internal/application/application_test.go` existe e passa.
- Testes cobrem wiring, shutdown e cleanup.

---

### 3.7 Lógica de negócio em handlers

**Evidência:**
- `internal/api/handlers/dashboard.go` tem 867 linhas com filtros, ordenação, paginação, cursores.
- `internal/api/handlers/namespace_scopes.go` também acumula lógica.

**Impacto:**
- Handlers inchados, violação da separação de responsabilidades.
- Dificuldade de testar regras de negócio.

**Prioridade:** P2. **Complexidade:** L.

**Recomendação:**
- Mover lógica de filtro/paginação/ordenação para serviços.
- Handlers devem apenas validar input, chamar serviço e formatar output.

---

### 3.8 Cobertura baixa em adapters e integração

**Evidência:**
- `internal/adapters/kubernetes`: 41–57%.
- `internal/integration/kubernetesruntime`: ~41%.
- `internal/securefs`: ~57%.

**Impacto:**
- Bugs de integração só detectados em E2E/manual.

**Prioridade:** P1. **Complexidade:** L.

**Recomendação:**
- Aumentar testes unitários com mocks/fakes.
- Adicionar testes de integração com kube-apiserver mock ou Kind.
- Focar em `client_cache.go`, `generation.go`, `runtime.go`, `resources_backend.go`, `dashboard_backend.go`.

---

## 4. Conformidade com documentação

| Documento | Conformidade | Observações |
| --- | --- | --- |
| `docs/architecture.md` | Parcial | Ports não centralizados; duplicação dashboard/resources. |
| `docs/api.md` | Alta | Envelopes, códigos, cursores, CSRF, sessão refletidos. |
| `docs/security.md` | Alta | Host/Origin/CSRF, tokens, redaction, Secret metadata-only. |
| `docs/data-model.md` | Alta | SQLite schema, preferências, migrations. |

## 5. Recomendações por prioridade

### P0
1. Corrigir testes flaky/race.
2. Remover `context.Background()` de ações.

### P1
3. Centralizar ports em `internal/ports/`.
4. Consolidar dashboard/resources.
5. Remover mutação global de `os.Stderr`.
6. Adicionar testes para `internal/application.Compose`.
7. Aumentar cobertura em adapters e integração.

### P2
8. Mover lógica de negócio de handlers para serviços.
9. Revisar acoplamento `Resources` handler ↔ `PreferenceService`.
10. Tipar `details` de erros mais fortemente.

## 6. Estratégia de migração

1. **Sem quebrar contratos:** manter rotas e DTOs estáveis.
2. **Refatoração interna:** mover interfaces e reutilizar classificadores sem alterar API.
3. **Testes como guarda:** adicionar testes antes das refatorações.
4. **Faseado:** uma camada por vez (ports → serviços → handlers).
