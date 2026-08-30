# Estado de execução do Kube Peep — 2026-08-30

Este documento é o inventário consolidado do projeto no ponto de captura de
2026-08-30. Ele registra o que foi especificado, implementado, validado e ainda
depende de evidência externa. O texto usa apenas paths relativos, identificadores
públicos do próprio projeto e resultados sanitizados.

## 1. Escopo e regra de leitura

### 1.1 Snapshot versionado

| Campo | Valor na captura |
| --- | --- |
| Branch | `main` |
| HEAD versionado | `9d7c2ea` — `fix: align restricted Kind validation contracts` |
| Relação com upstream | fetch/prune mostrou somente `origin/main`; nenhuma tag remota ou local |
| Último workflow auditado | verificação #23, run público `33289508322` |
| Resultado do workflow #23 | histórico em Go 1.25.13: 10 de 11 jobs passaram; somente `restricted-kind` falhou; não valida o toolchain atual Go 1.26.7 |
| Release/tag | nenhuma candidate ou tag foi criada por esta execução |

O working tree continha mudanças concorrentes ainda não versionadas quando este
relatório foi criado. Por isso, cada afirmação usa uma destas classes:

- **versionado:** existe no HEAD indicado;
- **validado localmente:** existe no working tree e já possui a validação local
  indicada, mas ainda exige commit e CI do próprio commit;
- **planejado:** contrato ou tarefa documentada sem evidência executada;
- **externo:** depende de Docker/Kind, runner nativo, candidate imutável ou ação
  do provedor.

Contagens de checkboxes medem rastreabilidade do plano, não esforço relativo nem
percentual de maturidade comercial.

### 1.2 Fontes usadas

- planos `plan/01-descoberta.md` a `plan/09-experiencia-operacional.md`;
- matrizes `plan/matriz-aceite-mvp.md` e `plan/matriz-aceite-ux.md`;
- especificações e ADRs versionados em `docs/`;
- evidências em `docs/research/`;
- rotas, símbolos, fronteiras e cobertura observados no grafo do código;
- histórico Git alcançável da branch atual;
- arquivos e testes do working tree, sem copiar logs brutos para este relatório.

## 2. Resumo executivo

O MVP funcional está implementado localmente. As Fases 1–3 e 5–7 estão
concluídas; a Fase 4 mantém somente a matriz exaustiva F4-49 aberta. O Kind
local passou `validate` e `app-e2e`, fechando F8-21–F8-26, mas o comando
`create` reutilizou o cluster existente e não fecha a recriação do zero F8-20.
A distribuição também possui um snapshot atual Go 1.26.7 com seis archives e
smoke do binário verde. F8-34, F8-36 e F8-41 ainda exigem nova CI nativa, e uma
release candidate imutável permanece gate separado. A Fase 9 começou com
paleta de navegação segura e, no working tree atual, evolui listas, filtros e
ordenação.

| Fase | Feitas | Total | Estado objetivo | Gate restante |
| --- | ---: | ---: | --- | --- |
| 1 — Descoberta | 44 | 44 | concluída | nenhum |
| 2 — Especificação | 61 | 61 | concluída | nenhum |
| 3 — Fundação | 54 | 54 | concluída | manutenção contínua da CI nativa |
| 4 — Kubernetes e RBAC | 58 | 59 | revogação real validada | F4-49 |
| 5 — Dashboard | 62 | 62 | concluída, inclusive Kind local | nenhum |
| 6 — Recursos | 67 | 67 | concluída, inclusive E2E local | nenhum |
| 7 — Ações | 47 | 47 | concluída, inclusive RBAC real | nenhum |
| 8 — Distribuição | 43 | 50 | snapshot Go 1.26.7 e Kind local verdes | Kind do zero, CI nativa e candidate |
| 9 — Experiência operacional | 12 | 84 | em execução | 72 tarefas e matriz UX |

Agregados na captura:

| Conjunto | Feitas | Total | Proporção informativa |
| --- | ---: | ---: | ---: |
| Fases originais 1–8 | 436 | 444 | 98,20% |
| Expansão Fase 9 | 12 | 84 | 14,29% |
| Plano completo atual | 448 | 528 | 84,85% |
| Critérios MVP | 25 | 27 | 92,59% |
| Critérios UX | 1 | 15 | 6,67% |

Os bloqueios atuais são independentes: revalidar Go 1.26.7 em CI nativa,
recriar o cluster Kind canônico do zero e exercitar uma candidate imutável.
Publicação e comandos de instalação não podem ser inferidos do snapshot local,
da reutilização do cluster nem do run #23 com toolchain anterior.

## 3. Cronologia consolidada

### 2026-07-27 — planejamento, pesquisa e decisões

- o prompt inicial foi convertido em oito fases executáveis e matriz de aceite;
- DWYT foi tratado como referência de organização e experiência, não como fonte
  de regra de negócio;
- Ginger v1.4.4 foi fixado, inspecionado e exercitado em scaffolds descartáveis;
- foram provados lifecycle Cobra + Ginger, bind por listener real, prontidão,
  SSE duradouro, limitações de WebSocket, cursor composto e controle local;
- foram aceitos os ADRs 0001–0004;
- a Fase 1 fechou 44/44 com provas locais e nativas delimitadas.

### 2026-08-03 — especificação e primeira fundação

- produto, arquitetura, API, dados, segurança e plano de implementação foram
  fechados antes da implementação definitiva;
- 61 tarefas da Fase 2 ganharam owner, contrato e evidência mínima;
- JSON, YAML, links, tabelas e fences da documentação foram validados;
- o módulo Go, o frontend React/TypeScript/Vite e a estrutura de aplicação
  começaram a ser materializados.

### 2026-08-10 — fundação concluída

- lifecycle local, CLI, SQLite, configuração, logging, health, sessão, embed da
  SPA e runtime seguro foram concluídos;
- os cross-builds CGO-free cobriram Linux, macOS e Windows em amd64/arm64;
- o workflow nativo #9 comprovou os caminhos relevantes em macOS e Windows;
- a Fase 3 fechou 54/54.

### 2026-08-24 e 2026-08-25 — MVP local completo

- integração Kubernetes, profiles, contextos, escopos e RBAC tri-state foram
  concluídos localmente;
- dashboard, recursos somente leitura, logs, SSE, preferências e ações foram
  implementados em fatias verticais;
