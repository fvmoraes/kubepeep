# Arquitetura do KubePeep

Arquitetura da base atual. O core é composto por `internal/application.Compose`
e compartilhado pelos runtimes web e desktop. O [plano v1](../plan/README.md)
controla a expansão funcional; [ADRs](decisions/README.md) preservam decisões
históricas. O [desktop](desktop-architecture.md) complementa o lifecycle web
abaixo com a janela Wails, bridge JSON e loopback de streaming.

## 1. Direcionadores

1. Ginger v1.4.4 é a camada principal de aplicação e HTTP.
2. Cobra expõe a experiência CLI.
3. Handlers não acessam clientsets, SQLite ou filesystem diretamente.
4. A API Kubernetes é a autoridade final para dados e autorização.
5. Todo trabalho remoto pertence a um contexto cancelável e a uma geração de seleção.
6. O produto funciona em loopback, em um único processo local e sem servidor de banco ou Node.js em runtime; desktop usa bibliotecas nativas.
7. Falhas parciais permanecem isoladas.
8. Streams e sessões têm owner, limite, cancelamento e cleanup.
9. Dados Kubernetes são convertidos para DTOs próprios.
10. Decisões que complementam Ginger são justificadas por spike e ADR.

## 2. Contexto do sistema

```text
┌──────────────────┐       loopback HTTP/SSE/WS       ┌──────────────────────┐
│ Browser do       │ <──────────────────────────────> │ Processo KubePeep   │
│ usuário          │                                  │ Cobra + Ginger       │
└──────────────────┘                                  └──────┬───────┬───────┘
                                                               │       │
                                          arquivos permitidos  │       │ client-go
                                                               │       │
                                                        ┌──────▼───┐ ┌─▼─────────────┐
                                                        │ SQLite / │ │ Kubernetes API │
                                                        │ runtime  │ │ e Metrics API  │
                                                        └──────────┘ └──────┬────────┘
                                                                           │
                                                              plugins exec │
                                                                           ▼
                                                                  processo externo
                                                                  do ambiente
```

### 2.1 Fronteiras de confiança

- **Browser ↔ API local:** fronteira não confiável; páginas externas podem tentar atingir loopback.
- **Processo ↔ filesystem:** paths e permissões precisam de validação por plataforma.
- **Processo ↔ kubeconfig/plugin `exec`:** entrada e erros podem conter segredos.
- **Processo ↔ Kubernetes API:** TLS e credenciais são controlados pelo kubeconfig; RBAC decide.
- **Processo ↔ SQLite/log local:** persistência é allowlisted e pode sobreviver ao processo.

As contramedidas estão em [security.md](security.md).

## 3. Containers e componentes

O produto possui dois componentes lógicos no mesmo artefato:

1. **Frontend React:** assets imutáveis embutidos, cliente HTTP e estado de apresentação.
2. **Backend Go:** CLI, lifecycle, HTTP, aplicação, adapters e persistência.

### 3.1 Componentes backend

```text
cmd/kubePeep
  └─ commands (Cobra)
      └─ bootstrap/lifecycle
          ├─ Ginger app/router/config/logger/response/errors/health
          ├─ API handlers + middleware
          ├─ application services
          ├─ ports
          ├─ adapters Kubernetes/SQLite/filesystem/browser/process
          ├─ session + watch registries
          └─ embedded web + migrations
```

A composição concreta fica em `internal/application`; os runtimes CLI e
desktop montam essa mesma plataforma. O scaffold Ginger foi referência
histórica, sem introduzir dependência do template CLI.

## 4. Regra hexagonal

Dependências apontam para dentro:

```text
HTTP/CLI/UI adapter
       │
       ▼
application service ───► port
                          ▲
                          │
                  infrastructure adapter
```

Regras verificáveis:

- Handler conhece DTO, serviço de aplicação e utilitários HTTP; não importa `kubernetes.Clientset`.
- Serviço de aplicação coordena ports e políticas; não importa router ou banco concreto.
- Adapter Kubernetes implementa ports e retorna modelos internos/DTO inputs, nunca objetos crus ao handler.
- Adapter SQLite não recebe objetos Kubernetes.
- Revalidação de ações usa o mesmo `AuthorizationService` compartilhado; não existe serviço paralelo.
- Frontend nunca é autoridade de autorização.

