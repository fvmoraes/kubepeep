# Fase 6 — Recursos somente leitura

**Estado inicial:** pendente

**Dependências:** Fase 5 concluída e fundação Kubernetes/RBAC da Fase 4
**Gate seguinte:** a Fase 7 reutiliza detalhes, capabilities e lifecycle de streams desta fase.

## Objetivo

Implementar navegação paginada, detalhes, YAML seguro e streams para os recursos Kubernetes permitidos. Todas as telas devem aplicar escopo, RBAC, DTOs próprios, timeout e cancelamento.

## Entregáveis

- áreas Workloads, Pods, Logs, Events, Network e Config;
- endpoints de lista/detalhe definidos como MVP;
- filtros e paginação com `continue`;
- logs atuais, anteriores e follow;
- YAML somente leitura com tratamento especial de Secrets;
- watches centralizados e SSE onde houver benefício comprovado;
- testes de API, services, adapters e frontend.

## Tarefas ordenadas

### Fundação de consultas

- [ ] **F6-01** Criar opções comuns de lista: escopo, `limit`, `continue`, `search`, `namespace`, `status`, `sort` e `order`.
- [ ] **F6-02** Aplicar limites máximos e usar cursor composto opaco para fan-out multi-namespace, preservando o token de cada chamada sem converter para page/per-page.
- [ ] **F6-03** Implementar merge determinístico e semântica documentada de busca/ordenação, deixando claro quando operam apenas na página coletada.
- [ ] **F6-04** Criar conversores puros de objetos Kubernetes para DTOs compactos.
- [ ] **F6-05** Normalizar idade, owner, ready/desired e condições sem fabricar valores ausentes.
- [ ] **F6-06** Implementar serialização YAML somente para recursos autorizados e sob demanda.
- [ ] **F6-07** Aplicar autorização por endpoint e por subresource antes da chamada real.
- [ ] **F6-08** Mapear listas parcialmente negadas sem derrubar namespaces permitidos.

### Workloads

- [ ] **F6-09** Implementar lista e detalhe de Deployments.
- [ ] **F6-10** Implementar lista e detalhe de StatefulSets.
- [ ] **F6-11** Implementar lista e detalhe de DaemonSets.
- [ ] **F6-12** Implementar lista e detalhe de Jobs.
- [ ] **F6-13** Implementar lista e detalhe de CronJobs.
- [ ] **F6-14** Implementar `GET /api/v1/workloads` e `GET /api/v1/workloads/{kind}/{namespace}/{name}`.
- [ ] **F6-15** Criar abas, tabela, filtros, detalhe e YAML na interface.

### Pods

- [ ] **F6-16** Implementar lista paginada de pods com namespace, nome, status, ready, restarts, node, IP, owner e idade.
- [ ] **F6-17** Implementar filtros por namespace, status, workload, node, restart, problemático e texto.
- [ ] **F6-18** Implementar detalhe com containers, init/ephemeral containers, condições e eventos relacionados permitidos.
- [ ] **F6-19** Implementar `GET /api/v1/pods` e `GET /api/v1/pods/{namespace}/{name}`.
- [ ] **F6-20** Criar tabela, detalhe, links para logs e YAML na interface.

### Logs

- [ ] **F6-21** Implementar seleção de namespace, pod e container condicionada a RBAC.
- [ ] **F6-22** Implementar logs atuais com timestamps, tail, since e `LimitBytes`, complementados por reader limitado.
- [ ] **F6-23** Implementar logs anteriores quando o container possuir instância anterior.
- [ ] **F6-24** Implementar follow com `pkg/sse` sobre rota raw, cadeia que preserve `http.Flusher`, tamanho máximo de evento e cancelamento por `request.Context()`, aplicando a estratégia validada para streams acima do timeout padrão do Ginger.
- [ ] **F6-25** Definir buffer/ring buffer, limite de bytes e backpressure; encerrar cliente lento conforme contrato.
- [ ] **F6-26** Implementar busca local, pausa visual, wrap, cópia e download apenas por ação explícita.
- [ ] **F6-27** Não registrar nem persistir conteúdo de logs.
- [ ] **F6-28** Encerrar stream ao sair da página, trocar seleção/contexto ou conforme a política explícita de revalidação; não prometer notificação instantânea de revogação RBAC.
- [ ] **F6-29** Implementar `GET /api/v1/pods/{namespace}/{name}/logs` e `GET /api/v1/pods/{namespace}/{name}/logs/stream` com erros padronizados.

