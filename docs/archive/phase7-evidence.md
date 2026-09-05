# Evidências da Fase 7 — Ações autorizadas

**Data da validação local:** 2026-08-24
**Data da validação Kind:** 2026-08-30
**Plataforma principal:** Linux amd64
**Estado:** fase concluída; 47 de 47 tarefas comprovadas, incluindo a matriz dinâmica no Kind

## Resultado

Restart, scale, delete de Pod, port-forward e exec estão implementados sobre o
mesmo `AuthorizationService` da Fase 4. Capabilities da interface são apenas
informativas: cada mutação ou upgrade revalida o SAR exato no backend. Tickets,
sessões, listeners e streams têm owner, geração, limites e cleanup.

O fechamento dinâmico confirmou restart, scale, delete de Pod, port-forward e
exec nos caminhos permitido, negado e revogado contra uma API Kubernetes real.

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
wire passaram. O frontend passou audit/lint/typecheck/build, 73 Vitest e três
Playwright. Ginger v1.4.4 `inspect`/`doctor` também passou.

## Evidência Kind canônica

O fechamento local executou:

```bash
rtk ./test/kind/harness.sh create
rtk ./test/kind/harness.sh validate
rtk ./test/kind/harness.sh kubeconfigs
rtk ./test/kind/harness.sh app-e2e ./dist/kubePeep
```

No perfil permitido, o black-box comprovou restart, scale, delete de Pod,
port-forward e exec reais. No perfil sem grants, restart, scale, delete e
port-forward receberam `503/AUTHORIZATION_UNAVAILABLE`, pois o SSAR não emitiu
decisão; exec recebeu `403/FORBIDDEN` quando a negação da operação alvo pela API
Kubernetes foi autoritativa.

Na revogação periódica, uma nova ação de exec e a reautorização de um ticket
WebSocket existente falharam fechadas com `503/AUTHORIZATION_UNAVAILABLE`. A
leitura direta do Pod pela identidade revogada confirmou, separadamente, a
negação autoritativa `Forbidden` da API Kubernetes. O cleanup restaurou os
bindings, recriou o Pod excluído, restaurou escala e anotação de restart e
removeu sessões, listeners e grant temporário.

As fixtures compartilhadas também recriaram de forma idempotente, sequencial e
com precondição de UID o Pod de previous-log e o Event inicial
`000-kp-warning`. A prova negativa de delete usa a UID do Pod gerenciado, exige
`Forbidden` autoritativo e confirma que a identidade permaneceu igual. A
recuperação fica armada antes de qualquer remoção, converge respostas ambíguas
e não incorpora conteúdo de nenhum recurso à evidência.

A inspeção negativa não encontrou comando, saída, ticket, conteúdo de log,
kubeconfig, token ou payload operacional persistido. O contrato final é:
operações falham fechadas; SSAR sem decisão resulta em
`503/AUTHORIZATION_UNAVAILABLE`; somente uma negação direta e autoritativa
resulta em `403/FORBIDDEN`.

Esta é evidência local do Kind; nenhum run adicional de CI é atribuído ao
fechamento.