- o commit de implementação do MVP consolidou Fases 4–7, deixando somente os
  cenários Kind como pendência;
- instaladores, updater e harness receberam correções multiplataforma;
- a política de segurança do repositório tornou a ausência de dados sensíveis
  uma premissa inegociável e adicionou gates locais/CI;
- os requisitos inspirados nos facilitadores oficiais do Aptakube foram
  separados em uma Fase 9 e uma matriz UX próprias;
- a primeira fatia F9 entregou a paleta somente de navegação e atalhos seguros.

### 2026-08-30 — CI #23, reconciliação e segunda fatia F9

- o workflow #23, ainda em Go 1.25.13, passou build/test, runtime nativo em
  macOS/Windows, snapshot e os seis smokes nativos de archives;
- F8-34, F8-36 e F8-41 foram fechadas naquele estado, mas reabertas após o
  upgrade para Go 1.26.7 até que nova CI nativa valide o toolchain atual;
- o Kind local posterior reutilizou o cluster, e `validate` mais
  `app-e2e ./dist/kubePeep` passaram, fechando F8-21–F8-26 sem fechar F8-20;
- o Pod do cenário `previous-log` e o Event `000-kp-warning` foram tornados
  idempotentes para permitir repetição segura do harness, com ownership
  estrito, precondição de UID, substituição sequencial e recuperação canônica;
- uma revisão destrutiva adicional fechou as janelas de corrida: erros do API
  server nunca significam ausência, a prova negativa de delete exige
  `Forbidden` e preservação da UID, a recuperação é armada antes do DELETE,
  sinais encerram pelo trap de saída e respostas ambíguas convergem por nova
  remoção com a mesma precondição antes de `create` canônico;
- um snapshot GoReleaser v2.17.1 atual em Go 1.26.7 gerou seis archives e o
  smoke do binário passou;
- a documentação das Fases 5–8 foi reconciliada para não apresentar CI parcial
  como aceite total;
- filtros de listas foram centralizados em estado `draft`/`applied`, com reset,
  filtros ativos, ordenação enviada ao backend e cursor ligado à geração;
- a ordenação natural determinística foi implementada no adapter de recursos
  para contexto único; F9-21 permanece aberta porque ainda falta incorporar a
  origem explícita ao desempate da agregação multi-contexto;
- o cenário de revogação Kind foi realinhado ao contrato tri-state: ausência de
  opinião do authorizer produz `unknown` e resposta pública 503, não uma negação
  403 inventada;
- o workflow passou a adicionar uma anotação Kind estática por estágio; os
  comandos continuam escrevendo no log normal do job, cuja segurança depende
  da sanitização implementada pelo próprio harness.
- fetch/prune mostrou somente `origin/main`, nenhuma tag, e o security gate
  remoto passou sobre os 25 commits alcançáveis, sem evidência de segredo ou
  ref a reescrever.

## 4. Execução por fase

### 4.1 Fase 1 — Descoberta, 44/44

Entregas concluídas:

- baseline de Go, Node, npm, Ginger, Kubernetes, SQLite e plataformas;
- inventário de DWYT com matriz “reutilizar/adaptar/substituir/não copiar”;
- análise do Ginger v1.4.4, inclusive `app`, router, health, SSE, WebSocket,
  generators, `inspect` e `doctor`;
- scaffolds `service` e `cli` comparados fora da árvore de produção;
- spikes de lifecycle, stream, controle local, cursor e embed;
- matriz de cross-build e prova nativa delimitada;
- quatro ADRs aceitos.

Decisões resultantes:

- projeto Ginger do tipo `service` com Cobra integrado manualmente;
- lifecycle HTTP próprio usando componentes Ginger, sem `app.Run()`;
- processo foreground e canal autenticado para `status`/`stop`;
- SSE para fluxos unidirecionais, WebSocket endurecido para `exec`;
- SQLite modernc sem CGO;
- API local em loopback e Kubernetes como autoridade final.

### 4.2 Fase 2 — Especificação, 61/61

Foram fechados seis documentos normativos: produto, arquitetura, segurança,
modelo de dados, API e implementação. Os contratos incluem:

- envelopes HTTP e códigos públicos;
- Host, Origin, CSRF e `Cache-Control: no-store`;
- configuração YAML estrita e CLI versionada;
- profiles, contextos, escopos `single`/`list`/`all` e geração monotônica;
- capability `allowed`/`denied`/`unknown`;
- cursores compostos, budgets, truncamento e falhas parciais;
- logs, watches, SSE, port-forward e protocolo de exec;
- retenção allowlisted e inspeção negativa;
- matriz RBAC e harness Kind canônico.

A validação documental registrada encontrou zero link local inválido, fence
aberta, exemplo estruturado inválido, tabela inconsistente, segredo real, e-mail
ou path pessoal concreto no snapshot auditado.

### 4.3 Fase 3 — Fundação, 54/54

Capacidades versionadas:

- `kubePeep`, `start`, `stop`, `status`, `version`, `doctor` e `update`;
- processo foreground, lock de instância e identidade autenticada;
- listener exclusivamente em `127.0.0.1`, readiness e abertura posterior do
  navegador;
- shutdown LIFO com cleanup mesmo após timeout ou erro;
- `config.yaml` v1 estrito, roots privados e adapters por plataforma;
- SQLite com migrations, WAL, backup/restore e scanner de persistência;
- health composto, status, sessão e CSRF;
- logging JSONL allowlisted, redigido e rotativo;
- frontend React embarcado no binário e fallback History API protegido;
- OpenTelemetry desligado e sem exporter por padrão.

Validações aceitas incluem Go test/race/vet/build, frontend, smoke sem Node em
runtime, seis cross-builds e workflow nativo macOS/Windows.

### 4.4 Fase 4 — Kubernetes e RBAC, 58/59

Implementado e validado localmente:

- leitura de kubeconfig com precedência explícita e nenhum fallback silencioso;
- múltiplos arquivos, fingerprints e invalidação segura de client cache;
- plugins `exec` tratados como dependência externa e erros sanitizados;
- profiles e seleção de contextos com geração monotônica;
- escopos `single`, `list` e `all`, importação em lote e optimistic concurrency;
- `all` restrito ao catálogo de namespaces realmente retornado pela API;
- SAR exato, SSRR apenas como dica, cache curto e capability tri-state;
- matriz de permissões no frontend.

