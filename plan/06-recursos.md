# Fase 6 — Recursos somente leitura

**Estado atual:** implementação local concluída (66/67); E2E real no Kind pendente

**Evidência:** [relatório rastreável da Fase 6](../docs/research/phase6-evidence.md)

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

- [x] **F6-01** Criar opções comuns de lista: escopo, `limit`, `continue`, `search`, `namespace`, `status`, `sort` e `order`.
- [x] **F6-02** Aplicar limites máximos e usar cursor composto opaco para fan-out multi-namespace, preservando o token de cada chamada sem converter para page/per-page.
- [x] **F6-03** Implementar merge determinístico e semântica documentada de busca/ordenação, deixando claro quando operam apenas na página coletada.
- [x] **F6-04** Criar conversores puros de objetos Kubernetes para DTOs compactos.
- [x] **F6-05** Normalizar idade, owner, ready/desired e condições sem fabricar valores ausentes.
- [x] **F6-06** Implementar serialização YAML somente para recursos autorizados e sob demanda.
- [x] **F6-07** Aplicar autorização por endpoint e por subresource antes da chamada real.
- [x] **F6-08** Mapear listas parcialmente negadas sem derrubar namespaces permitidos.

### Workloads

- [x] **F6-09** Implementar lista e detalhe de Deployments.
- [x] **F6-10** Implementar lista e detalhe de StatefulSets.
- [x] **F6-11** Implementar lista e detalhe de DaemonSets.
- [x] **F6-12** Implementar lista e detalhe de Jobs.
- [x] **F6-13** Implementar lista e detalhe de CronJobs.
- [x] **F6-14** Implementar `GET /api/v1/workloads` e `GET /api/v1/workloads/{kind}/{namespace}/{name}`.
- [x] **F6-15** Criar abas, tabela, filtros, detalhe e YAML na interface.

### Pods

- [x] **F6-16** Implementar lista paginada de pods com namespace, nome, status, ready, restarts, node, IP, owner e idade.
- [x] **F6-17** Implementar filtros por namespace, status, workload, node, restart, problemático e texto.
- [x] **F6-18** Implementar detalhe com containers, init/ephemeral containers, condições e eventos relacionados permitidos.
- [x] **F6-19** Implementar `GET /api/v1/pods` e `GET /api/v1/pods/{namespace}/{name}`.
- [x] **F6-20** Criar tabela, detalhe, links para logs e YAML na interface.

### Logs

- [x] **F6-21** Implementar seleção de namespace, pod e container condicionada a RBAC.
- [x] **F6-22** Implementar logs atuais com timestamps, tail, since e `LimitBytes`, complementados por reader limitado.
- [x] **F6-23** Implementar logs anteriores quando o container possuir instância anterior.
- [x] **F6-24** Implementar follow com `pkg/sse` sobre rota raw e wire contract fixo: meta/line/heartbeat/error/end, sem `Last-Event-ID`, linha 64 KiB, evento 68 KiB, buffer 1 MiB/1.000, heartbeat 15 s, duração 4 h e cancelamento por `request.Context()`.
- [x] **F6-25** Definir buffer/ring buffer, limite de bytes e backpressure; encerrar cliente lento conforme contrato.
- [x] **F6-26** Implementar busca local, pausa visual, wrap, cópia e download apenas por ação explícita.
- [x] **F6-27** Não registrar nem persistir conteúdo de logs.
- [x] **F6-28** Encerrar stream ao sair da página, trocar seleção/contexto ou conforme a política explícita de revalidação; não prometer notificação instantânea de revogação RBAC.
- [x] **F6-29** Implementar `GET /api/v1/pods/{namespace}/{name}/logs` e `GET /api/v1/pods/{namespace}/{name}/logs/stream` com guards antes dos headers e eventos terminais depois do início do SSE.

### Events

- [x] **F6-30** Implementar lista cronológica paginada com filtros por namespace, tipo, objeto, reason e texto.
- [x] **F6-31** Preservar timestamps, count, source e mensagens reais.
- [x] **F6-32** Reutilizar agrupamento somente quando explicitamente solicitado pelo contrato.
- [x] **F6-33** Implementar `GET /api/v1/events` e a tela Events.

### Network

- [x] **F6-34** Implementar o port `NetworkService` e seu adapter para Services com ports, selectors, type, cluster IP e external endpoints permitidos.
- [x] **F6-35** Implementar Ingresses `networking.k8s.io/v1` com hosts, paths e backends.
- [x] **F6-36** Implementar somente EndpointSlices `discovery.k8s.io/v1`; se indisponível, retornar `FEATURE_UNAVAILABLE`, sem fallback implícito para Endpoints.
- [x] **F6-37** Implementar list/detail/YAML de `services`, `ingresses` e `endpoint-slices` exatamente nas rotas de `docs/api.md`, com as respectivas telas de detalhe.
- [x] **F6-38** Reservar a área de sessões de port-forward para a Fase 7 sem simular sessões.

### Config

