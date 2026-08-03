# Fase 2 — Especificação

**Estado:** concluída em 2026-08-03

**Dependências:** gate da Fase 1 concluído
**Gate seguinte:** código de produção começa apenas depois que contratos e critérios de aceite estiverem objetivos.

## Objetivo

Converter os requisitos do prompt e as descobertas técnicas em especificações implementáveis. Esta fase fecha limites de produto, arquitetura, segurança, dados, API e experiência de uso antes do scaffold definitivo.

## Entregáveis obrigatórios

- `docs/product-spec.md`
- `docs/architecture.md`
- `docs/security.md`
- `docs/data-model.md`
- `docs/api.md`
- `docs/implementation-plan.md`
- ADRs adicionais identificados durante a especificação;
- atualização da [matriz de aceite](matriz-aceite-mvp.md) com a evidência planejada em nível de teste.

## Tarefas ordenadas

### Produto e experiência

- [x] **F2-01** Definir personas, problemas resolvidos, objetivos, não objetivos e fronteira exata do MVP.
- [x] **F2-02** Especificar os fluxos: primeiro uso, seleção de contexto, seleção de escopo, overview, navegação de recursos, logs e ações.
- [x] **F2-03** Definir estados de carregamento, vazio, offline, proibido, parcialmente disponível e cancelado.
- [x] **F2-04** Definir quando uma ação fica oculta e quando fica visível porém desabilitada com explicação.
- [x] **F2-05** Especificar a arquitetura de informação e o menu compacto: Overview, Workloads, Pods, Logs, Events, Network, Config, Namespaces, Permissions e Settings.
- [x] **F2-06** Documentar tokens visuais Catppuccin Mocha, acento mauve, densidade, tipografia monoespaçada e requisitos de acessibilidade.
- [x] **F2-07** Definir os critérios de aceite por jornada e ligar cada um à matriz do MVP.

### Arquitetura

- [x] **F2-08** Desenhar contexto, containers e componentes do processo local.
- [x] **F2-09** Fixar a regra de dependência hexagonal: handlers dependem de ports/services e nunca do clientset.
- [x] **F2-10** Especificar as interfaces `KubeconfigLoader`, `ContextService`, `NamespaceService`, `AuthorizationService`, `WorkloadService`, `PodService`, `LogService`, `EventService`, `NetworkService`, `ConfigResourceService`, `MetricsService`, `PreferenceService`, `ActionService`, `PortForwardService`, `ExecService` e `DashboardService`.
- [x] **F2-11** Definir composição do entrypoint Cobra, bootstrap Ginger, ownership do listener/HTTP, SQLite, browser, sinais, cleanup e arquivos de runtime.
- [x] **F2-12** Definir ownership e cancelamento ao trocar contexto/escopo, fechar página ou substituir refresh.
- [x] **F2-13** Especificar cache de clientsets, cache RBAC, invalidações, limites de concorrência e política de timeout/backoff.
- [x] **F2-14** Especificar watches compartilhados, SSE unidirecional e um transporte bidirecional seguro para exec, condicionado ao spike do Ginger.
- [x] **F2-15** Definir carregamento progressivo do dashboard e contrato de erros parciais por bloco.

### Segurança

- [x] **F2-16** Produzir threat model para aplicação local, kubeconfig, plugins `exec`, API HTTP, browser, SQLite, logs, port-forward e exec.
- [x] **F2-17** Definir bind exclusivo em loopback e proteções contra Host header/DNS rebinding, Origin indevida e CSRF em operações mutáveis, incluindo `GET /api/v1/session`, TTL/rotação do nonce e rebootstrap após mudança de geração.
- [x] **F2-18** Criar classificação de dados: persistível, apenas em memória, sensível e proibido.
- [x] **F2-19** Definir redaction por chave e conteúdo, sinks/rotação do arquivo local e sanitização de tokens, JWTs, senhas, chaves privadas, connection strings, headers e erros de plugins.
- [x] **F2-20** Especificar consulta e cache de RBAC por contexto, namespace, API group, recurso, subresource e verbo.
- [x] **F2-21** Definir comportamento fail-closed, erro 403 padronizado e revalidação imediatamente antes de toda ação.
- [x] **F2-22** Definir confirmação contextual para ações destrutivas e a política de logs operacionais sem conteúdo sensível.
- [x] **F2-23** Definir política de Secrets: somente metadados; nenhum valor em DTO, YAML, log, fixture ou banco.