O E2E real fechou F4-50: revogação entre capability e operação preservou a API
Kubernetes como autoridade, distinguindo negação 403 de autorização
indisponível/no-opinion 503.

Pendente: F4-49, matriz exaustiva de gramática, limites, seleção, concorrência e
estados parciais que excede os cenários dinâmicos agora executados.

### 4.5 Fase 5 — Dashboard, 62/62

Implementado e validado localmente:

- resumo e blocos independentes;
- Pods problemáticos e restarts de containers regulares, init e ephemeral;
- Deployments, StatefulSets, DaemonSets, Jobs e CronJobs degradados;
- Events `Warning` agrupados com contagem e timestamp canônico;
- scan de possíveis erros em logs com budgets, concorrência, cancelamento e
  redaction;
- Metrics API opcional e degradação isolada.

F5-59 foi fechada no Kind local: o dashboard parcialmente autorizado respondeu
HTTP 200, manteve os blocos permitidos e isolou as falhas negadas/indisponíveis.

### 4.6 Fase 6 — Recursos somente leitura, 67/67

Implementado e validado localmente:

- listas, detalhes e DTOs próprios para Workloads, Pods, Events, Services,
  Ingresses, EndpointSlices, ConfigMaps e Secrets;
- YAML sob demanda somente para recursos elegíveis;
- Secret estritamente metadata-only e sem rota de YAML;
- logs atuais, anteriores e follow;
- paginação composta, fan-out limitado, cursor opaco, expiração e generation
  fencing;
- LIST/watch, relist 410, fallback HTTP, replay e filas limitadas;
- preferências e filtros salvos por allowlist;
- páginas Workloads, Pods, Logs, Events, Network, Config e Settings.

F6-57 foi fechada pelo `app-e2e`: lista → detalhe → YAML/logs passou nos
caminhos permitidos e negados, incluindo `previous-log` repetível.

### 4.7 Fase 7 — Ações autorizadas, 47/47

Implementado e validado localmente:

- restart com patch mínimo;
- scale via subresource e preconditions;
- delete de Pod com UID/resourceVersion;
- port-forward somente em loopback, com owner, limite e cleanup;
- exec sem shell implícito, com argv, ticket one-shot e WebSocket endurecido;
- idempotência ligada a alvo/body/generation;
- confirmação contextual e capability tri-state no frontend;
- SAR imediatamente antes de toda mutação ou upgrade;
- auditoria allowlisted sem comando, saída ou payload sensível.

F7-44 foi fechada com a matriz real permitido/negado/revogado. Negação
autoritativa permaneceu 403 e estado de autorização não determinável falhou
fechado em 503.

### 4.8 Fase 8 — Distribuição, 43/50

Implementado localmente ou comprovado no toolchain histórico:

- GoReleaser v2, seis alvos e archives determinísticos;
- checksum SHA-256, allowlist exata de conteúdo e `trimpath`;
- instaladores Unix/PowerShell com instalação, upgrade, rollback, uninstall e
  purge;
- updater com versão exata, checksum, locks e troca segura por plataforma;
- workflows de verify/release separados, actions fixadas por SHA e privilégio
  de escrita somente no job de publicação;
- pin atual de linguagem Go 1.26.0 e toolchain exato Go 1.26.7;
- snapshot GoReleaser v2.17.1 atual gerando os seis archives e smoke do binário
  aprovado;
- F8-21–F8-26 comprovadas por `validate` e
  `app-e2e ./dist/kubePeep` no Kind local reutilizado;
- runtime nativo macOS/Windows e smoke dos seis archives no workflow #23 como
  evidência histórica do toolchain Go 1.25.13;
- documentação legal, instalação, remoção e troubleshooting.

Pendências exatas:

- F8-20: remover e recriar do zero o cluster Kind canônico; `create` reutilizou
  a instância existente nesta execução;
- F8-34, F8-36 e F8-41: repetir instalador/updater e os seis smokes nativos com
  Go 1.26.7; o run #23 não valida os novos binários;
- F8-42: instaladores contra candidate imutável;
- F8-46: fechamento integral da matriz MVP e gates complementares;
- F8-48: comandos publicados e casing dos assets contra candidate real.

### 4.9 Fase 9 — Experiência operacional, 12/84

Fechado e versionado:

- benchmark oficial e limite de não-infringimento;
- inventário inicial de facilitadores;
- catálogo tipado de dez destinos;
- paleta `Ctrl/Cmd+K` somente de navegação;
- teclado, focus trap, retorno de foco e ajuda de atalhos;
- ausência de mutações, fetch próprio ou persistência pela paleta;
- threat-model delta;
- critério UX-M02.

Validado localmente no working tree, ainda dependente de commit/CI:

- controles comuns de lista com estado `draft` e `applied`;
- nenhuma nova consulta antes de `Apply filters`;
- reset explícito e resumo acessível de filtros ativos;
- ordenação enviada ao backend e rotulada honestamente como ordenação de página
  limitada, não como ordenação global;
- estado independente por aba em Network e Config;
- filtros salvos sanitizados, sem cursor, corpo de recurso, YAML ou log;
- `problematic=false` preservado como filtro explícito;
- cursores vinculados à geração;
- ordenação natural `item-2` antes de `item-10`, estabilidade e tie-breaker por
  identidade canônica nas coleções de contexto único.

F9-19 e F9-20 já estão marcadas. F9-21 permanece **aberta** mesmo com o
comparador natural implementado: o requisito também exige origem explícita em
agregações, e a futura coleção multi-contexto ainda precisa incluir essa origem
no desempate determinístico. UX-M03 permanece aberto porque inclui ainda
filtros positivos/negativos/multitermo, colunas allowlisted e alta
cardinalidade.

## 5. Arquitetura e capacidades atuais

### 5.1 Visão de execução

```text
Browser
  │ HTTP JSON / SSE / WebSocket em loopback
  ▼
processo único kubePeep
  ├─ Cobra e lifecycle próprio
  ├─ Ginger router, middlewares, response, errors e health contracts
  ├─ handlers e DTOs próprios
  ├─ services de aplicação e geração
  ├─ adapters SQLite/filesystem/runtime/browser
  ├─ client-go / Metrics API / SPDY
  └─ SPA React embutida
        │
        ├─ SQLite local allowlisted
        └─ Kubernetes API, autoridade de dados e RBAC
```

### 5.2 Stack fixada