> **Implementado:** `internal/ports/architecture_test.go` falha se qualquer arquivo de `internal/api/handlers` importar `internal/adapters/...` ou `internal/integration/...`, e garante que o pacote `internal/ports` permaneça livre de dependências internas. Handlers dependem exclusivamente de interfaces de serviço e DTOs; a fiação concreta pertence a `internal/application.Compose` e aos runtimes (CLI/desktop).
>
> **Consolidação de classificadores (F4-02):** a classificação de workloads já é única (`resources.Convert*` → `dashboard.Classify*`), a redação de texto é única (`dashboard.Redact`) e as primitivas de pod agora vivem no pacote folha `internal/services/podhealth` — `ControllingOwner` (seleção do owner controller, antes triplicada em resources/dashboard/kubernetesruntime) e `Problematic` (definição canônica do badge "problem" das listas; o diagnóstico rico do dashboard permanece em `ClassifyProblemPod`, que é um refinamento com evidência/severidade e períodos de tolerância deliberadamente diferentes).

## 5. Ports

As interfaces Go vivem junto aos serviços que as consomem (idioma Go de "consumer-defined interfaces"); o pacote `internal/ports` é a fronteira documentada, mantido livre de dependências internas e vigiado pelo teste arquitetural acima. A semântica é definida aqui.

| Port | Responsabilidade | Entradas mínimas | Saídas/garantias |
| --- | --- | --- | --- |
| `KubeconfigLoader` | resolver conjunto ordenado de arquivos e contexto | flags, env, default, contexto | descritor sem credenciais e factory segura |
| `ContextService` | listar/selecionar contexto e criar geração | profile e nome | contexto ativo, cluster sanitizado, generation ID |
| `NamespaceService` | listar namespaces e gerenciar escopos | contexto, modo, entrada em massa | escopo validado e cobertura |
| `AuthorizationService` | capability tri-state e revalidação | contexto, namespace, group, resource, subresource, verb, resourceName | `allowed`, `denied` ou `unknown`, razão segura |
| `WorkloadService` | listar/detalhar/classificar workloads | escopo, filtros, cursor | DTOs, cursor, cobertura e falhas parciais |
| `PodService` | listar/detalhar/classificar pods | escopo, filtros, cursor | DTOs compactos e condições reais |
| `LogService` | logs atuais, anteriores, follow e scan limitado | alvo, limites, contexto | stream/resultado sanitizado, nunca persistido |
| `EventService` | listar/agrupar eventos | escopo, filtros, cursor | timestamps/count preservados |
| `NetworkService` | listar/detalhar Services, Ingresses e EndpointSlices | escopo, filtros, cursor | DTOs de rede, YAML autorizado e cobertura |
| `ConfigResourceService` | listar/detalhar ConfigMaps e metadata de Secrets | escopo, filtros, cursor | DTO allowlisted; Secret nunca vira objeto genérico |
| `MetricsService` | discovery e métricas opcionais | escopo e autorização | dados ou estado opcional indisponível |
| `PreferenceService` | ler/substituir preferências allowlisted | schema versionado e snapshot completo | transação local, defaults e rejeição de sensível |
| `ActionService` | confirmar, revalidar e executar restart/scale/delete | alvo, capability, precondition, idempotência | resultado aceito ou erro autoritativo |
| `PortForwardService` | criar/consultar/encerrar sessão | pod, portas, geração | sessão loopback registrada e cancelável |
| `ExecService` | executar protocolo remoto seguro | pod, container, argv, TTY | sessão bidirecional limitada e cancelável |
| `DashboardService` | orquestrar blocos independentes | seleção, budgets | blocos com `complete`, `truncated` e erros parciais |

Todos os métodos remotos recebem `context.Context`. Operações de lista recebem um objeto de seleção imutável contendo profile, contexto, escopo e generation ID.

## 6. Composição do processo

### 6.1 Responsabilidades do runtime web

O processo possui um coordenador de lifecycle, único owner de:

- lock de instância;
- listener efetivamente adquirido;
- arquivos de runtime;
- banco e migrations;
- servidor HTTP;
- registro de cancelamentos por geração;
- watch manager;
- sessões de stream, port-forward e `exec`;
- abertura do navegador;
- shutdown e cleanup.