### Dados

- [x] **F2-24** Especificar tabelas, tipos, chaves, timestamps, índices e foreign keys de `cluster_profiles`, `namespace_scopes`, `namespace_scope_items` e `preferences`; scopes incluem `context_name` e unicidade por profile/contexto, e profiles são reconciliados somente pelo bootstrap a partir dos paths resolvidos.
- [x] **F2-25** Definir migrations versionadas, transações e estratégia de rollback/backup durante upgrade.
- [x] **F2-26** Definir constraints de `single`, `list`, `all`, namespace padrão e índice único de item.
- [x] **F2-27** Garantir que `all` seja atributo e nunca o namespace `*`.
- [x] **F2-28** Documentar tudo o que é proibido persistir e como inspecionar o banco nos testes.

### API

- [x] **F2-29** Classificar rotas como MVP obrigatório, posterior ao MVP ou suporte interno.
- [x] **F2-30** Definir DTOs de request/response sem expor objetos completos do client-go.
- [x] **F2-31** Fixar envelopes Ginger para sucesso e erros e definir DTO cursor próprio para paginação Kubernetes.
- [x] **F2-32** Definir `limit`, `continue`, `search`, `namespace`, `status`, `sort` e `order`, incluindo limites máximos e token opaco.
- [x] **F2-33** Definir códigos de erro estáveis e decoding JSON estrito para validação, RBAC, cluster offline, contexto inválido, timeout, cancelamento, campo desconhecido, body excedente e conflito.
- [x] **F2-34** Especificar contratos de profile sanitizado, contexto, escopos, permissões, dashboard, recursos, logs, ações e sessões, incluindo lifecycle de geração/CSRF ao atualizar ou excluir o scope ativo.
- [x] **F2-35** Especificar rotas raw, eventos SSE e o fluxo único de exec: `ExecInit` completo no POST, ticket one-shot ligado ao request e WebSocket apenas para frames tipados; incluir validação de Origin, limites de frame/payload, reconexão, backpressure, heartbeat e encerramento.
- [x] **F2-36** Definir idempotência onde aplicável e semântica de requests cancelados.

### Plano de implementação