| Camada | Tecnologia/versão |
| --- | --- |
| Linguagem backend | Go 1.26.0; toolchain 1.26.7 |
| Framework principal | Ginger v1.4.4 |
| CLI | Cobra v1.10.2 |
| Kubernetes | client-go/API/apimachinery/metrics v0.35.7 |
| Banco | modernc SQLite v1.54.0, CGO desativável |
| WebSocket local de exec | coder/websocket v1.8.15 |
| Frontend | React 19.2.8, React Router 8.3.0, TanStack Query 5.101.4 |
| Build frontend | TypeScript 6.0.3, Vite 8.2.0, npm 11.16.0 |
| E2E | Playwright 1.62.1 |
| Distribuição | GoReleaser v2.17.1 |

### 5.3 Fronteiras de código

| Área | Responsabilidade |
| --- | --- |
| `cmd/kubePeep/` | entrypoint compilável |
| `internal/cli/` | comandos e experiência CLI |
| `internal/lifecycle/`, `internal/runtime/` | listener, prontidão, estado, controle e cleanup |
| `internal/app/` | composição Ginger, rotas comuns/raw e embed |
| `internal/api/handlers/` | contratos HTTP, DTOs e handlers |
| `internal/api/middlewares/` | Host, Origin, CSRF, recovery e raw chain |
| `internal/services/` | autorização, seleção, dashboard, recursos e ações |
| `internal/adapters/` | filesystem, SQLite, Kubernetes e plataforma |
| `internal/integration/kubernetesruntime/` | operações concretas client-go/SPDY |
| `internal/migrations/` | schema SQLite embutido |
| `internal/web/` | assets Vite embutidos no binário |
| `web/src/` | shell, páginas, API client e componentes React |
| `test/kind/` | cluster, RBAC, fixtures e E2E canônico |
| `scripts/` | segurança, instaladores e smoke |

### 5.4 Fluxos arquiteturais críticos

1. A seleção ativa produz uma geração monotônica.
2. Queries, cursores, watches, logs, port-forwards e exec pertencem à geração.
3. Troca de contexto/escopo cancela a geração anterior.
4. O frontend descarta respostas obsoletas mesmo se o cancelamento remoto
   chegar tarde.
5. Capability na UI é apenas informativa.
6. O backend repete SAR e a operação Kubernetes continua sendo a autoridade.
7. Falhas parciais ficam ligadas ao bloco, namespace, recurso ou origem que
   falhou; não apagam resultados válidos dos demais.

## 6. Inventário das superfícies

### 6.1 Rotas do frontend

O catálogo versionado contém exatamente dez destinos de navegação:

| Path | Superfície |
| --- | --- |
| `/` | Overview/dashboard |
| `/workloads` | Workloads |
| `/pods` | Pods |
| `/logs` | Logs |
| `/events` | Events |
| `/network` | Services, Ingresses e EndpointSlices |
| `/config` | ConfigMaps e Secrets metadata-only |
| `/namespaces` | editor de escopos |
| `/permissions` | matriz de permissões |
| `/settings` | preferências allowlisted |

Qualquer outro path recebe estado de página não encontrada. O fallback History
API serve a SPA somente para GET/HEAD que aceitam HTML e nunca captura API,
health ou canal de controle.

### 6.2 API local

Todas as rotas abaixo usam o prefixo `/api/v1`, salvo `/health` e o canal
interno de controle.

| Domínio | Métodos e rotas |
| --- | --- |
| Saúde/sessão | `GET /health`, `GET /status`, `GET /session` |
| Profiles/contextos | `GET /cluster/profiles`, `GET /cluster/profile`, `GET /contexts`, `POST /contexts/select` |
| Namespaces/scopes | `GET /namespaces`, CRUD de `/namespace-scopes`, `POST /namespace-scopes/validate`, `POST /namespace-scopes/{id}/select` |
| Permissões | `GET /permissions` |
| Dashboard | `GET /dashboard/summary`, `/problems`, `/restarts`, `/events`, `GET /metrics`, `POST /dashboard/log-scan` |
| Workloads | lista, detalhe, YAML, restart e scale |
| Pods | lista, detalhe, YAML, logs, delete, port-forward e ticket de exec |
| Events | lista paginada |
| Network | listas/detalhes/YAML de Services, Ingresses e EndpointSlices |
| Config | listas/detalhes de ConfigMaps/Secrets e YAML somente de ConfigMap |
| Preferências | `GET/PUT /preferences` |
| Sessões | `GET/DELETE /port-forwards` por coleção/ID |
| Streams raw | follow de logs, stream de recursos e stream WebSocket de exec |

O canal `status`/`stop` usado pela CLI é separado da API de produto, exige
token local privado e prova da identidade completa da instância.

### 6.3 Dados Kubernetes modelados

| Classe | Conteúdo entregue |
| --- | --- |
| Workloads | identidade, status, réplicas, condições e relações allowlisted |
| Pods | identidade, phase, condições, restarts, containers e relações allowlisted |
| Events | tipo, razão, mensagem sanitizada, contagem e timestamps |
| Network | campos compactos de Service, Ingress e EndpointSlice |
| ConfigMap | metadata no catálogo; detalhe/YAML somente sob demanda e autorização |
| Secret | metadata allowlisted; nenhum valor e nenhuma rota YAML |
| Logs | stream/resultado transitório, limitado e redigido |
| Métricas | CPU/memória opcionais, nunca checker crítico do processo local |
| Capabilities | tri-state, chaveada por origem/recurso/verbo e generation |

## 7. Persistência e classificação de dados

### 7.1 Schema SQLite atual

| Tabela | Finalidade | Dados proibidos |
| --- | --- | --- |
| `schema_migrations` | versão/checksum das migrations | conteúdo remoto |
| `cluster_profiles` | nome local, contexto escolhido e default | credencial/rest.Config |
| `cluster_profile_kubeconfig_files` | referências ordenadas de paths | conteúdo dos arquivos |
| `namespace_scopes` | agregado local e versão | snapshot Kubernetes |
| `namespace_scope_items` | nomes de namespaces allowlisted | `*` como item |
| `preferences` | chaves e JSON por schema fechado | chave arbitrária ou payload sensível |

As preferências atuais abrangem idioma, opções de logs, organização do
dashboard e conjuntos de filtros aprovados. Cada valor tem schema e limite de
tamanho. Não existe endpoint de chave arbitrária.