### Events

- [ ] **F6-30** Implementar lista cronológica paginada com filtros por namespace, tipo, objeto, reason e texto.
- [ ] **F6-31** Preservar timestamps, count, source e mensagens reais.
- [ ] **F6-32** Reutilizar agrupamento somente quando explicitamente solicitado pelo contrato.
- [ ] **F6-33** Implementar `GET /api/v1/events` e a tela Events.

### Network

- [ ] **F6-34** Implementar Services com ports, selectors, type, cluster IP e external endpoints permitidos.
- [ ] **F6-35** Implementar Ingresses `networking.k8s.io/v1` com hosts, paths e backends.
- [ ] **F6-36** Implementar EndpointSlices ou Endpoints conforme discovery/capability do cluster.
- [ ] **F6-37** Implementar `GET /api/v1/services`, `GET /api/v1/ingresses`, a rota aprovada de EndpointSlices e suas rotas/telas de detalhe.
- [ ] **F6-38** Reservar a área de sessões de port-forward para a Fase 7 sem simular sessões.

### Config

- [ ] **F6-39** Listar ConfigMaps por resposta metadata-only; buscar conteúdo completo apenas no detalhe e quando `get` for permitido.
- [ ] **F6-40** Listar Secrets apenas via `PartialObjectMetadata`/metadata API e allowlist aprovada, excluindo annotations, managedFields e qualquer campo não enumerado.
- [ ] **F6-41** Impedir que conversor genérico ou YAML exponha `data`, `stringData`, annotations, managedFields ou valores equivalentes de Secret.
- [ ] **F6-42** Implementar `GET /api/v1/configmaps`, a rota aprovada de metadados de Secrets e suas telas de detalhe com mensagens claras de limitação.

### Atualização em tempo real

- [ ] **F6-43** Criar manager backend com LIST inicial + resourceVersion e watch por contexto, escopo, GVR e seletor.
- [ ] **F6-44** Compartilhar watch entre consumidores compatíveis; nunca criar um watcher por componente React.
- [ ] **F6-45** Tratar `resourceVersion`, permissão `watch` distinta, encerramento, `410 Gone` com relist, bookmarks, timeoutSeconds, reconexão e backoff.
- [ ] **F6-46** Multiplexar atualizações simples por SSE quando o benefício superar HTTP manual.
- [ ] **F6-47** Limitar subscribers, buffers e watchers e expor métricas/logs operacionais.
- [ ] **F6-48** Cancelar todo watch e descartar eventos SSE enfileirados por generation ID ao trocar contexto/escopo ou encerrar o processo.

### Settings, preferências e fechamento

- [ ] **F6-49** Implementar adapter/service e `GET/PUT /api/v1/preferences` com allowlist, validação, versionamento e escrita transacional.
- [ ] **F6-50** Implementar a tela Settings para opções locais aprovadas, sem aceitar kubeconfig, token, Secret ou configuração arbitrária.
- [ ] **F6-51** Implementar filtros salvos para telas suportadas, vinculados ao contexto/escopo quando necessário.
- [ ] **F6-52** Aplicar `Cache-Control: no-store` a respostas de API com recursos, permissões, YAML e logs.
- [ ] **F6-53** Garantir que service worker, browser cache e TanStack Query não persistam dados do cluster fora da memória.
- [ ] **F6-54** Completar logs com toggle de timestamps, fonte monoespaçada e estado claro de follow/pausa.
- [ ] **F6-55** Implementar as rotas de YAML sob demanda definidas na Fase 2, sem incluir YAML em respostas de lista.
- [ ] **F6-56** Estender o harness restrito com workloads, logs atuais/anteriores, Service, Ingress, ConfigMap e Secret sintético.
- [ ] **F6-57** Executar E2E lista → detalhe → YAML/logs nos caminhos permitido e negado.
- [ ] **F6-58** Revalidar ou encerrar streams longos segundo a política RBAC definida, sem prometer detecção instantânea de mudança de permissão.
- [ ] **F6-59** Testar cursor composto com namespaces em páginas diferentes, autorização parcial, retomada e ordenação determinística.
- [ ] **F6-60** Executar e registrar `ginger inspect`, `ginger doctor`, testes, lint e build no fechamento da fase.