- [x] **F2-37** Transformar cada fase seguinte em tarefas rastreáveis a requisito, endpoint, tela e teste.
- [x] **F2-38** Definir fixtures sintéticos e a matriz de identidades RBAC para integração/E2E.
- [x] **F2-39** Definir os comandos oficiais de lint, teste, build, smoke e E2E no `docs/implementation-plan.md`.
- [x] **F2-40** Revisar todas as especificações em conjunto e resolver contradições antes do gate.
- [x] **F2-41** Fixar nomenclatura, casing, module path, nomes de archives e URLs de instalação canônicos.
- [x] **F2-42** Definir o contrato operacional de foreground/daemon, bind por tentativa real, prontidão, single instance, `start`, `stop`, `status`, timeout e sinais por plataforma.
- [x] **F2-43** Definir allowlist e schema versionado de preferências, filtros salvos e configurações do dashboard.
- [x] **F2-44** Proibir impersonation, coleta de credenciais adicionais e qualquer autorização fora do Kubernetes.
- [x] **F2-45** Definir `Cache-Control: no-store` para API Kubernetes/logs/permissões e impedir cache offline/service worker de dados do cluster.
- [x] **F2-46** Definir OpenTelemetry como opt-in, desativado por padrão e sem dependência obrigatória no core.
- [x] **F2-47** Planejar um harness Kind canônico incremental a partir da Fase 4, mantendo K3d somente como alternativa local equivalente, para não postergar a validação RBAC real até a distribuição.
- [x] **F2-48** Definir como `resourceName` participa das consultas e chaves de cache de autorização para ações sobre objetos específicos.
- [x] **F2-49** Definir cursor composto vinculado a query/context generation para fan-out em namespace/kind, merge determinístico, expiração/410 e semântica honesta de busca/ordenação.
- [x] **F2-50** Fixar o payload de `/health` com aplicação, SQLite, kubeconfig, cluster e contexto, distinguindo falha local crítica de dependência Kubernetes degradada.
- [x] **F2-51** Fixar o schema observável `timestamp`, `level`, `component`, `operation`, `request_id`, `context`, `namespace`, `resource`, `duration` e `error_code`.
- [x] **F2-52** Fixar `GET /api/v1/workloads/{kind}/{namespace}/{name}/yaml`, `GET /api/v1/pods/{namespace}/{name}/yaml` e o padrão equivalente para recursos permitidos; Secret não possui rota YAML.
- [x] **F2-53** Fixar `GET /api/v1/endpoint-slices` e `GET /api/v1/secrets` metadata-only, incluindo paginação e a allowlist exata de campos expostos.
- [x] **F2-54** Incluir `complete`, `truncated`, cobertura de namespaces e erros parciais em agregações limitadas por budget.
- [x] **F2-55** Definir limites de bytes por linha, container, stream e scan, além de linhas/pods/concorrência.
- [x] **F2-56** Definir SQLite por conexão: foreign keys, busy timeout, pool, journal/WAL, lock de instância e inspeção de journal/backup.
- [x] **F2-57** Definir LIST inicial + resourceVersion, permissão distinta de watch, relist após 410, bookmarks, timeoutSeconds e fallback quando watch for negado.
- [x] **F2-58** Definir como o conjunto ordenado de arquivos de `KUBECONFIG` é representado/persistido sem armazenar conteúdo ou credenciais.
- [x] **F2-59** Fixar `GET /api/v1/port-forwards` e `DELETE /api/v1/port-forwards/{id}` para lifecycle das sessões criadas pelo endpoint do Pod.
- [x] **F2-60** Fixar `GET/PUT /api/v1/preferences` com chaves allowlisted para Settings, filtros e dashboard.
- [x] **F2-61** Definir checks, formato, códigos de saída e sanitização de `kubePeep doctor`, distinguindo falha local de cluster apenas degradado.

## Decisões que não podem ficar implícitas

- base `--service` adaptada com Cobra ou composição alternativa comprovada;
- semântica HTTP de `/health` quando cluster, contexto ou Metrics API estiverem indisponíveis;
- precedência `--kubeconfig` > `KUBECONFIG` > profile persistido default > path recomendado, com contexto e `--namespace` definidos;
- representação de múltiplos arquivos em `KUBECONFIG`;
- escopo e chave de cada cache;
- paginação e busca em listas Kubernetes;
- protocolo de logs follow, port-forward e exec;
- proteção da API local contra chamadas originadas por páginas externas;
- preservação de `Flusher`/`Hijacker` e hardening do transporte de exec;
- sink/rotação de logs e cursor Kubernetes, dadas as limitações dos helpers Ginger;
- estratégia de restart e scale por kind/subresource;
- experiência de update e remoção.
- casing canônico de produto, comando, módulo e repositório;
- política de preferências persistíveis e cache HTTP de dados do cluster;

## Validação documental

- Cada endpoint possui request, response, erros, autorização e paginação quando necessária.
- Cada fluxo de interface possui estados permitido, negado, vazio, offline e parcial.
- Cada dado sensível possui política explícita de memória, persistência e log.
- Cada um dos 27 itens de aceite aponta para ao menos um teste futuro.
- Diagramas e texto usam os mesmos nomes de ports, services, DTOs e adapters.
- Nenhuma especificação contradiz o uso obrigatório do Ginger v1.4.4.

## Fora de escopo

- Implementação ou scaffold definitivo.
- Decisões de produto sem relação com o MVP.
- Edição irrestrita de YAML, conteúdo de Secrets, autenticação própria ou servidor cloud.

## Critério de saída

Os seis documentos obrigatórios estão revisados, coerentes entre si e contêm critérios objetivos. Não há decisão de segurança, lifecycle, persistência ou contrato HTTP necessária para a Fase 3 marcada como “a decidir”.