### 7.2 Estado somente em memória

- rest configs, clients e cache de autorização;
- geração e contextos de cancelamento;
- nonce CSRF e tickets WebSocket;
- cursores e replay rings;
- resultados Kubernetes, YAML permitido, logs e métricas;
- sessões de watch, SSE, port-forward e exec;
- rascunhos de filtros e cache TanStack Query.

### 7.3 Dados que nunca podem persistir ou ir ao repositório

- credenciais, tokens, certificados, chaves privadas e kubeconfigs completos;
- conteúdo de Secret ou referência expandida de Secret;
- logs, YAML remoto, objetos Kubernetes serializados ou diffs;
- comando e saída de exec ou tráfego de port-forward;
- headers de autorização, erro bruto de plugin e endpoint privado;
- PII privada, paths da máquina, bancos, dumps, logs e artefatos gerados.

O frontend não usa persister do TanStack Query, service worker, Cache API,
IndexedDB, localStorage ou sessionStorage para dados do cluster.

## 8. Decisões arquiteturais

### ADR 0001 — Bootstrap híbrido CLI e serviço Ginger

Aceita Ginger `service` como base, Cobra como owner do processo e lifecycle
HTTP próprio. Rejeita `app.Run()` por incompatibilidade com listener adquirido,
readiness, streams longos e cleanup determinístico.

### ADR 0002 — Saúde local e dependências degradadas

Separa saúde do processo/SQLite de kubeconfig, contexto, cluster e Metrics API.
Dependência externa degradada pode manter HTTP 200; falha local crítica produz
503. Mensagens públicas são estáveis e allowlisted.

### ADR 0003 — Protocolos de streaming

Usa SSE para fluxo unidirecional e coder/websocket para exec. Rotas raw mantêm
interfaces de streaming e reaplicam guards de segurança. Tickets de exec são
one-shot, curtos e ligados ao init canônico.

### ADR 0004 — Runtime local e lifecycle

Define foreground, lock real, identidade de instância, bind por listener,
readiness antes do browser, canal autenticado e cleanup LIFO multiplataforma.
PID isolado nunca é autoridade.

### Decisões transversais ainda sem novo ADR numerado

- Secret permanece metadata-only;
- multi-contexto futuro é somente leitura e mantém proveniência por origem;
- ordenação de coleção paginada é descrita como página limitada;
- ausência de opinião do authorizer é `unknown`, não `denied` inventado;
- benchmark Aptakube orienta facilitadores, nunca identidade ou código.

O registro de ADR específico do mecanismo de grafo estava vazio na captura. Os
quatro arquivos versionados em `docs/decisions/` continuam sendo a fonte
canônica; sincronizar o registro auxiliar pode ser feito no fechamento F9-84,
sem substituir os documentos Git.

## 9. Benchmark Aptakube e não-infringimento

As fontes oficiais do Aptakube foram usadas para descobrir problemas de
usabilidade e facilitadores. Não foram usadas como blueprint visual nem fonte
de código. O Kube Peep mantém nomes, componentes, DTOs, rotas, estilos e
identidade próprios.

| Facilitador observado | Adaptação Kube Peep | Limite adicional |
| --- | --- | --- |
| Entrada por kubeconfig | usa configuração já existente | somente leitura; não modifica nem copia credencial |
| Paleta/teclado | catálogo local de rotas | primeira versão não executa mutação nem indexa recurso remoto |
| Listas/filtros | estado explícito e query no backend | reset visível, allowlist e escopo de página honesto |
| Ordenação natural | comparador estável no backend | tie-breaker de identidade e cursor independente |
| Favoritos/recentes | requisito futuro | apenas referências mínimas, limitadas e removíveis |
| Visão humana | DTOs compactos e status reais | nenhum diagnóstico inventado |
| Ações rápidas | requisito F9 sobre ações existentes | capability informativa; SAR repetido no backend |
| YAML | visualização futura aprimorada | Secret sempre recusado; conteúdo apenas em memória |
| Diff | requisito somente leitura | origens explícitas, normalização opt-in e recusa de Secret |
| Logs agregados | requisito com múltiplas fontes | proveniência por linha, budgets e zero persistência |
| Métricas | bloco opcional | falha isolada, sem afetar health local |
| Port-forward | gerenciador futuro sobre serviço existente | loopback obrigatório, owner, limite e stop explícito |
| Multi-cluster | fan-out futuro somente leitura | capabilities nunca se misturam; mutação exige alvo único |

Desvios deliberados incluem não expandir referências de Secret, não copiar
trade dress, não habilitar telemetria externa por padrão, não instalar agente no
cluster e não transformar conveniência em autorização.

## 10. Validações executadas

### 10.1 Gates locais consolidados do MVP

| Gate | Evidência acumulada |
| --- | --- |
| Go | toolchain atual Go 1.26.7; suíte focada do Kubernetes runtime passou 48/48 normal e 48/48 com race |
| Vulnerabilidades Go | `govulncheck` executado com Go 1.26.7 sem vulnerabilidades alcançáveis |
| Frontend | npm audit, lint, typecheck, Vitest, build e Playwright passaram |
| Ginger | `version`, `inspect` e `doctor` executados; diagnósticos heurísticos documentados |
| Runtime | smoke de start/status/health/stop e cleanup sem Node em runtime |
| Distribuição | GoReleaser v2.17.1 gerou snapshot Go 1.26.7 com seis archives; smoke do binário passou |
| Instalador Unix | instalação, upgrade, checksum, rollback, uninstall e purge |
| Segurança | scanner de staged/history, identidades, paths, mensagens e Gitleaks |
| Kind | gates estáticos, `create` com reutilização, `validate` e `app-e2e ./dist/kubePeep` passaram |

### 10.2 Evidência nativa

- Fase 1: probe de controle executado nativamente em Linux e Windows amd64;
- Fase 3: workflow #9 passou build/test e runtime nativo macOS/Windows;
- Fase 8: workflow #23 passou runtime nativo macOS/Windows com Go 1.25.13;
- Fase 8: os seis archives históricos foram verificados e executados nos
  runners de suas combinações Linux/macOS/Windows × amd64/arm64;
- instalador/updater PowerShell passaram no runner Windows do workflow #23.

