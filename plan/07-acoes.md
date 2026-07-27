# Fase 7 — Ações autorizadas

**Estado inicial:** pendente

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

- [ ] **F7-01** Mapear para cada ação grupo de API, recurso, subresource e verbo exatos.
- [ ] **F7-02** Reutilizar e estender o `AuthorizationService`/helper da Fase 4 para revalidar SAR, sem criar uma autorização paralela nem confiar no resultado antigo da interface.
- [ ] **F7-03** Aplicar proteção de Origin/CSRF definida na Fase 2 a todas as requisições mutáveis e upgrades.
- [ ] **F7-04** Criar DTO de confirmação com contexto, namespace, kind, nome, ação e consequência.
- [ ] **F7-05** Exigir confirmação explícita para operações destrutivas e impedir replay conforme contrato.
- [ ] **F7-06** Criar registro em memória para sessões, limites de concorrência, cancelamento e cleanup.
- [ ] **F7-07** Registrar somente metadados operacionais: nunca conteúdo de terminal, comando, log, token ou stream.
- [ ] **F7-08** Padronizar forbidden, conflict, timeout, cancelamento, alvo inexistente e cluster offline.
- [ ] **F7-09** Atualizar capabilities após sucesso, negação ou mudança de contexto.

## Ação 1 — Restart de Deployment

- [ ] **F7-10** Confirmar permissão necessária para o patch de Deployment.
- [ ] **F7-11** Implementar restart por patch mínimo da pod template conforme ADR, preservando campos não relacionados.
- [ ] **F7-12** Implementar endpoint `POST /api/v1/workloads/{kind}/{namespace}/{name}/restart` restrito ao kind suportado no MVP.
- [ ] **F7-13** Exibir confirmação, progresso e resultado sem afirmar que o rollout terminou antes da condição real.
- [ ] **F7-14** Testar permitido, negado, conflito de versão, alvo removido e request cancelado.

## Ação 2 — Scale

- [ ] **F7-15** Verificar `update`/`patch` no subresource `scale` conforme contrato e comportamento do cluster.
- [ ] **F7-16** Validar réplica mínima/máxima e representar a consequência de escalar para zero.
- [ ] **F7-17** Implementar scale para Deployment e StatefulSet via subresource, sem substituir o objeto completo.
- [ ] **F7-18** Implementar `PUT /api/v1/workloads/{kind}/{namespace}/{name}/scale`.
- [ ] **F7-19** Testar permitido, negado, valor inválido, conflito, cancelamento e atualização da interface.

## Ação 3 — Delete Pod

- [ ] **F7-20** Verificar permissão `delete pods` no namespace do alvo.
- [ ] **F7-21** Exibir owner e explicar que a recriação depende do controlador; não prometer recriação para pod sem owner.
- [ ] **F7-22** Implementar `DELETE /api/v1/pods/{namespace}/{name}` com preconditions definidas no contrato.
- [ ] **F7-23** Exigir confirmação destrutiva e invalidar listas relacionadas após sucesso.
- [ ] **F7-24** Testar pod controlado, pod independente, negado, alvo já removido e conflito.

## Ação 4 — Port-forward

- [ ] **F7-25** Derivar o verbo de `pods/portforward` do método do transporte escolhido — normalmente `create` para POST — e testar o atributo SAR exato.
- [ ] **F7-26** Validar pod, bind loopback e faixas das portas remota/local, sem exigir container nem `containerPort` declarado.
- [ ] **F7-27** Escutar somente em loopback e escolher porta local disponível quando omitida.
- [ ] **F7-28** Implementar `POST /api/v1/pods/{namespace}/{name}/port-forward`, `GET /api/v1/port-forwards` e `DELETE /api/v1/port-forwards/{id}`.
- [ ] **F7-29** Exibir sessões ativas na área Network, com contexto, namespace, pod, portas e estado.
- [ ] **F7-30** Encerrar sessão em ação do usuário, troca de contexto, término do pod ou shutdown.
- [ ] **F7-31** Limitar quantidade de sessões e evitar goroutines/sockets órfãos.
- [ ] **F7-32** Testar permitido, negado, porta ocupada, conexão interrompida, cancelamento e cleanup.

## Ação 5 — Exec

- [ ] **F7-33** Derivar o verbo de `pods/exec` do método do transporte escolhido e revalidar o atributo SAR imediatamente antes do upgrade.
- [ ] **F7-34** Validar namespace, pod, container e estado executável do alvo.
- [ ] **F7-35** Aprovar o transporte do client-go; usar `pkg/ws` apenas se o spike provar ou complementar com segurança Origin, masking, opcodes, fragmentação, ping/pong, limites e deadlines.
- [ ] **F7-36** Definir stdin, stdout, stderr, TTY, resize, heartbeat e encerramento no contrato.
- [ ] **F7-37** Validar command/argv como lista, limites de sessões/mensagens, timeout de idle e cancelamento, sem concatenação de shell.
- [ ] **F7-38** Implementar `POST /api/v1/pods/{namespace}/{name}/exec` e o transporte aprovado, sem registrar comando nem saída.
- [ ] **F7-39** Fechar a sessão em troca de contexto/escopo, perda do socket, término do container ou shutdown.
- [ ] **F7-40** Criar interface de terminal somente após revisão de segurança e justificativa de qualquer dependência adicional.
- [ ] **F7-41** Testar permitido, negado, container inválido, resize, desconexão abrupta, backpressure e cleanup.
- [ ] **F7-42** Testar argv adversarial, ausência de shell implícito e ausência de comando/saída nos logs.

### E2E e fechamento

- [ ] **F7-43** Estender o harness restrito com identidades separadas para restart, scale, delete, port-forward e exec.
- [ ] **F7-44** Executar cada ação no caminho permitido e provar 403 no caminho negado, inclusive após revogação de RBAC.
- [ ] **F7-45** Verificar que confirmações exibem o alvo correto e que nenhuma sessão, porta ou goroutine permanece após cancelamento.
- [ ] **F7-46** Inspecionar logs e SQLite após exec/port-forward para provar ausência de comando, saída e payload.
- [ ] **F7-47** Executar e registrar `ginger inspect`, `ginger doctor`, testes, lint e build no fechamento da fase.

## Gate de cada ação

Antes de iniciar a ação seguinte, a atual precisa comprovar:

- capability correta na interface;
- ação ausente/desabilitada para usuário negado;
- SAR revalidado no backend;
- confirmação contextual quando aplicável;
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
| WebSocket Ginger aceitar frames/Origins inseguros | Gate adversarial; não liberar exec com `pkg/ws` sem hardening comprovado |
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