### 6.2 Startup web

```text
parse CLI e resolver o data root canônico
  → criar/proteger o root e ler `config.yaml` estritamente
  → compor opções efetivas e adquirir lock
  → abrir SQLite e aplicar migrations embutidas
  → montar serviços/adapters/router
  → adquirir listener loopback por bind real
  → iniciar HTTP e comprovar `/health` na própria instância
  → gravar estado privado temporário com PID/porta/identidade
  → publicar `instance.json` por substituição atômica
  → abrir browser, se permitido
  → aguardar sinal/erro/comando de parada
```

Não se faz “descoberta de porta livre” separada do bind. A porta publicada é
obtida do listener adquirido. PID e porta permanecem apenas em memória até o
health local responder; ambos aparecem juntos no estado versionado, nunca em
arquivos parcialmente atualizados ou antes da prontidão.

### 6.3 Shutdown web

```text
cancelar root context
  → recusar novas sessões
  → cancelar geração ativa, watches e scans
  → encerrar port-forwards e exec
  → encerrar HTTP até deadline
  → forçar fechamento seguro do restante
  → fechar SQLite
  → remover arquivos transitórios/liberar lock
```

Cleanup deve rodar mesmo se o shutdown HTTP atingir timeout.

### 6.4 Decisão de lifecycle

Os ADRs 0001 e 0004 escolheram lifecycle HTTP próprio usando `app.New` e os
componentes Ginger, sem `app.Run()`/`OnStop`. Cobra é o único owner de sinais e
do contexto raiz; o modo inicial é foreground. O coordenador adquire o listener,
instala o mux externo mínimo de health, publica prontidão e executa seu registro
LIFO de cleanup em cancelamento, erro de Serve e timeout.

O módulo isolado `spikes/phase1/` reproduz a decisão histórica. A produção
usa `internal/runtime`; validar seu comportamento e os arquivos empacotados
pelos testes atuais, sem considerar resultados antigos prova de uma release.

## 7. Contrato operacional CLI

| Comando | Comportamento |
| --- | --- |
| `kubePeep` | janela em build desktop; runtime web em build CLI |
| `serve` / `start` | runtime web em foreground |
| `stop` | controle autenticado da instância web; verifica identidade antes de cancelar |
| `status` | consulta autenticada da instância web e detecção de estado obsoleto |
| `version` | versão, commit e build date |
| `doctor` | checks sanitizados de aplicação, filesystem, SQLite e Kubernetes |
| `update --version X.Y.Z` | download explícito, SHA-256 e substituição com rollback |

O endpoint interno usado pelos comandos `status` e `stop` é distinto de
`/api/v1/status`: ambos os comandos de controle apresentam o token privado e
exigem a mesma prova de identidade. Os harnesses de distribuição validam os comandos de produção dentro dos
archives e instaladores reais.

Flags de contexto, kubeconfig e namespace alimentam o bootstrap Kubernetes. A precedência de source é `--kubeconfig`, lista
ordenada de `KUBECONFIG`, profile persistido `is_default` e path recomendado da
plataforma; a precedência de contexto é `--context`, `context_name` persistido e
`current-context` somente no primeiro reconcile. Fonte escolhida inválida não
faz fallback silencioso: o shell permanece localmente saudável e Kubernetes
fica degradado. O descritor persiste apenas paths, nunca conteúdo, fingerprints
ou credenciais. Fingerprints de modificação existem somente em memória para
invalidar o clientset.

`--namespace` é somente uma seleção inicial: um scope `single` efêmero aplicado
uma vez ao primeiro contexto válido, sem linha no SQLite. Depois que a UI
seleciona um scope, essa intenção explícita substitui o valor CLI já consumido.

O root de dados é `~/.kubePeep/` em Unix e
`%LOCALAPPDATA%\kubePeep\` em Windows. O adapter Windows resolve
`FOLDERID_LocalAppData` diretamente para manter banco, logs, cache e runtime
fora do perfil Roaming; todos os arquivos permitidos ficam sob esse único root.

### 7.1 Configuração operacional local

`config.yaml` é um único documento YAML estrito, UTF-8, máximo 64 KiB, sem
aliases, anchors, tags, duplicate keys ou campos desconhecidos. Ausência cria o
arquivo com defaults por escrita privada/atômica; arquivo inválido impede
prontidão e retorna erro sanitizado. Schema v1 (validação em `internal/config`):

```yaml
version: 1
server:
  port: null
  openBrowser: true
  shutdownTimeout: 10s