As três evidências da Fase 8 acima são históricas. Elas não validam o binário,
instaladores ou archives reconstruídos com Go 1.26.7; por isso F8-34, F8-36 e
F8-41 permanecem reabertas até nova CI nativa.

### 10.3 Resultado histórico do workflow #23

O run foi executado com Go 1.25.13, antes do upgrade do projeto para Go 1.26.7.

| Job | Resultado |
| --- | --- |
| `build-and-test` | passou |
| `native-runtime (macos-latest)` | passou |
| `native-runtime (windows-latest)` | passou |
| `release-snapshot` | passou |
| seis `native-archive-smoke` | passaram |
| `restricted-kind` | falhou |

O resultado documenta o comportamento do HEAD executado, mas não fecha os
gates do toolchain atual. A evidência local posterior fechou F4-50, F5-59,
F6-57, F7-44 e F8-21–F8-26; F4-49, F8-20, F8-34, F8-36 e F8-41 permanecem
abertas pelos motivos registrados neste relatório.

### 10.4 Working tree da segunda fatia F9

Resultados locais já observados para os controles de lista:

| Gate | Resultado |
| --- | --- |
| ESLint | passou |
| TypeScript | passou sem emissão |
| Vitest | 17 arquivos, 73/73 testes |
| Vite build | passou; 1.904 módulos transformados |
| Playwright Chromium | 3/3 cenários |
| Storage browser nos cenários | permaneceu vazio |
| Go — Kubernetes runtime | 48/48 testes focados passaram |
| Go — Kubernetes runtime com race | 48/48 testes focados passaram |
| `govulncheck` | Go 1.26.7, nenhuma vulnerabilidade alcançável |

A ordenação natural recebeu testes normais e race no pacote Kubernetes runtime.
Ainda se exigem o security gate, commit e workflow do commit que contiver a
mudança. Esses gates também não bastam para F9-21: a origem multi-contexto
continua sem implementação.

### 10.5 Kind: diagnóstico e evidência local

O cenário de revogação periódica combinava uma leitura direta negada com uma
expectativa de produto HTTP 403. Em Kubernetes, um authorizer sem opinião pode
fazer o SelfSubjectAccessReview resultar incompleto; o contrato do produto
converte isso em capability `unknown` e erro público
`AUTHORIZATION_UNAVAILABLE` com HTTP 503. Transformar esse caso em 403 faria o
produto inventar uma negação.

O working tree atual:

- mantém a prova direta de que a identidade restrita não consegue ler o Pod;
- usa uma identidade sem concessão alternativa para essa prova;
- espera 503 + código público estável na leitura feita pelo produto;
- acrescenta uma anotação CI adicional somente com `stage` e `status`.

`restricted_stage` não captura, filtra nem substitui a saída dos comandos. Cada
comando continua no log normal do job; portanto, a ausência de body, path local,
stack trace ou dado sensível depende da sanitização interna do harness e dos
programas que ele chama. A anotação estática reduz o conteúdo do resumo de
erro, mas não torna o log inteiro automaticamente sanitizado.

Depois de tornar idempotentes, com ownership estrito, delete condicionado à
UID e recuperação canônica armada antes da remoção, o Pod usado em `previous-log` e o Event
`000-kp-warning`, `create` reutilizou o cluster, `validate` passou e
`app-e2e ./dist/kubePeep` passou. A execução comprovou:

- HTTP 503 para autorização indisponível/no-opinion, sem inventar 403;
- HTTP 403 para negação autoritativa;
- HTTP 200 no dashboard parcialmente autorizado, com falhas isoladas;
- caminhos permitidos/negados de recursos e ações;
- DELETE negado com UID precondicionada, `Forbidden` autoritativo e a mesma UID
  preservada; nenhuma rota destrutiva opera sobre objeto sem ownership;
- convergência de recuperação diante de sinal ou resposta de DELETE ambígua,
  sem `apply` sobre um homônimo surgido durante a restauração;
- inspeção negativa de persistência prevista em F8-26.

Essa evidência fecha F8-21–F8-26 e as tarefas correlatas F4-50, F5-59, F6-57 e
F7-44. Não fecha F8-20 porque o cluster não foi recriado do zero, nem substitui
o job `restricted-kind` da CI atual.

## 11. Segurança

### 11.1 Premissa permanente

Nunca subir dados sensíveis é requisito de projeto, não recomendação. Antes de
todo commit e push deve rodar:

```sh
rtk ./scripts/security_check.sh HEAD
```

O gate cobre conteúdo staged, histórico alcançável, identidades GitHub noreply,
mensagens, tags, nomes de arquivos arriscados, paths de máquina e Gitleaks com
redaction. Hooks de pre-commit/pre-push e workflows usam o mesmo controle.

### 11.2 Estado do repositório remoto

- após fetch/prune, o único ref remoto observado foi `origin/main` e não havia
  tag local ou remota;
- `scripts/security_check.sh origin/main` passou sobre os 25 commits
  alcançáveis;
- nesse escopo não há evidência de segredo remoto nem ref que precise ser
  reescrito;
- o histórico alcançável da branch principal foi reescrito/corrigido quando a
  política de segurança foi introduzida;
- a auditoria registrada não encontrou credencial real no histórico remoto
  alcançável atual;
- fixtures aprovadas são sintéticas ou valores públicos exatos com exceção
  estreita e justificada;
- nenhum achado deve ser reproduzido em issue, commit, relatório ou log.

Limite importante: Git não prova a remoção de objetos órfãos, caches ou logs já
retidos pelo provedor. Se qualquer objeto antigo continuar acessível, a ação
correta é revogar/rotacionar primeiro, reescrever todos os refs afetados,
validar novamente e acionar o suporte autenticado do provedor. Até essa
confirmação externa, não se deve afirmar “purga completa do provedor”.

### 11.3 RBAC e fail-closed

- Kubernetes é a autoridade final;
- SSRR pode otimizar apresentação, mas nunca conceder operação;
- SAR usa namespace, group, resource, subresource, verb e resourceName;
- `unknown` permanece distinto de `denied`;
- mutação e upgrade repetem autorização imediatamente antes do efeito;
- resposta real 403 da API continua autoritativa;
- troca de geração cancela trabalho e invalida estado antigo.

### 11.4 Browser e API local

- bind somente em loopback;
- Host exato e mitigação de DNS rebinding;
- Origin e CSRF em rotas mutáveis;
- sem CORS aberto;
- CSP e headers defensivos;
- resposta Kubernetes/log/permissão com `no-store`;
- cadeia raw própria para SSE/WS preservando streaming sem remover guards.

