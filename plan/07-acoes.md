# Fase 7 — Ações autorizadas

**Estado atual:** implementação local concluída (46/47); matriz dinâmica no Kind pendente

**Evidência:** [relatório rastreável da Fase 7](../docs/research/phase7-evidence.md)

**Dependências:** autorização da Fase 4 e detalhes de recursos da Fase 6
**Regra de execução:** concluir uma ação verticalmente, incluindo testes negativos e cancelamento, antes de iniciar a próxima.

## Objetivo

Adicionar restart, scale, exclusão de pod, port-forward e exec sem criar uma autorização paralela ao Kubernetes. A interface antecipa capabilities, mas o backend revalida cada operação imediatamente antes de executá-la.

## Entregáveis

- framework comum de confirmação, capability e revalidação;
- restart de Deployment;
- scale de Deployment e StatefulSet;
- delete de Pod;
- port-forward de Pod com lifecycle local;
- exec de container por WebSocket;
- testes autorizados, negados, cancelados e concorrentes para cada ação.

## Fundação comum

- [x] **F7-01** Mapear para cada ação grupo de API, recurso, subresource e verbo exatos.
- [x] **F7-02** Reutilizar e estender o `AuthorizationService`/helper da Fase 4 para revalidar SAR, sem criar uma autorização paralela nem confiar no resultado antigo da interface.
- [x] **F7-03** Reutilizar os guards/decoder instalados nas Fases 3–4: Host/Origin, CSRF, `Content-Type: application/json`, JSON estrito e body limit em toda rota mutável; CSRF via `fetch` em SSE; no upgrade WebSocket, Host/Origin, ticket one-shot, geração e SAR (o browser não envia header CSRF no construtor WebSocket).
- [x] **F7-04** Implementar o port `ActionService` e o DTO de confirmação comum com contexto, namespace, kind, nome, ação e consequência.
- [x] **F7-05** Exigir `ActionTargetDTO`, confirmação/consequência explícitas e impedir replay; no registry idempotente, ligar a chave a método, path/alvo, profile, geração e hash canônico do body por 10 minutos.
- [x] **F7-06** Implementar os ports `PortForwardService` e `ExecService` sobre registros em memória com owner, generation ID, limites de concorrência, cancelamento e cleanup.
- [x] **F7-07** Registrar somente metadados operacionais: nunca conteúdo de terminal, comando, log, token ou stream.
- [x] **F7-08** Padronizar forbidden, conflict, timeout, cancelamento, alvo inexistente e cluster offline.
- [x] **F7-09** Atualizar capabilities após sucesso, negação ou mudança de contexto.

## Ação 1 — Restart de Deployment

- [x] **F7-10** Confirmar permissão necessária para o patch de Deployment.
- [x] **F7-11** Implementar restart de Deployment pelo strategic merge patch mínimo definido em `docs/api.md`, alterando apenas `kubectl.kubernetes.io/restartedAt`, preservando campos não relacionados e aplicando a precondition de resourceVersion.
- [x] **F7-12** Implementar endpoint `POST /api/v1/workloads/{kind}/{namespace}/{name}/restart` restrito ao kind suportado no MVP, exigindo `Idempotency-Key` ligada à identidade canônica completa e TTL terminal de 10 minutos.
- [x] **F7-13** Exibir confirmação, progresso e resultado sem afirmar que o rollout terminou antes da condição real.
- [x] **F7-14** Testar permitido, negado, conflito de versão, alvo removido, request cancelado, duplicata concorrente sem segundo patch e `IDEMPOTENCY_CONFLICT` ao mudar body, path/alvo, profile ou geração com a mesma chave.

## Ação 2 — Scale