### Robustez final

- [ ] **F6-61** Rejeitar cursor expirado, de outra query/coleção/context generation ou usado depois de troca de escopo; nunca registrar seu conteúdo.
- [ ] **F6-62** Tratar `410 ResourceExpired` também em LIST paginada e reiniciar de modo explícito, sem combinar snapshots inconsistentes silenciosamente.
- [ ] **F6-63** Implementar fallback para refresh HTTP quando `list` for permitido e `watch` negado.
- [ ] **F6-64** Proibir LIST/watch de objetos completos de Secret e provar que seus valores nunca entram na memória da aplicação.
- [ ] **F6-65** Testar linha de log gigante, stream sem newline, cliente lento e ring buffer frontend no limite.
- [ ] **F6-66** Quando watch global for negado, aplicar SAR e fan-out por namespace com limite rígido; acima do limite, usar refresh HTTP explícito.
- [ ] **F6-67** Expor `complete/truncated`, cobertura e erros parciais em listas fan-out; nunca apresentar página parcial como coleção global completa.

## Ordem recomendada das fatias

1. Pods lista → detalhe → YAML.
2. Workloads lista → detalhe → YAML.
3. Logs atuais → anteriores → follow.
4. Events.
5. Services → Ingresses → EndpointSlices.
6. ConfigMaps → metadados de Secrets.
7. Watches compartilhados somente para telas que já funcionam por HTTP.
8. Settings, preferências e filtros salvos.

Cada fatia inclui port, adapter, service, handler/DTO, frontend e testes antes de iniciar a próxima.

## Testes obrigatórios

- paginação com `continue`, limite máximo e token inválido;
- cursor composto multi-namespace/kind, expiração e `410 ResourceExpired`;
- busca e ordenação sem leitura ilimitada;
- lista multi-namespace com autorização parcial;
- conversores de cada kind;
- lista e detalhe permitidos/negados;
- logs atuais, anteriores, follow, cliente lento e cancelamento;
- limites de bytes, linha gigante e ring buffer;
- troca de pod/container fechando stream anterior;
- watch reconectando após encerramento e `410 Gone`;
- troca de contexto fechando watchers;
- ConfigMap autorizado e negado;
- Secret metadata-only sem valores na memória, JSON, YAML, logs ou snapshots de teste;
- Settings/preferências aceitando somente chaves allowlisted;
- interface com filtros, paginação, vazio, offline, proibido e erro parcial.

## Riscos específicos

| Risco | Mitigação |
| --- | --- |
| Filtro textual exigir lista gigante | Limites e estratégia fechada na especificação |
| Watch se multiplicar por tela | Manager compartilhado e contadores de subscribers |
| Stream lento consumir memória | Buffer finito e política de desconexão |
| YAML revelar campo sensível | Conversores por tipo e testes de ausência |
| Recursos variam por versão do cluster | Discovery e APIs estáveis do client-go |

## Fora de escopo

- Edição de YAML.
- Conteúdo de Secrets.
- Delete, restart, scale, port-forward ou exec.
- Watch indiscriminado de todos os recursos.

## Critério de saída

O usuário navega das listas aos detalhes/YAML de todos os recursos somente leitura do MVP, visualiza logs permitidos e streams canceláveis, recebe paginação e filtros limitados; valores de Secret nunca entram na memória/resposta, e nenhum objeto interno do client-go é devolvido.