### 11.5 Secret e dados transitórios

- Secret possui apenas DTO metadata-only;
- não existe YAML, diff, coluna arbitrária, favorito ou busca de valor de Secret;
- logs, YAML permitido, diff futuro e terminal ficam apenas em memória;
- cópia/download exige gesto explícito e não cria cópia interna;
- erros públicos usam código estável e mensagem allowlisted;
- log local nunca recebe corpo remoto, token, comando ou saída de exec.

### 11.6 Supply chain e release

- dependências e toolchains são fixados;
- actions externas usam SHA completo;
- downloads de Kind/kubectl usam versão e checksum fixos;
- archives têm allowlist e checksum;
- instaladores recusam versão/candidate mutável;
- somente o workflow de release possui permissão de escrita;
- nenhuma tag deve ser criada sem autorização e candidate explicitamente
  aprovadas.

## 12. Matrizes de aceite

### 12.1 MVP, 25/27

A evidência Kind local fecha MVP-10 e MVP-21, elevando a matriz para 25/27. Há
uma ressalva de validade temporal: provas nativas de runtime/archives do run #23
foram obtidas com Go 1.25.13 e não demonstram o toolchain atual Go 1.26.7. Essa
reabertura afeta gates complementares e F8-34/F8-36/F8-41, enquanto MVP-23
permanece aberto até a pipeline integral do estado atual.

Critérios ainda abertos:

| ID | Motivo |
| --- | --- |
| MVP-23 | pipeline do estado final precisa ficar integralmente verde |
| MVP-26 | checksum de instaladores precisa da candidate/publicação aplicável |

Além dos 27 critérios, gates complementares ainda exigem recriação Kind do
zero, CI nativa Go 1.26.7 e comandos publicados contra artefatos finais.

### 12.2 UX, 1/15

| Critério | Estado |
| --- | --- |
| UX-M02 | fechado: paleta/atalhos de navegação seguros |
| UX-M03 | parcial: estado/filtros/reset e natural sort em evolução; parser composto e alta cardinalidade pendentes |
| UX-M01 e UX-M04–M15 | abertos |

Os itens futuros UX-P01–P04 — edição YAML, ações especializadas, adapters de CRD
e exportação avançada — não pertencem ao gate F9 e não devem aparecer como
promessa de produto.

## 13. Divergências encontradas e reconciliadas

| Divergência | Tratamento |
| --- | --- |
| documentos antigos tratavam workflow parcial como gate amplo | run #23 foi registrado como histórico, 10/11, Go 1.25.13 e sem fechar Kind |
| F8 podia reutilizar prova de toolchain anterior | F8-34, F8-36 e F8-41 foram reabertas; nova CI Go 1.26.7 é obrigatória |
| Kind local estava sem evidência positiva | `validate` e `app-e2e` passaram, fechando F8-21–26; F8-20 segue aberta porque `create` reutilizou o cluster |
| F4–F7 podiam parecer incompletas em código | F4-50, F5-59, F6-57 e F7-44 receberam a evidência real; somente F4-49 permanece aberta |
| revogação sem opinião esperava 403 | contrato corrigido para `unknown`/503 sem inventar negação |
| UI podia sugerir ordenação global | wording e contrato dizem “página limitada” |
| digitação alterava query imediatamente | `draft` e `applied` foram separados |
| cursor podia sobreviver visualmente à troca | cursor foi ligado à generation |
| estado Network/Config podia se misturar entre abas | cada tipo mantém estado/cursor independentes |
| ordenação lexicográfica não era natural | comparador natural estável foi implementado para contexto único; F9-21 segue aberta sem origem multi-contexto no desempate |
| registro auxiliar de ADR do grafo está vazio | ADRs Git são canônicos; sincronização fica para o reindex final |

F9-21 não deve mudar a contagem de 12/84 enquanto a agregação multi-contexto não
tiver origem explícita e desempate determinístico. Quando essa parte for
implementada e validada, atualizar `plan/09-experiencia-operacional.md`,
`plan/README.md`, a evidência F9 e este relatório no mesmo commit.

## 14. Reindexação do projeto

### 14.1 Índice observado antes do fechamento final

O quadro abaixo é somente a baseline anterior à nova evidência local. Ele não é
apresentado como reindex final do working tree atual:

| Métrica | Valor observado |
| --- | ---: |
| Nodes | 5.990 |
| Edges | 32.246 |
| File nodes | 396 |
| Linguagens | 10 |
| Routes no grafo | 98, incluindo rotas de produto, raw e testes |
| Arquivos skipped por falha | 0 |
| Arquivos `parse_partial` | 4 |

`parse_partial` conhecido:

| Arquivo | Faixas |
| --- | --- |
| `internal/migrations/sql/0001_initial.sql` | 39–41, 68–116 |
| `scripts/install_test.ps1` | 265 |
| `test/kind/harness.sh` | 786 |
| `web/src/api/client.ts` | 58 |

Essas faixas foram ou devem ser lidas diretamente; cobertura do grafo é sinal
best-effort e não prova completude. Diretórios de build, dependências, Git e
configurações locais ignoradas permanecem fora do índice por desenho.

### 14.2 Métricas a preencher após estabilizar e versionar

| Campo final | Valor |
| --- | --- |
| Commit reindexado | `A preencher após o commit funcional` |
| Modo | `full`, sem persistir artefato do índice no repositório |
| Nodes | `A preencher` |
| Edges | `A preencher` |
| Arquivos skipped | `A preencher` |
| Arquivos `parse_partial` | `A preencher` |
| Mudanças detectadas | `A preencher` |
| Blast radius inbound | `A preencher` |
| Módulos impactados | `A preencher` |
| Cobertura dos arquivos alterados | `A preencher` |
| Estado do watcher/metadata | `A preencher` |

Sequência de fechamento F9-84:

1. terminar testes e documentação;
2. executar security gate;
3. versionar o mesmo estado que será indexado;
4. executar indexação `full` com persistência desativada;
5. consultar status e cobertura dos paths alterados;
6. ler diretamente todas as faixas parciais;
7. executar detecção de mudanças/blast radius contra a base adequada;
8. substituir os placeholders acima por números e conclusões sanitizados;
9. registrar as métricas finais neste relatório e nas evidências versionadas.