- [x] **F7-15** Usar e testar SAR `update apps/deployments/scale` ou `update apps/statefulsets/scale`, com `resourceName`; não usar `patch` no MVP.
- [x] **F7-16** Validar réplica mínima/máxima e representar a consequência de escalar para zero.
- [x] **F7-17** Implementar scale para Deployment e StatefulSet via `UpdateScale`, sem substituir o workload completo.
- [x] **F7-18** Implementar `PUT /api/v1/workloads/{kind}/{namespace}/{name}/scale`.
- [x] **F7-19** Testar permitido, negado, valor inválido, conflito, cancelamento e atualização da interface.

## Ação 3 — Delete Pod

- [x] **F7-20** Verificar permissão `delete pods` no namespace do alvo.
- [x] **F7-21** Exibir owner e explicar que a recriação depende do controlador; não prometer recriação para pod sem owner.
- [x] **F7-22** Implementar `DELETE /api/v1/pods/{namespace}/{name}` com preconditions definidas no contrato.
- [x] **F7-23** Exigir confirmação destrutiva e invalidar listas relacionadas após sucesso.
- [x] **F7-24** Testar pod controlado, pod independente, negado, alvo já removido e conflito.

## Ação 4 — Port-forward

- [x] **F7-25** Usar e testar o atributo SAR exato `create pods/portforward`; não derivar o verbo do método HTTP local.
- [x] **F7-26** Validar pod, `remotePort` 1–65535 e `localPort` null ou 1024–65535, sem exigir container nem `containerPort` declarado.
- [x] **F7-27** Escutar somente em loopback; quando a porta local for omitida, adquirir diretamente `127.0.0.1:0` e manter o listener, sem probe separado.
- [x] **F7-28** Implementar `POST /api/v1/pods/{namespace}/{name}/port-forward`, exigindo `Idempotency-Key` ligada a método + path/Pod + profile + geração + hash do body por 10 minutos, além de `GET /api/v1/port-forwards` e `DELETE /api/v1/port-forwards/{id}`.
- [x] **F7-29** Exibir sessões ativas e terminais retidas por 10 min na área Network, com contexto, namespace, pod, portas, `createdAt`, `expiresAt`, `endedAt`, status e razão, sem tráfego.
- [x] **F7-30** Encerrar sessão em ação do usuário, qualquer troca de geração (contexto ou scope), evidência upstream de término do pod, duração absoluta de 8 h ou shutdown.
- [x] **F7-31** Limitar exatamente oito sessões port-forward ativas e evitar goroutines/sockets órfãos.
- [x] **F7-32** Testar permitido, negado, limites/faixas, bind `:0`, porta ocupada, todos os códigos públicos, expiração de 8 h, Pod gone, retenção terminal de 10 min, conexão interrompida, cancelamento, cleanup, duplicata concorrente sem segundo listener/sessão e `IDEMPOTENCY_CONFLICT` ao mudar body, Pod/path, profile ou geração com a mesma chave.

## Ação 5 — Exec

- [x] **F7-33** Usar `create pods/exec` no POST e revalidar exatamente o mesmo atributo SAR imediatamente antes do upgrade GET; o verbo Kubernetes não é derivado do método HTTP local.
- [x] **F7-34** Validar namespace, pod, container e estado executável do alvo.
- [x] **F7-35** Usar `github.com/coder/websocket v1.8.15` entre browser e backend, manter as bibliotecas oficiais Kubernetes no stream remoto e não usar `pkg/ws` do Ginger no caminho de `exec`; aplicar Origin, masking, opcodes, fragmentação, ping/pong, limites e deadlines do ADR 0003.
- [x] **F7-36** Implementar os schemas wire, encoding, stdin, stdout, stderr, TTY, resize, heartbeat e encerramento fixados em `docs/api.md`.
- [x] **F7-37** Validar command/argv como lista, duas sessões, mensagens de dados/controle, fila, heartbeat, idle de 30 min, duração de 4 h e cancelamento, sem concatenação de shell.
- [x] **F7-38** Implementar `POST /api/v1/pods/{namespace}/{name}/exec`, que cria ticket one-shot de 10 s, e a rota interna `GET /api/v1/exec/{sessionId}/stream`; exigir os subprotocolos `kubepeep.exec.v1` + `kp-ticket.*`, consumir uma vez, selecionar na resposta somente `kubepeep.exec.v1`, desabilitar compressão e nunca registrar ticket/comando/saída.
- [x] **F7-39** Fechar a sessão em troca de contexto/escopo, perda do socket, término do container ou shutdown.
- [x] **F7-40** Criar interface de terminal somente após revisão de segurança e justificativa de qualquer dependência adicional.
- [x] **F7-41** Testar permitido, negado, container inválido, ticket válido/vencido/reusado/outra geração, lista/seleção de subprotocolos, compressão desabilitada, `ready`/`exit`, stdout/stderr com e sem TTY, resize, fragmentação, masking, heartbeat iniciado pelo backend/ecoado pelo browser e cada mapeamento causa → terminal → close code, além de desconexão abrupta, backpressure e cleanup.
- [x] **F7-42** Testar argv adversarial, ausência de shell implícito e ausência de comando/saída nos logs.