- [x] **F6-39** Implementar o port `ConfigResourceService`: listar ConfigMaps por resposta metadata-only e buscar conteúdo completo apenas no detalhe e quando `get` for permitido.
- [x] **F6-40** Listar Secrets apenas via `PartialObjectMetadata`/metadata API e allowlist aprovada, excluindo annotations, managedFields e qualquer campo não enumerado.
- [x] **F6-41** Impedir que conversor genérico ou YAML exponha `data`, `stringData`, annotations, managedFields ou valores equivalentes de Secret.
- [x] **F6-42** Implementar list/detail/YAML de `configmaps` e list/detail metadata-only de `secrets` exatamente nas rotas de `docs/api.md`; Secret não possui YAML e a UI explica a limitação.

### Atualização em tempo real

- [x] **F6-43** Criar manager backend com LIST inicial + resourceVersion e watch por contexto, escopo, GVR e seletor.
- [x] **F6-44** Compartilhar watch entre consumidores compatíveis; nunca criar um watcher por componente React.
- [x] **F6-45** Tratar `resourceVersion`, permissão `watch` distinta, encerramento, `410 Gone` com relist, bookmarks, timeoutSeconds, reconexão e backoff.
- [x] **F6-46** Implementar `/api/v1/stream` com a allowlist exata de sete tópicos e sua matriz GVR/DTO: exigir 1–7 sem duplicata/desconhecido, proibir Secrets, autorizar list+watch all-or-nothing e aplicar IDs/binding, chunks, eventos, replay, heartbeat e terminal do wire contract.
- [x] **F6-47** Limitar oito streams, evento serializado de 64 KiB, snapshot ao primeiro entre 10.000 items/10 MiB e ring/fila ao menor entre 1 MiB e 1.000 eventos; snapshot excedente envia `snapshot_too_large` e todos os chunks são descartados, nunca apresentados como truncamento utilizável; expor somente métricas/logs operacionais sem payload.
- [x] **F6-48** Cancelar todo watch e descartar eventos SSE enfileirados por generation ID ao trocar contexto/escopo ou encerrar o processo.

### Settings, preferências e fechamento

- [x] **F6-49** Implementar o port `PreferenceService`, seu adapter e `GET/PUT /api/v1/preferences` com allowlist, validação, versionamento e escrita transacional; a partir daqui persistir as chaves preparadas na F5.
- [x] **F6-50** Implementar a tela Settings para opções locais aprovadas, sem aceitar kubeconfig, token, Secret ou configuração arbitrária.
- [x] **F6-51** Implementar filtros salvos para telas suportadas, vinculados ao contexto/escopo quando necessário.
- [x] **F6-52** Aplicar `Cache-Control: no-store` a respostas de API com recursos, permissões, YAML e logs.
- [x] **F6-53** Garantir que service worker, browser cache e TanStack Query não persistam dados do cluster fora da memória.
- [x] **F6-54** Completar logs com toggle de timestamps, fonte monoespaçada e estado claro de follow/pausa.
- [x] **F6-55** Implementar as rotas de YAML sob demanda definidas na Fase 2, sem incluir YAML em respostas de lista.
- [x] **F6-56** Estender o harness restrito com workloads, logs atuais/anteriores, Service, Ingress, ConfigMap e Secret sintético.
- [ ] **F6-57** Executar E2E lista → detalhe → YAML/logs nos caminhos permitido e negado.
- [x] **F6-58** Revalidar ou encerrar streams longos segundo a política RBAC definida e testar cada tópico/GVR, cardinalidade/duplicata/desconhecido, Secret proibido, list/watch negado all-or-nothing, descarte de snapshot excedente, resume válido, ID malformado, ring expirado, slow consumer, 410/relist em nova conexão, generation change e cada evento terminal, sem prometer detecção instantânea de mudança de permissão.
- [x] **F6-59** Testar cursor composto com namespaces em páginas diferentes, autorização parcial, retomada e ordenação determinística.
- [x] **F6-60** Executar e registrar `ginger inspect`, `ginger doctor`, testes, lint e build no fechamento da fase.

### Robustez final

- [x] **F6-61** Emitir cursor com TTL fixo não deslizante de 5 min e rejeitar cursor expirado, de outra query/coleção/context generation ou usado depois de troca de escopo; nunca registrar seu conteúdo.
- [x] **F6-62** Tratar `410 ResourceExpired` também em LIST paginada e reiniciar de modo explícito, sem combinar snapshots inconsistentes silenciosamente.
- [x] **F6-63** Implementar fallback para refresh HTTP quando `list` for permitido e `watch` negado.
- [x] **F6-64** Proibir LIST/watch de objetos completos de Secret e provar que seus valores nunca entram na memória da aplicação.
- [x] **F6-65** Testar linha gigante/escaping/truncamento até 68 KiB serializados, cumulativo follow de 10 MiB, stream sem newline, heartbeat, `Last-Event-ID` rejeitado, cada `end.reason`, erro após headers, duração, cliente lento e ring buffer frontend no limite.
- [x] **F6-66** Quando watch global for negado, aplicar SAR e fan-out por namespace com limite rígido; acima do limite, usar refresh HTTP explícito.
- [x] **F6-67** Expor `complete/truncated`, cobertura e erros parciais em listas fan-out; nunca apresentar página parcial como coleção global completa.

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

As provas locais de serviço, adapter, protocolo e frontend estão concluídas.
F6-56 e F6-57 permanecem abertas até lista → detalhe → YAML/logs ser executado
contra o Kind canônico nos caminhos permitido e negado.