dashboard:
  blockTimeout: 8s
resources:
  collectionTimeout: 30s
observability:
  metrics:
    enabled: false
  otel:
    enabled: false
    endpoint: null
    protocol: http/protobuf
    insecure: false
```

`server.port` é null ou inteiro 1024–65535. `shutdownTimeout` usa inteiro
positivo seguido de `s`, entre 1s e 30s. `dashboard.blockTimeout` limita o
tempo de cada bloco do dashboard (mesma sintaxe de duração), entre 1s e 60s;
para clusters grandes, aumente o valor quando blocos do overview reportarem
erros parciais de timeout. `resources.collectionTimeout` limita a janela total
de um fan-out de listagem de recursos (mesma sintaxe), entre 5s e 300s; o
default cobre scopes típicos e o teto evita esperas sem limite. Deadline por
chamada Kubernetes permanece fixo no cliente (15s); o budget configurável é o
total da janela, não por chamada. Resultados autorizados de namespaces
concluídos são preservados quando outra origem falha; a paginação por cursor
continua o caminho explícito para concluir cargas maiores que o budget de uma
janela. O endpoint OTel é obrigatório somente
quando `enabled=true`: URL absoluta HTTP(S), máximo 2.048 bytes, sem userinfo,
query ou fragment; HTTP exige host loopback e `insecure=true`. O schema aceita somente `http/protobuf`; headers/tokens não são configuráveis.
A exportação OTel ainda não é implementada; aceitar configuração não
significa iniciar um exporter. Veja [observabilidade](observability.md). Com
`enabled=false`, endpoint precisa ser null e nenhuma resolução, socket ou
exporter é iniciado.

Precedência operacional é `flag CLI explicitamente presente > config.yaml >
default embutido`. `--port` substitui `server.port`; `--no-browser` força
`openBrowser=false`. Contexto, kubeconfig e namespace não existem no YAML e
mantêm a precedência Kubernetes documentada abaixo. Host, data root, paths de
arquivo, CORS/CSRF, política de logs e limites de segurança não são
configuráveis; `host` é campo desconhecido e o listener permanece exatamente
`127.0.0.1`.

### 7.2 Tree e estado de instância

```text
<data-root>/
├── config.yaml
├── kubePeep.db
├── logs/
│   └── kubePeep.log
├── runtime/
│   ├── kubePeep.lock
│   └── instance.json
└── cache/
```

Backups rotacionados do log e arquivos SQLite `-wal`/`-shm` são derivados
permitidos; temporários de escrita/backup existem somente durante a operação.
Não há arquivo PID, porta ou token paralelo. `InstanceStateV1`, gravado em
`instance.json`, existe somente
depois da prontidão e durante uma instância publicada. Ele é JSON estrito,
máximo 64 KiB, sem trailing content, e possui exatamente:

```json
{
  "schema": 1,
  "instance_id": "inst_base64url128",
  "pid": 12345,
  "fingerprint": "platform-start-fingerprint",
  "port": 2748,
  "protocol": "kubepeep-control/v1",
  "control_token": "base64url-256-bit-token"
}
```

`instance_id` usa 128 bits aleatórios e `control_token` 256 bits, ambos
base64url sem padding e gerados pelo CSPRNG. PID é positivo, porta 1024–65535,
fingerprint é opaco não vazio até 256 bytes e protocolo é a literal mostrada.
O token é enviado apenas no header privado do canal de controle; o
`ControlIdentityDTO` deliberadamente omite o sétimo campo.

### 7.3 Resultados dos comandos

Exit codes comuns: 0 sucesso/estado desejado, 1 argumento ou config inválida, 2
falha operacional local, 3 estado consultado indisponível/degradado e 4 falha
interna inesperada. Saída humana vai para stdout em sucesso/estado; erro
sanitizado vai para stderr.

| Caso | Resultado |
| --- | --- |
| `start`/raiz encontra identidade ativa comprovada | não cria processo; abre a URL existente salvo `--no-browser`; exit 0 |
| lock livre + `instance.json` obsoleto comprovado | remove somente o estado obsoleto e inicia normalmente |
| lock ocupado sem identidade comprovável ou estado corrompido/adulterado | fail-closed, não sinaliza/não remove; exit 2 |
| porta explícita ocupada ou qualquer erro de bind não elegível a avanço | sem fallback; exit 2 |
| SIGINT/SIGTERM, `stop` ou encerramento normal com cleanup completo | exit 0 |
| erro de Serve, timeout ou falha de qualquer hook de cleanup | cleanup restante continua; exit 2 |
| `status` prova identidade ativa | imprime `running`, PID/porta/protocolo sem token; exit 0 |
| `status` sem estado ou com obsolescência comprovada | imprime `not running`, limpa apenas estado comprovadamente stale; exit 3 |
| `status` encontra estado inseguro/corrompido | não remove/não sinaliza; exit 2 |
| `stop` ativo recebe prova válida | cancelamento aceito; exit 0 sem aguardar término ilimitado |
| `stop` sem instância ou com obsolescência comprovada | sucesso idempotente; exit 0 |
| `stop` encontra identidade divergente/estado inseguro | não encerra PID nem remove estado; exit 2 |

## 8. Seleção, geração e cancelamento

### 8.1 Generation ID

Cada combinação ativa de profile/contexto/escopo produz uma geração monotônica em memória. Toda query, cursor, watch e sessão registra a geração de origem.

Ao trocar contexto ou escopo:

1. criar a próxima geração;
2. cancelar o contexto da anterior;
3. fechar watches e sessões vinculados;
4. invalidar caches que dependem da seleção;
5. rejeitar respostas/cursors da geração anterior.

O frontend também associa requests à geração e descarta respostas obsoletas, mesmo se o cancelamento de rede não chegar a tempo.

Um coordenador monotônico único serializa `contexts/select`, `scopes/select` e
todo PUT/DELETE de scope. Cada operação recebe sequência, cancela a intenção
anterior, compara `expectedGeneration` e relê seleção/scope sob o mesmo lock
lógico imediatamente antes do commit. Só a sequência mais nova pode publicar
generation/nonce; isso inclui update/delete cujo alvo se tornou ativo durante a
request.

### 8.2 Hierarquia de contextos

```text
process context
  └─ selection generation
      ├─ page/query
      │   └─ Kubernetes request
      ├─ dashboard refresh
      │   ├─ events block
      │   └─ log scan
      ├─ watch subscription
      ├─ log follow
      ├─ port-forward session
      └─ exec session