### E2E e fechamento

- [x] **F7-43** Estender o harness restrito com identidades separadas para restart, scale, delete, port-forward e exec.
- [ ] **F7-44** Executar cada ação no caminho permitido e provar 403 no caminho negado, inclusive após revogação de RBAC.
- [x] **F7-45** Verificar que confirmações exibem o alvo correto e que nenhuma sessão, porta ou goroutine permanece após cancelamento.
- [x] **F7-46** Inspecionar logs e SQLite após exec/port-forward para provar ausência de comando, saída e payload.
- [x] **F7-47** Executar e registrar `ginger inspect`, `ginger doctor`, testes, lint e build no fechamento da fase.

## Gate de cada ação

Antes de iniciar a ação seguinte, a atual precisa comprovar:

- capability correta na interface;
- ação ausente/desabilitada para usuário negado;
- SAR revalidado no backend;
- confirmação contextual quando aplicável;
- idempotência/replay conforme o contrato da ação;
- preconditions e generation ID validados quando o contrato exigir;
- Content-Type, JSON estrito, campos desconhecidos, trailing data e body limit testados;
- resultado e erro padronizados;
- timeout e cancelamento;
- cleanup sem sessão/goroutine órfã;
- teste unitário, integração e cenário negativo;
- documentação da API e segurança atualizada.

## Riscos específicos

| Risco | Mitigação |
| --- | --- |
| Cache RBAC ficar desatualizado | SAR imediatamente antes da operação |
| Restart sobrescrever spec | Patch mínimo com precondition e teste de preservação |
| Scale perder atualização concorrente | Subresource e tratamento de conflito |
| Delete induzir falsa promessa | Mostrar owner e consequência real |
| Port-forward expor porta na rede | Bind somente em loopback |
| Transporte WebSocket aceitar frames/Origins inseguros | `coder/websocket` configurado pelo contrato e gate adversarial; `pkg/ws` não entra no caminho de `exec` |
| Exec vazar comando ou saída | Nenhum log de payload e testes de observabilidade |
| Sessões ficarem órfãs | Registry, contextos e cleanup no shutdown/troca |

## Fora de escopo

- Edição genérica de recursos.
- Aplicação de YAML.
- Shell privilegiado ou bypass de RBAC.
- Port-forward em endereço não-loopback.
- Persistência ou replay de sessões.

## Critério de saída

As cinco ações funcionam apenas para identidades autorizadas; negações aparecem corretamente na interface e retornam 403 no backend; operações destrutivas exigem confirmação; streams e sessões encerram de forma determinística; nenhum comando ou conteúdo é persistido/logado.

Os caminhos locais, wire contracts e cleanup estão comprovados. F7-43 e
F7-44 permanecem abertos até as cinco identidades e a revogação serem
exercitadas contra a API Kubernetes real.