## 15. Pendências priorizadas

### P0 — impedir regressão e vazamento

- rodar `scripts/security_check.sh HEAD` antes de commit e novamente antes de
  push;
- preservar identidade GitHub noreply;
- nunca copiar log bruto do Kind/Windows para documentação; manter a
  sanitização dentro do harness porque a anotação `restricted_stage` não filtra
  a saída normal dos comandos;
- manter Secret metadata-only e storage browser vazio;
- não criar tag/release sem autorização explícita.

### P1 — fechar Kind

- executar `create` em ambiente comprovadamente sem o cluster canônico para
  fechar F8-20;
- preservar a idempotência e as precondições de UID do Pod `previous-log`, do
  Event `000-kp-warning` e das RoleBindings temporariamente revogadas;
- repetir o workflow e exigir `restricted-kind` verde com Go 1.26.7;
- manter 503 para no-opinion, 403 para negação e dashboard parcial 200.

### P2 — fechar distribuição

- revalidar F8-34, F8-36 e F8-41 na CI nativa atual;
- produzir candidate imutável somente quando autorizada;
- executar instaladores contra essa candidate e checksum;
- testar comandos publicados e casing dos assets;
- completar MVP-26, F8-42, F8-46 e F8-48;
- manter a ressalva provider-side até confirmação externa aplicável.

### P3 — continuar Fase 9

- manter F9-21 aberta e implementar origem multi-contexto no desempate antes de
  considerar seu fechamento;
- implementar parser positivo/negativo/multitermo e alta cardinalidade;
- finalizar atalhos seguros e deep links;
- especificar persistência mínima de favoritos/recentes;
- evoluir visões humanas e ações rápidas;
- adicionar viewer YAML e diff somente leitura;
- aprimorar logs/métricas e gerenciador de port-forward;
- desenhar multi-contexto somente leitura com proveniência e isolamento;
- executar acessibilidade, inspeções negativas, Kind, archives e CI finais;
- reindexar e atualizar toda a documentação no mesmo estado versionado.

### P4 — coordenação externa

- usar suporte autenticado do provedor se objetos antigos/caches continuarem
  acessíveis;
- não registrar identificadores de objetos antigos ou conteúdo afetado em
  documentação pública;
- rotacionar/revogar primeiro se algum segredo real for descoberto no futuro.

## 16. Reprodução segura

Os comandos abaixo não incluem paths de máquina, credenciais ou endpoints
privados. Executá-los a partir da raiz do clone.

### 16.1 Verificação local

```sh
rtk go mod tidy
rtk go mod verify
rtk go vet ./...
rtk go test -count=1 ./...
rtk go test -count=1 -race ./internal/...
rtk go build -trimpath -o dist/kubePeep ./cmd/kubePeep

rtk npm --prefix web ci
rtk npm --prefix web audit --audit-level=high
rtk npm --prefix web run lint
rtk npm --prefix web run typecheck
rtk npm --prefix web test
rtk npm --prefix web run build
rtk npm --prefix web run test:e2e
```

### 16.2 Distribuição e segurança

```sh
rtk ./scripts/security_check.sh HEAD
rtk ./scripts/install_test.sh
rtk go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
rtk goreleaser check
```

### 16.3 Kind canônico

```sh
rtk ./test/kind/harness.sh static
rtk ./test/kind/harness.sh create
rtk ./test/kind/harness.sh validate
rtk ./test/kind/harness.sh kubeconfigs
rtk ./test/kind/harness.sh app-e2e ./dist/kubePeep
```

Não publicar kubeconfig, saída do cluster, bodies, eventos, logs ou diagnósticos
brutos ao registrar o resultado. Documentar somente estágio, classe pública,
status, commit alcançável e conclusão.

## 17. Inventário do working tree na captura

Mudanças concorrentes relacionadas à continuação atual, todas com paths
relativos:

### CI e Kind

- `.github/workflows/release.yml`;
- `.github/workflows/verify.yml`;
- `.gitignore`;
- `test/kind/app_e2e.py`;
- `test/kind/harness.sh`;
- `test/kind/rbac.yaml`;
- `test/kind/README.md`.

### Toolchain e runtime nativo

- `go.mod`;
- `internal/cli/native_process_test.go`.

### Ordenação backend

- `internal/integration/kubernetesruntime/resources_backend.go`;
- `internal/integration/kubernetesruntime/resources_backend_test.go`;
- `internal/integration/kubernetesruntime/resources_sort.go`;
- `internal/integration/kubernetesruntime/resources_sort_test.go`.

### Interface de listas

- `web/src/components/ResourcePages.tsx`;
- `web/src/components/ResourcePages.test.tsx`;
- `web/src/components/ResourceListControls.tsx`;
- `web/src/components/ResourceListControls.test.tsx`;
- `web/src/styles.css`;
- `web/e2e/app.spec.ts`.

### Documentação reconciliada

- `README.md`;
- `docs/research/phase4-evidence.md`;
- `docs/research/phase5-evidence.md`;
- `docs/research/phase6-evidence.md`;
- `docs/research/phase7-evidence.md`;
- `docs/research/phase8-evidence.md`;
- `docs/research/phase9-evidence.md`;
- `plan/04-kubernetes-rbac.md`;
- `plan/05-dashboard.md`;
- `plan/06-recursos.md`;
- `plan/07-acoes.md`;
- `plan/08-distribuicao.md`;
- `plan/09-experiencia-operacional.md`;
- `plan/README.md`;
- `plan/matriz-aceite-mvp.md`;
- este relatório.

## 18. Critério de atualização deste relatório

Atualizar este documento somente com evidência do mesmo estado versionado:

- substituir HEAD e métricas após commit/reindex;
- alterar contagens ao marcar/desmarcar tarefas no plano;
- registrar CI somente depois do workflow correspondente terminar;
- nunca converter falha parcial em aceite positivo;
- não reutilizar run anterior para código novo;
- não incluir segredo, PII privada, e-mail, path de máquina, identificador de
  objeto órfão, log bruto ou payload Kubernetes;
- reabrir critérios quando uma mudança invalidar a prova existente.

O estado final do projeto só pode ser declarado concluído quando Fases 1–9,
matrizes MVP/UX, Kind restritivo, inspeções negativas, archives nativos,
candidate aplicável, CI do commit final e reindexação possuírem evidência
coerente e sanitizada.
