# Evidências da Fase 7 — Ações autorizadas

**Data da validação local:** 2026-08-24
**Plataforma principal:** Linux amd64
**Estado:** implementação local concluída; 46 de 47 tarefas comprovadas; matriz dinâmica permitida/negada no Kind pendente

## Resultado

Restart, scale, delete de Pod, port-forward e exec estão implementados sobre o
mesmo `AuthorizationService` da Fase 4. Capabilities da interface são apenas
informativas: cada mutação ou upgrade revalida o SAR exato no backend. Tickets,
sessões, listeners e streams têm owner, geração, limites e cleanup.

## Rastreabilidade da implementação

| Área | Implementação | Evidência automatizada |
| --- | --- | --- |
| Fundação, confirmação e idempotência | `internal/services/actions/service.go`, `validation.go`, `idempotency.go`, `audit.go` | `service_test.go` cobre SAR, validação antes de efeitos, bind da chave a body/path/profile/generation, concorrência, cancelamento e auditoria allowlisted |
| Restart | service + `internal/adapters/kubernetes/actions.go` | patch contém somente `restartedAt` e resourceVersion; testes cobrem replay sem segundo patch, conflito e erros públicos |
| Scale | mesmo adapter via `UpdateScale` | testes cobrem Deployment/StatefulSet, subresource/verbo exato, faixa, conflito e cancelamento |
| Delete Pod | service/adapter com UID e resourceVersion | testes cobrem owner/consequência, preconditions, inexistente, conflito e negação |
| Port-forward | `portforward.go` e adapter SPDY | testes cobrem bind loopback `:0`, porta ocupada, limite oito, idempotência, geração, Pod gone, 8 h, retenção e liberação real da porta/goroutine |
| Exec | `exec.go`, `actions_exec.go`, adapter SPDY e `coder/websocket` v1.8.15 | tickets one-shot de 10 s, duas sessões, argv sem shell, resize/heartbeat, idle/duração, terminal e close mappings |
| Wire WebSocket | handler raw com subprotocolos fixos e compressão desabilitada | `actions_exec_wire_test.go` cobre fragmentação mascarada, frame sem mask, ofertas adversariais, heartbeat, canais TTY/non-TTY e todos os mapeamentos terminal → close |
| UI | `web/src/components/ResourceActions.tsx` e área Network | capabilities tri-state, confirmação contextual, restart/scale/delete/port-forward e terminal exec; testes verificam fail-closed, ticket efêmero e cleanup por geração |
| Não persistência | sink de auditoria e composição runtime | `actions_nonpersistence_test.go` inspeciona logs e artefatos SQLite; comando, saída, ticket e payload sintéticos não aparecem |

## Invariantes comprovados

- restart usa patch mínimo; scale usa `UpdateScale`; delete carrega as duas
  preconditions;
- port-forward escuta apenas `127.0.0.1` e limita oito sessões;
- exec preserva argv, nunca concatena shell e nunca registra comando/saída;
- WebSocket exige `kubepeep.exec.v1` e ticket `kp-ticket.*`, seleciona somente
  o protocolo público e consome o ticket uma vez;
- troca de geração, cancelamento, timeout, término remoto e shutdown liberam
  recursos com razões terminais estáveis;
- a UI mantém `denied` e `unknown` desabilitados mesmo após confirmação.

## Gates locais executados

Go 1.25.13 `test`, race, vet, build e `govulncheck` passaram. Os testes
black-box locais de lifecycle de port-forward/exec e os testes adversariais do
wire passaram. O frontend passou audit/lint/typecheck/build, 63 Vitest e três
Playwright. Ginger v1.4.4 `inspect`/`doctor` também passou.

## Pendências exatas

- **F7-44:** executar cada subresource real no caminho permitido, negado e
  revogado: `./test/kind/harness.sh validate`; para os quatro caminhos HTTP do
  produto, completar também
  `./test/kind/harness.sh app-e2e ./kubePeep`.

F7-45 e F7-46 estão fechadas por testes locais de confirmação, cleanup e
inspeção de persistência. A API Kubernetes real continua sendo o único gate
aberto; não há run atual de CI a citar.