```

Cancelar um filho não cancela irmãos. Cancelar a geração cancela todos os filhos.

## 9. Caches

### 9.1 Clientset

Chave lógica:

```text
ordered normalized kubeconfig paths + context
```

Invalidações:

- mudança do conjunto/caminho;
- mudança de contexto;
- modificação observada de qualquer arquivo;
- erro de autenticação classificado como reconstruível;
- encerramento do processo.

Credenciais e `rest.Config` existem somente em memória. A persistência mantém apenas paths ordenados e contexto.

### 9.2 RBAC

Chave completa:

```text
selection generation
+ namespace
+ apiGroup
+ resource
+ subresource
+ verb
+ resourceName (quando a consulta é sobre objeto específico)
```

- TTL limitado a 30–60 segundos nas opções do serviço; default 45 segundos.
- Deduplicação de consultas simultâneas para a mesma chave.
- `SelfSubjectRulesReview` pode preencher dicas de UI, nunca conceder uma ação.
- `SelfSubjectAccessReview` decide capability quando disponível.
- Resposta incompleta, timeout ou erro produz `unknown`, não 403 inventado.
- Mutação/upgrade sempre revalida e a chamada Kubernetes continua sendo autoridade final.

### 9.3 Dados do cluster

O MVP não mantém cache persistente de dados Kubernetes. TanStack Query pode manter dados apenas em memória durante a sessão, respeitando `no-store` e sem persister/service worker.

### 9.4 Timeouts, concorrência e backoff

Defaults iniciais do backend:

| Operação | Timeout/limite |
| --- | --- |
| conectividade/discovery Kubernetes | 5 s |
| SAR individual | 5 s |
| GET/LIST de uma página/origem | 15 s (client factory) |
| janela total de fan-out de recursos | `resources.collectionTimeout`, default 30 s, 5–300 s |
| bloco comum do dashboard | `dashboard.blockTimeout`, default 8 s, 1–60 s |
| consulta individual de log no scan | 8 s |
| scan completo | 30 s |
| prontidão local antes de abrir browser | 5 s |
| fan-out de namespaces | concorrência 4 |
| fan-out de kinds | concorrência 3 |
| blocos simultâneos do dashboard | concorrência 6 |
| tentativas automáticas de leitura idempotente | no máximo 2 |

Mutação, port-forward e `exec` não são repetidos automaticamente. O shutdown
gracioso tem default de 10 s, seguido de fechamento forçado e cleanup; o valor
é configurável dentro dos limites de `config.yaml`.

Watch/reconexão usa somente a sequência fechada da §11 (250 ms até teto de 10
s, jitter ±20% e reset após 60 s estáveis). Troca de geração cancela o backoff
imediatamente.

## 10. Listas, cursor e fan-out

### 10.1 Cursor externo

O cursor público é opaco, assinado ou autenticado contra adulteração e ligado a:

- versão do schema;
- hash da query/filtros;
- generation ID;
- contexto e escopo;
- kinds/namespaces pendentes;
- tokens `continue` por origem;
- posição de merge;
- expiração.

### 10.2 Semântica

- `single`: normalmente encapsula um token Kubernetes.
- `all`: exige `list namespaces`; para cada recurso, usa lista global quando
  autorizada e, caso contrário, faz fan-out limitado somente pelos namespaces
  descobertos e autorizados;
- `list`: pode exigir fan-out limitado e cursor composto por namespace.
- múltiplos kinds usam merge determinístico por chave documentada.
- cursor de outra query/geração retorna `CURSOR_MISMATCH`;
- token malformado, adulterado ou assinado por outra instância retorna
  `CURSOR_INVALID`;
- cursor cujo TTL terminou retorna `CURSOR_EXPIRED`/HTTP 410.

Busca e ordenação globais só são anunciadas quando podem ser cumpridas sem coleta ilimitada. Caso contrário, a API restringe campos a selectors suportados ou declara ordenação local da página. O contrato detalhado está em [api.md](api.md).

O spike de F1-33 aprovou JSON versionado, base64url e HMAC-SHA-256 com chave
aleatória por processo. Adulteração, expiração e mudança de query/geração são
rejeitadas. O DTO público continua opaco.

## 11. Watches e atualização em tempo real

Fluxo de atualização:

1. executar LIST inicial e capturar `resourceVersion`;
2. verificar separadamente `list` e `watch`;
3. iniciar watch compartilhado por contexto/escopo/GVR/seletor;
4. multiplexar eventos para subscribers compatíveis;
5. usar bookmarks quando suportados;
6. em `410 Gone`, relistar e reiniciar;
7. aplicar backoff com jitter;
8. cair para refresh HTTP quando watch for negado ou inviável.

Cada watch solicita `timeoutSeconds=300` e bookmarks. Timeout limpo reinicia a
partir do último `resourceVersion`; `410 Gone` sempre força novo LIST. Falha de
rede, 429 ou 5xx usa backoff exponencial de 250 ms, 500 ms, 1 s, 2 s, 4 s, 8 s
e teto de 10 s, com jitter uniforme de ±20%; 60 s estáveis zeram a sequência.
Não se repete erro de autenticação/autorização. Quando `list` é permitido mas
`watch` é negado, a tela usa LIST ao entrar, mudar filtros/seleção ou receber
refresh explícito; não inicia polling periódico implícito.

Limites:

- `timeoutSeconds`, backoff, tópicos, máximo de oito streams e buffer por
  conexão são os valores fechados de `api.md` §18, não configuração do browser;
- nenhum watcher por componente React;
- cliente lento é desconectado conforme contrato;
- troca de geração encerra tudo.

SSE é o transporte preferencial para atualizações unidirecionais. Conforme o
ADR 0003, usa `HandleRaw`, writer original, request ID/recovery próprios,
Host/Origin guard, `WriteTimeout=0`, budgets por rota e cancelamento pelo
contexto.

## 12. Logs e transporte bidirecional

### 12.1 Logs

- leitura comum por HTTP;
- follow por SSE raw;
- limite por linha, resposta, container, stream e tempo;
- backpressure finita;
- conteúdo sanitizado antes do DTO;
- zero persistência interna.

### 12.2 Exec

Exec requer transporte bidirecional, argv como lista, streams stdin/stdout/stderr, TTY opcional, resize, heartbeat, limite de payload, timeout idle e cleanup.

O POST protegido cria um ticket one-shot; o upgrade GET consome esse ticket
pelo subprotocolo WebSocket e repete Origin, geração e autorização. O token não
aparece em URL. O ADR 0003 rejeitou `pkg/ws` para esse caminho e fixou
`github.com/coder/websocket v1.8.15`, com Origin, masking, opcodes,
fragmentação, ping/pong, limites, deadlines e desconexão cobertos antes de
habilitar `exec`.

## 13. Dashboard progressivo

O frontend dispara queries independentes para:

- summary;
- problems;
- restarts;
- events;
- log scan;
- metrics.

Cada resposta agregada contém:

- `complete`;
- `truncated`;
- namespaces consultados;
- namespaces omitidos/negados;
- instante e generation ID;
- erros parciais sanitizados.

O dashboard coordena serviços de recursos compartilhados, com budgets e
erros parciais por bloco. Novas famílias devem estender essas fronteiras.

## 14. Health e status

### 14.1 `/health`

Representa prontidão local e separa:

- aplicação;
- SQLite;
- kubeconfig;
- contexto;
- cluster.

Falha de aplicação ou SQLite pode tornar o health não saudável. Kubeconfig ausente, contexto inválido ou cluster offline produzem estado externo degradado sem transformar automaticamente toda a aplicação em 503.

O payload e a semântica HTTP estão em [api.md](api.md) e no ADR 0002.
`health.Checker` é preservado como contrato, mas wrappers próprios fornecem
timeout, recover e erro público sanitizado.

### 14.2 `/api/v1/status`

É um endpoint de produto, não um probe. Inclui versão, commit, build date, porta, componentes e seleção sanitizada. Pode fornecer diagnóstico degradado mais rico que `/health`.

## 15. Observabilidade

Todo evento operacional estruturado usa, quando aplicável:

```text
timestamp, level, component, operation, request_id,
context, namespace, resource, duration, duration_ms, error_code
```

O tipo `logger.Logger` envolve um `slog.Logger` com handler próprio do Kube
Peep, JSON line em stdout/arquivo rotativo e sanitização recursiva antes dos
sinks. A política de conteúdo está fixada em [security.md](security.md):
payloads, credenciais, logs Kubernetes e comandos de `exec` não entram no log.

OpenTelemetry é opt-in, desativado por padrão e não pode ser dependência necessária do core. Sem configuração explícita, nenhum exporter ou tráfego é iniciado.

## 16. Decisões confirmadas

| Tema | Decisão |
| --- | --- |
| Framework | Ginger service v1.4.4 + Cobra manual; sem Gin |
| CLI | web em foreground; raiz desktop quando compilado com tag; stop/status para a instância web |
| HTTP | lifecycle próprio, loopback, bind real e raw middleware seguro |
| Persistência | modernc SQLite sem CGO, migrations/frontend embutidos |
| Health | crítico local separado de dependências externas degradadas |
| SSE | `pkg/sse` em `HandleRaw`, limites/cancelamento próprios |
| Exec | `coder/websocket`; `pkg/ws` rejeitado para terminal |
| Logging | tipo Ginger com handler/sink/redactor próprios |
| Cursor | HMAC, query/generation bound e expiração |
| Kubeconfig | loader oficial, lista ordenada multi-arquivo e plugins `exec` |

## 17. Validação e evolução

Testes arquiteturais, de geração/cursor, RBAC, lifecycle, transportes e
persistência acompanham as mesmas camadas no código. O [guia de
desenvolvimento](development.md) descreve como executá-los; a release também
exige validação nativa e dos instaladores.

Para adicionar uma família de recursos, estender adapter, serviço/DTO,
capabilities, API e resource framework, preservando limites e cancelamento.
As fases e os critérios de aceite estão no [plano v1](../plan/README.md).
