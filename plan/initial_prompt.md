Você é um engenheiro de software sênior especializado em Go, React, Kubernetes, arquitetura hexagonal, segurança RBAC e ferramentas locais para desenvolvedores.

Sua tarefa é planejar e construir o projeto **Kube Peep**, uma interface web local e minimalista para Kubernetes, destinada principalmente a desenvolvedores que possuem acesso restrito aos clusters.

O Kube Peep deve ser um produto irmão do DWYT, reaproveitando sua linguagem visual, experiência de instalação, estrutura de frontend e modelo de distribuição, mas utilizando o **Ginger Framework v1.4.4** como base estrutural do backend Go.

## Referências obrigatórias

Antes de implementar qualquer código, estude cuidadosamente:

* DWYT:
  https://github.com/fvmoraes/dwyt

* Ginger Framework v1.4.4:
  https://pkg.go.dev/github.com/fvmoraes/ginger@v1.4.4

* Repositório do Ginger:
  https://github.com/fvmoraes/ginger

Não copie regras de negócio do DWYT. Use-o somente como referência para:

* Organização do repositório.
* Identidade visual.
* Cores.
* Tipografia.
* Componentes compactos.
* Navegação.
* Frontend React.
* Embedding do frontend no binário Go.
* Comandos CLI.
* Scripts de instalação.
* GoReleaser.
* GitHub Actions.
* Experiência de execução local.
* Forma de empacotamento e distribuição.

## Objetivo do produto

O Kube Peep deve funcionar como uma IDE Kubernetes minimalista, segura e focada no desenvolvedor.

Ele não deve tentar substituir completamente ferramentas administrativas como Rancher, Lens ou OpenShift Console.

O foco é permitir que um desenvolvedor com RBAC limitado consiga rapidamente:

* Selecionar um contexto Kubernetes.
* Trabalhar com um namespace, vários namespaces ou todos os namespaces permitidos.
* Visualizar a saúde das aplicações.
* Encontrar pods problemáticos.
* Ver pods com muitos restarts.
* Consultar logs.
* Encontrar trechos de logs com erros.
* Visualizar eventos de Warning.
* Consultar Deployments, StatefulSets, DaemonSets, Jobs e CronJobs.
* Consultar Services e Ingresses.
* Ver detalhes e YAML dos recursos.
* Executar somente ações realmente permitidas pelo cluster.
* Fazer port-forward quando autorizado.
* Reiniciar workloads quando autorizado.
* Escalar workloads quando autorizado.
* Executar comandos em containers somente quando autorizado.

A interface nunca deve induzir o usuário a acreditar que possui uma permissão que não possui.

## Filosofia do produto

O comportamento central deve ser:

> Mostrar somente o que o usuário pode acessar e habilitar somente o que ele pode executar.

O Kube Peep deve ser:

* Minimalista.
* Rápido.
* Local-first.
* RBAC-aware.
* Seguro por padrão.
* Fácil de instalar.
* Fácil de remover.
* Sem dependências de runtime.
* Sem exigir Node.js depois da compilação.
* Sem armazenar credenciais Kubernetes.
* Sem tentar contornar permissões.
* Sem depender de acesso cluster-admin.
* Funcional mesmo sem Metrics Server.
* Funcional com kubeconfigs que utilizem plugins `exec`.

## Stack obrigatória

### Backend

* Go 1.25.
* Ginger Framework fixado exatamente em:

```go
github.com/fvmoraes/ginger v1.4.4
```

* Pacotes do Ginger sempre que aplicáveis:

  * `pkg/app`
  * `pkg/router`
  * `pkg/config`
  * `pkg/logger`
  * `pkg/errors`
  * `pkg/response`
  * `pkg/health`
  * `pkg/sse`
  * `pkg/ws`
  * `pkg/testhelper`

* Cobra para os comandos CLI.

* Bibliotecas oficiais do Kubernetes:

  * `k8s.io/client-go`
  * `k8s.io/api`
  * `k8s.io/apimachinery`
  * `k8s.io/metrics`, apenas para integração opcional com Metrics API.

* SQLite usando driver sem CGO, preferencialmente `modernc.org/sqlite`.

* `log/slog` por meio do logger estruturado do Ginger.

* `go:embed` para incorporar o frontend compilado.

* GoReleaser para builds multiplataforma.

Não usar Gin no novo backend. O Ginger deve ser a camada principal de aplicação e HTTP.

### Frontend

Utilizar a mesma família tecnológica do DWYT:

* React.
* TypeScript.
* Vite.
* Tailwind CSS.
* React Router.
* Componentes próprios e pequenos.
* Sem bibliotecas visuais pesadas.
* Sem Material UI.
* Sem Ant Design.
* Sem componentes com aparência genérica de painel corporativo.

Para dados remotos, usar uma solução simples e adequada, preferencialmente TanStack Query.

Para ícones, usar Lucide React.

## Identidade visual

O Kube Peep deve parecer um produto irmão do DWYT.

Reutilizar a identidade baseada em Catppuccin Mocha:

* Fundo escuro.
* Cards compactos.
* Bordas discretas.
* Fonte monoespaçada.
* Pouco espaçamento desperdiçado.
* Informações densas, mas legíveis.
* Estados representados por cores suaves.
* Sombras discretas.
* Cantos levemente arredondados.
* Sem excesso de gradientes.
* Sem visual de dashboard SaaS genérico.

Usar como base:

```css
--ctp-mauve: #cba6f7;
--ctp-red: #f38ba8;
--ctp-peach: #fab387;
--ctp-yellow: #f9e2af;
--ctp-green: #a6e3a1;
--ctp-sky: #89dceb;
--ctp-text: #cdd6f4;
--ctp-overlay1: #7f849c;
--ctp-base: #1e1e2e;
--ctp-mantle: #181825;
--ctp-crust: #11111b;
```

No Kube Peep, a cor principal deve ser **mauve/roxo ameixa**, enquanto no DWYT o amarelo continua sendo mais característico.

Sugestão:

```css
--accent: var(--ctp-mauve);
--success: var(--ctp-green);
--warning: var(--ctp-yellow);
--danger: var(--ctp-red);
--info: var(--ctp-sky);
```

A interface deve transmitir:

* Kubernetes.
* Simplicidade.
* Segurança.
* Clareza.
* Produto para desenvolvedores.

## Modelo de execução

O produto deve ser entregue como um único binário autocontido.

Fluxo esperado:

```bash
kubePeep
```

O comando deve:

1. Localizar o kubeconfig.
2. Inicializar o banco local.
3. Iniciar a API local.
4. Servir o frontend embutido.
5. Escolher uma porta disponível, começando por `2748`.
6. Abrir automaticamente o navegador.
7. Exibir a interface em `http://localhost:2748`.

Também implementar:

```bash
kubePeep start
kubePeep stop
kubePeep status
kubePeep version
kubePeep update
kubePeep doctor
kubePeep --context nome-do-contexto
kubePeep --kubeconfig /caminho/config
kubePeep --namespace payments
kubePeep --no-browser
kubePeep --port 2748
```

O comando padrão `kubePeep` deve equivaler a `kubePeep start`.

## Diretórios locais

Usar:

```text
~/.kubePeep/
├── config.yaml
├── kubePeep.db
├── logs/
│   └── kubePeep.log
├── runtime/
│   ├── kubePeep.pid
│   └── kubePeep.port
└── cache/
```

No Windows, usar o diretório de dados adequado ao usuário.

Nunca copiar nem armazenar o conteúdo completo do kubeconfig no banco.

Guardar somente:

* Caminho do kubeconfig selecionado.
* Nome do contexto.
* Preferências da interface.
* Escopos de namespaces.
* Filtros salvos.
* Configurações do dashboard.

Nunca armazenar:

* Tokens.
* Certificados.
* Client keys.
* Senhas.
* Conteúdo de secrets Kubernetes.
* Resultado persistente de logs de aplicações.

## Configuração de namespaces

Criar uma tela de cadastro chamada **Namespace Scopes** ou **Escopos de namespaces**.

Um escopo deve conter:

* Nome.
* Contexto Kubernetes.
* Modo de seleção.
* Lista de namespaces.
* Namespace padrão opcional.
* Data de criação.
* Data de atualização.

Modos disponíveis:

```text
single
list
all
```

### Modo single

Permite selecionar um único namespace.

### Modo list

Permite cadastrar vários namespaces de uma só vez.

A tela deve possuir:

* Campo de nome do escopo.
* Seletor de contexto.
* Textarea para inclusão em massa.
* Visualização em chips.
* Contador de namespaces válidos.
* Contador de duplicados descartados.
* Contador de nomes inválidos.
* Botão de validar.
* Botão de salvar.
* Opção de remover individualmente um namespace.
* Opção de limpar a lista.

Aceitar namespaces separados por:

* Quebra de linha.
* Vírgula.
* Ponto e vírgula.
* Espaço, quando não houver ambiguidade.
* Lista YAML simples.
* Lista JSON simples.

Exemplos aceitos:

```text
payments
billing
identity
notifications
```

```text
payments,billing,identity,notifications
```

```yaml
- payments
- billing
- identity
- notifications
```

Regras do processamento:

1. Remover espaços extras.
2. Converter entradas vazias em descarte.
3. Remover duplicados.
4. Preservar nomes válidos.
5. Validar conforme nomenclatura de namespace do Kubernetes.
6. Informar entradas inválidas antes de salvar.
7. Validar se o namespace existe quando o usuário possuir permissão para isso.
8. Não impedir o cadastro somente porque o usuário não possui permissão para listar namespaces.
9. Salvar toda a lista em uma única transação.
10. Não fazer uma requisição separada ao backend para cada namespace.

Criar também uma API de importação em massa.

Exemplo:

```http
POST /api/v1/namespace-scopes
```

```json
{
  "name": "Aplicações Financeiras",
  "context": "development",
  "mode": "list",
  "namespaces": [
    "payments",
    "billing",
    "invoices"
  ],
  "defaultNamespace": "payments"
}
```

### Modo all

Deve existir uma opção visual chamada:

```text
All namespaces
```

ou, na interface em português:

```text
Todos os namespaces
```

Essa opção não representa acesso irrestrito.

Ela significa:

> Consultar todos os namespaces e recursos que a identidade atual consegue listar por meio da API Kubernetes.

Nunca interpretar `all` como tentativa de contornar RBAC.

Quando o usuário selecionar `all`:

* Verificar se pode listar namespaces.
* Se puder, carregar somente namespaces retornados pela API.
* Se não puder listar namespaces, apresentar uma mensagem clara.
* Permitir que o usuário volte para um escopo com lista manual.
* Não exibir namespaces inferidos ou fictícios.
* Não persistir o caractere `*` como se fosse um namespace real.
* Guardar o modo `all` como atributo separado no banco.

Para operações em todos os namespaces, preferir chamadas Kubernetes de escopo global quando forem permitidas, em vez de executar uma requisição individual para cada namespace.

## Modelo de dados sugerido

Criar migrations versionadas.

### Tabela `cluster_profiles`

```text
id
name
kubeconfig_path
context_name
is_default
created_at
updated_at
```

### Tabela `namespace_scopes`

```text
id
cluster_profile_id
name
mode
default_namespace
created_at
updated_at
```

### Tabela `namespace_scope_items`

```text
id
namespace_scope_id
namespace
created_at
```

Adicionar índice único para:

```text
namespace_scope_id + namespace
```

### Tabela `preferences`

```text
key
value
updated_at
```

Não guardar snapshots permanentes do cluster no MVP.

## Integração Kubernetes

Criar uma camada isolada para o Kubernetes.

Não permitir que handlers HTTP chamem diretamente o clientset.

Usar interfaces em `internal/ports` e implementações em `internal/adapters/kubernetes`.

Separar responsabilidades:

```text
KubeconfigLoader
ContextService
NamespaceService
AuthorizationService
WorkloadService
PodService
LogService
EventService
MetricsService
PortForwardService
ExecService
DashboardService
```

Usar o mesmo kubeconfig utilizado pelo kubectl.

Suportar:

* `KUBECONFIG`.
* `~/.kube/config`.
* Caminho definido por flag.
* Contextos múltiplos.
* Plugins de autenticação `exec`.
* Certificados.
* Tokens já referenciados pelo kubeconfig.
* Configuração in-cluster futuramente, mas não como prioridade do MVP.

Configurar timeouts.

Não usar clientes globais mutáveis sem controle.

Criar cache de clientsets por:

```text
kubeconfig path + context
```

Invalidar o cache quando:

* O caminho mudar.
* O contexto mudar.
* O arquivo kubeconfig for modificado.
* Uma autenticação retornar erro que exija reconstrução.

## Segurança e RBAC

Antes de apresentar ações, consultar as permissões do usuário por meio de:

* `SelfSubjectAccessReview`.
* `SelfSubjectRulesReview` como otimização e resumo, quando disponível.
* A API Kubernetes como autoridade final.

Verificar permissões por:

* Contexto.
* Namespace.
* API group.
* Recurso.
* Subresource.
* Verbo.

Exemplos:

```text
get pods
list pods
watch pods
get pods/log
create pods/exec
create pods/portforward
get deployments
patch deployments
update deployments/scale
list events
get configmaps
get secrets
```

Regras:

* Esconder ações não autorizadas.
* Quando for importante mostrar a ação, exibi-la desabilitada com explicação.
* Nunca confiar somente na interface.
* Validar novamente a permissão no backend.
* Retornar HTTP 403 padronizado.
* Não fazer impersonation.
* Não solicitar credenciais adicionais.
* Não tentar descobrir Secrets sem permissão.
* Não exibir conteúdo de Secrets no MVP.
* Não registrar tokens ou cabeçalhos sensíveis.
* Sanitizar erros produzidos por plugins de autenticação.
* Registrar somente metadados operacionais necessários.

Implementar cache curto de permissões, separado por contexto e namespace.

Tempo sugerido:

```text
TTL entre 30 e 60 segundos
```

Disponibilizar botão para atualizar permissões manualmente.

## Dashboard inicial

A tela inicial deve ser rápida, útil e orientada a problemas.

Não criar um dashboard cheio de gráficos decorativos.

O dashboard deve responder rapidamente:

* O cluster está acessível?
* Qual contexto está ativo?
* Qual escopo de namespaces está ativo?
* Existem aplicações quebradas?
* Quais pods mais reiniciaram?
* Existem eventos de Warning?
* Existem erros recentes nos logs?
* O usuário possui quais permissões principais?

### Cabeçalho

Exibir:

* Nome Kube Peep.
* Contexto atual.
* Cluster atual.
* Escopo atual.
* Quantidade de namespaces no escopo.
* Indicador de conexão.
* Tempo da última atualização.
* Botão de atualizar.
* Atalho para troca de contexto.
* Atalho para troca de escopo.

### Cards de resumo

Exibir cards compactos para:

* Namespaces acessíveis.
* Pods totais.
* Pods saudáveis.
* Pods problemáticos.
* Workloads degradados.
* Restarts encontrados.
* Eventos Warning recentes.
* Logs com possíveis erros.

Cada card deve ser clicável e abrir a lista correspondente já filtrada.

### Pods com mais restarts

Criar uma tabela chamada:

```text
Top restarting pods
```

Campos:

* Namespace.
* Pod.
* Workload proprietário.
* Container.
* Total de restarts.
* Status atual.
* Último motivo.
* Idade.
* Ação para abrir detalhes.
* Ação para abrir logs.

Ordenar por maior número de restarts.

Calcular restarts usando os status dos containers.

Considerar separadamente:

* Containers normais.
* Init containers.
* Containers efêmeros, quando existirem.

Exibir pelo menos os 10 primeiros.

Não considerar um restart isolado automaticamente como erro crítico. Usar níveis visuais:

```text
0 = saudável
1–2 = atenção leve
3–9 = warning
10 ou mais = crítico
```

Esses limites devem ser configuráveis posteriormente.

### Pods problemáticos

Identificar condições como:

* CrashLoopBackOff.
* ImagePullBackOff.
* ErrImagePull.
* CreateContainerConfigError.
* RunContainerError.
* OOMKilled.
* Evicted.
* Failed.
* Pending por período prolongado.
* Containers não prontos.
* Readiness probe falhando.
* Liveness probe falhando.
* Scheduling failures.

Exibir o motivo real retornado pelo Kubernetes.

Não inventar diagnósticos.

### Workloads degradados

Exibir:

* Deployment com réplicas indisponíveis.
* StatefulSet incompleto.
* DaemonSet com pods indisponíveis.
* Job falhando.
* CronJob com falhas recentes.

Campos:

* Namespace.
* Tipo.
* Nome.
* Ready.
* Desired.
* Available.
* Updated.
* Status.
* Idade.

### Eventos recentes

Carregar eventos dos namespaces permitidos.

Dar prioridade a:

```text
type = Warning
```

Exibir:

* Timestamp.
* Namespace.
* Tipo do objeto.
* Nome do objeto.
* Reason.
* Message.
* Count.
* Source.

Agrupar eventos repetidos quando fizer sentido.

Não perder o contador original do Kubernetes.

### Logs de erros

Não escanear indiscriminadamente todos os logs de todos os pods do cluster a cada abertura do dashboard.

Isso pode gerar carga excessiva, lentidão e problemas de autorização.

Implementar uma análise limitada e segura.

Fluxo inicial:

1. Priorizar pods problemáticos.
2. Priorizar pods com restarts.
3. Priorizar containers terminados recentemente.
4. Consultar apenas namespaces do escopo atual.
5. Respeitar permissão `get` para `pods/log`.
6. Usar limite de concorrência.
7. Usar timeout por consulta.
8. Limitar a quantidade de pods.
9. Limitar linhas por container.
10. Limitar janela temporal.
11. Não persistir o conteúdo dos logs.

Valores iniciais sugeridos:

```text
janela: últimos 15 minutos
tailLines: 200
máximo de pods: 20
máximo de containers concorrentes: 4
timeout por requisição: 8 segundos
```

Permitir configuração para:

```text
15 minutos
30 minutos
1 hora
4 horas
```

Buscar padrões comuns, sem afirmar que todo resultado é necessariamente um erro:

```text
error
exception
fatal
panic
failed
failure
timeout
refused
unavailable
oom
killed
segmentation fault
```

A busca deve ser case-insensitive.

Quando possível, reconhecer logs JSON e analisar campos como:

```text
level
severity
message
msg
error
stack
timestamp
time
```

Considerar níveis:

```text
error
fatal
panic
critical
```

Exibir como:

```text
Possible error logs
```

ou:

```text
Possíveis erros nos logs
```

Não chamar o resultado simplesmente de “erros confirmados”.

Cada resultado deve conter:

* Timestamp, quando detectável.
* Namespace.
* Pod.
* Container.
* Workload.
* Trecho do log.
* Motivo pelo qual foi classificado.
* Link para abrir os logs completos.
* Opção para copiar.
* Opção para aplicar filtro.

Ocultar ou mascarar valores que pareçam:

* Bearer tokens.
* Senhas.
* Chaves privadas.
* Authorization headers.
* JWTs longos.
* Connection strings sensíveis.

Adicionar um botão explícito:

```text
Scan logs now
```

O dashboard pode executar uma busca limitada inicial, mas pesquisas mais amplas devem depender de ação do usuário.

Cancelar requisições quando:

* O contexto mudar.
* O escopo mudar.
* O usuário sair da tela.
* Uma nova atualização substituir a anterior.

### Métricas de CPU e memória

Detectar se a API `metrics.k8s.io` está disponível e se o usuário possui permissão.

Quando disponível, exibir:

* CPU atual.
* Memória atual.
* Pods com maior CPU.
* Pods com maior memória.

Quando não estiver disponível:

* Não considerar isso um erro global.
* Ocultar os cards.
* Exibir uma indicação discreta de que Metrics API não está disponível.
* Não exigir Metrics Server para o restante do dashboard funcionar.

## Navegação principal

Criar menu lateral compacto:

```text
Overview
Workloads
Pods
Logs
Events
Network
Config
Namespaces
Permissions
Settings
```

### Overview

Dashboard rápido e orientado a problemas.

### Workloads

Abas:

* Deployments.
* StatefulSets.
* DaemonSets.
* Jobs.
* CronJobs.

### Pods

Tabela com:

* Namespace.
* Nome.
* Status.
* Ready.
* Restarts.
* Node.
* IP.
* Owner.
* Idade.

Filtros:

* Namespace.
* Status.
* Workload.
* Node.
* Com restarts.
* Somente problemáticos.
* Busca textual.

### Logs

Permitir:

* Escolher namespace.
* Escolher pod.
* Escolher container.
* Logs atuais.
* Logs anteriores, quando disponíveis.
* Follow.
* Timestamps.
* Tail lines.
* Since.
* Busca local.
* Pausar stream.
* Copiar linha.
* Baixar somente quando acionado explicitamente.
* Quebra de linha opcional.
* Tema monoespaçado.

Usar streaming via SSE ou WebSocket, priorizando a solução mais simples e robusta compatível com o Ginger.

Garantir cancelamento do stream quando a tela for fechada.

### Events

Lista cronológica com filtros.

### Network

Exibir:

* Services.
* Ingresses.
* Endpoints ou EndpointSlices.
* Port-forward ativo.

### Config

No MVP:

* ConfigMaps somente leitura.
* Secrets apenas como metadados, sem revelar valores.
* YAML de recursos permitidos.

### Permissions

Criar uma matriz legível por namespace e recurso.

Exemplo:

```text
Resource       View   Watch   Logs   Edit   Delete   Exec   Port Forward
Pods           Yes    Yes     Yes    No     No       No     Yes
Deployments    Yes    Yes     —     Restart Scale   No      —
Services       Yes    Yes     —      No     No       —      —
```

Não utilizar apenas ícones; incluir texto ou tooltip acessível.

## Ações permitidas

Implementar inicialmente:

* Visualizar detalhes.
* Visualizar YAML.
* Abrir logs.
* Abrir logs anteriores.
* Reiniciar Deployment.
* Alterar escala de Deployment e StatefulSet.
* Excluir Pod para permitir recriação, quando autorizado.
* Port-forward.
* Exec, quando autorizado.

Operações destrutivas devem exigir confirmação.

A confirmação deve mostrar:

* Contexto.
* Namespace.
* Tipo.
* Nome.
* Ação.
* Consequência esperada.

Nunca criar uma autorização própria paralela ao Kubernetes.

## API HTTP

Usar prefixo:

```text
/api/v1
```

Rotas sugeridas:

```text
GET    /api/v1/status
GET    /api/v1/contexts
POST   /api/v1/contexts/select

GET    /api/v1/cluster/profile
GET    /api/v1/namespaces

GET    /api/v1/namespace-scopes
POST   /api/v1/namespace-scopes
GET    /api/v1/namespace-scopes/{id}
PUT    /api/v1/namespace-scopes/{id}
DELETE /api/v1/namespace-scopes/{id}
POST   /api/v1/namespace-scopes/validate

GET    /api/v1/dashboard/summary
GET    /api/v1/dashboard/restarts
GET    /api/v1/dashboard/problems
GET    /api/v1/dashboard/events
POST   /api/v1/dashboard/log-scan

GET    /api/v1/workloads
GET    /api/v1/workloads/{kind}/{namespace}/{name}
POST   /api/v1/workloads/{kind}/{namespace}/{name}/restart
PUT    /api/v1/workloads/{kind}/{namespace}/{name}/scale

GET    /api/v1/pods
GET    /api/v1/pods/{namespace}/{name}
DELETE /api/v1/pods/{namespace}/{name}

GET    /api/v1/pods/{namespace}/{name}/logs
GET    /api/v1/pods/{namespace}/{name}/logs/stream
POST   /api/v1/pods/{namespace}/{name}/exec
POST   /api/v1/pods/{namespace}/{name}/port-forward

GET    /api/v1/events
GET    /api/v1/services
GET    /api/v1/ingresses
GET    /api/v1/configmaps
GET    /api/v1/permissions
GET    /api/v1/metrics
```

Não implementar todas as rotas de uma vez sem planejamento.

Usar DTOs próprios. Não retornar objetos internos completos do client-go diretamente ao frontend.

Usar envelopes padronizados do Ginger:

```json
{
  "data": {}
}
```

E erros padronizados:

```json
{
  "code": "FORBIDDEN",
  "message": "You do not have permission to access pod logs in this namespace."
}
```

Paginar listas potencialmente grandes.

Aceitar:

```text
limit
continue
search
namespace
status
sort
order
```

## Atualização em tempo real

Usar inicialmente:

* Requisições HTTP para carregamento.
* Watches Kubernetes compartilhados quando houver benefício.
* SSE para enviar atualizações simples ao frontend.
* WebSocket apenas para casos realmente bidirecionais, como exec.

Evitar polling agressivo.

Quando usar watch:

* Tratar `resourceVersion`.
* Tratar encerramento do watch.
* Reconectar com backoff.
* Cancelar ao mudar contexto ou escopo.
* Não criar um watcher por componente React.
* Centralizar watchers no backend.
* Compartilhar resultados quando possível.

## Estrutura sugerida

```text
kubePeep/
├── cmd/
│   └── kubePeep/
│       └── main.go
│
├── internal/
│   ├── commands/
│   │   ├── root.go
│   │   ├── start.go
│   │   ├── stop.go
│   │   ├── status.go
│   │   ├── doctor.go
│   │   ├── update.go
│   │   └── version.go
│   │
│   ├── api/
│   │   ├── router.go
│   │   ├── handlers/
│   │   ├── middleware/
│   │   └── dto/
│   │
│   ├── config/
│   ├── models/
│   ├── ports/
│   ├── services/
│   │   ├── dashboard/
│   │   ├── authorization/
│   │   ├── namespaces/
│   │   ├── workloads/
│   │   ├── pods/
│   │   ├── logs/
│   │   ├── events/
│   │   └── metrics/
│   │
│   ├── adapters/
│   │   ├── kubernetes/
│   │   ├── sqlite/
│   │   ├── filesystem/
│   │   └── browser/
│   │
│   ├── migrations/
│   ├── runtime/
│   └── web/
│       └── embed.go
│
├── web/
│   ├── src/
│   │   ├── api/
│   │   ├── components/
│   │   ├── features/
│   │   │   ├── dashboard/
│   │   │   ├── contexts/
│   │   │   ├── namespaces/
│   │   │   ├── workloads/
│   │   │   ├── pods/
│   │   │   ├── logs/
│   │   │   ├── events/
│   │   │   └── permissions/
│   │   ├── hooks/
│   │   ├── layouts/
│   │   ├── pages/
│   │   ├── routes/
│   │   ├── styles/
│   │   ├── types/
│   │   └── utils/
│   ├── package.json
│   └── vite.config.ts
│
├── configs/
│   └── app.yaml
│
├── docs/
│   ├── product-spec.md
│   ├── architecture.md
│   ├── api.md
│   ├── security.md
│   ├── data-model.md
│   ├── implementation-plan.md
│   └── decisions/
│
├── tests/
│   ├── integration/
│   └── e2e/
│
├── devops/
│   ├── docker/
│   └── pipelines/
│
├── install.sh
├── install.ps1
├── Makefile
├── ginger.yaml
├── go.mod
├── go.sum
├── .goreleaser.yaml
├── README.md
└── LICENSE
```

Adapte essa estrutura ao que o Ginger v1.4.4 realmente suporta.

Não force uma estrutura que entre em conflito com o Ginger.

## Uso obrigatório do Ginger

O projeto deve ser construído sobre o Ginger, não apenas mencionar o framework no `go.mod`.

Primeiro avalie a melhor forma de iniciar o projeto.

Provável fluxo:

```bash
ginger new kubePeep --service
cd kubePeep
ginger inspect
ginger doctor
```

Como o Kube Peep também possui CLI, adapte o entrypoint para usar Cobra e iniciar a aplicação Ginger no comando `start`.

Caso os geradores do Ginger não suportem automaticamente um projeto híbrido CLI + serviço HTTP:

* Não abandone o Ginger.
* Não substitua o Ginger por Gin.
* Crie a camada Cobra manualmente.
* Continue usando o Ginger para bootstrap, router, config, logs, erros, respostas, health checks, testes e estrutura.
* Documente essa decisão em `docs/decisions/`.

Antes de aplicar geradores ou integrações, usar sempre que disponível:

```bash
ginger <comando> --plan
```

Não usar `--force` sem necessidade comprovada.

Executar periodicamente:

```bash
ginger inspect
ginger doctor
```

## Entrega e instalação

Seguir a experiência do DWYT.

### Linux e macOS

```bash
curl -fsSL https://raw.githubusercontent.com/fvmoraes/kubePeep/main/install.sh | bash
```

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/fvmoraes/kubePeep/main/install.ps1 | iex
```

O instalador deve:

* Detectar sistema operacional.
* Detectar arquitetura.
* Baixar a release correta.
* Validar checksum SHA-256.
* Instalar no PATH do usuário.
* Substituir versão anterior com segurança.
* Não exigir permissões administrativas quando não forem necessárias.
* Informar claramente o próximo comando.

Gerar releases para:

* Linux amd64.
* Linux arm64.
* macOS amd64.
* macOS arm64.
* Windows amd64.
* Windows arm64, se as dependências permitirem.

O binário final deve conter:

* Backend.
* Frontend.
* Migrations.
* Assets.
* Versão.
* Commit.
* Data de build.

## Health checks

Implementar:

```text
GET /health
GET /api/v1/status
```

O health check deve separar:

* Aplicação local.
* Banco SQLite.
* Kubeconfig.
* Conexão com cluster.
* Contexto selecionado.

A aplicação local pode estar saudável mesmo que o cluster esteja temporariamente inacessível.

Não retornar 503 para toda a aplicação apenas porque o cluster está offline.

## Testes obrigatórios

### Unitários

Cobrir:

* Parser de namespaces em massa.
* Remoção de duplicados.
* Validação de namespaces.
* Modo `all`.
* Cálculo de restarts.
* Detecção de pods problemáticos.
* Classificação de eventos.
* Classificação de possíveis erros em logs.
* Sanitização de dados sensíveis.
* Cache de permissões.
* Ordenação do dashboard.
* Conversão de objetos Kubernetes em DTOs.

### Integração

Usar:

* Fake clientset do client-go para casos simples.
* Servidor HTTP de teste para handlers.
* Banco SQLite temporário.
* Testes do Ginger em handlers e respostas.
* `envtest` apenas se realmente necessário.

Cobrir:

* Usuário com acesso a um namespace.
* Usuário com acesso a vários namespaces.
* Usuário que pode listar todos os namespaces.
* Usuário que não pode listar namespaces.
* Usuário que pode ver pods, mas não logs.
* Usuário que pode ver logs, mas não executar exec.
* Usuário que pode escalar, mas não excluir.
* Metrics API indisponível.
* Cluster offline.
* Contexto inexistente.
* Plugin de autenticação falhando.

### Frontend

Cobrir:

* Cadastro de vários namespaces.
* Importação por linhas e vírgulas.
* Duplicados.
* Entradas inválidas.
* Seleção de todos os namespaces.
* Dashboard vazio.
* Dashboard carregando.
* Dashboard com erro parcial.
* Permissões desabilitando ações.
* Mudança de contexto cancelando requisições.
* Logs sendo interrompidos ao sair da página.

### E2E

Criar cenário mínimo com Kind ou K3d:

* Namespace permitido.
* Namespace negado.
* Deployment saudável.
* Deployment degradado.
* Pod com restart.
* Pod gerando logs de erro.
* Eventos Warning.
* Service.
* Ingress.
* Role e RoleBinding restritos.

## Performance

Metas iniciais:

* Interface local disponível rapidamente.
* Dashboard inicial carregado progressivamente.
* Primeira resposta de status sem depender da coleta completa.
* Não bloquear a tela esperando logs.
* Não carregar YAML completo em listas.
* Não retornar listas gigantes sem paginação.
* Não abrir watchers desnecessários.
* Não executar uma consulta de logs por pod sem limite.

Dashboard dividido em blocos independentes:

```text
summary
problems
restarts
events
log scan
metrics
```

Cada bloco pode carregar separadamente.

Erros parciais não devem derrubar todo o dashboard.

Exemplo:

* Pods carregaram.
* Eventos foram negados.
* Metrics API não existe.
* Logs não são permitidos.

A interface deve continuar utilizável e informar cada limitação separadamente.

## Observabilidade local

Usar logs estruturados.

Campos mínimos:

```text
timestamp
level
component
operation
request_id
context
namespace
resource
duration
error_code
```

Não registrar:

* Conteúdo completo de logs Kubernetes por padrão.
* Tokens.
* Certificados.
* Secrets.
* Authorization headers.
* Conteúdo do kubeconfig.
* Comandos digitados em exec.

OpenTelemetry deve ser opcional e desativado por padrão.

## SDD e ordem de trabalho

Não comece criando dezenas de arquivos imediatamente.

Execute o trabalho nesta ordem.

### Fase 1 — Descoberta

1. Inspecionar DWYT.
2. Inspecionar Ginger v1.4.4.
3. Identificar componentes reaproveitáveis.
4. Identificar incompatibilidades.
5. Confirmar a estratégia para projeto híbrido CLI + serviço.
6. Registrar decisões.

### Fase 2 — Especificação

Criar:

```text
docs/product-spec.md
docs/architecture.md
docs/security.md
docs/data-model.md
docs/api.md
docs/implementation-plan.md
```

Os documentos devem incluir critérios de aceite objetivos.

### Fase 3 — Fundação

Criar:

* Projeto Ginger.
* Comandos Cobra.
* Configuração.
* Logging.
* SQLite.
* Migrations.
* Health check.
* Frontend React.
* Embedding.
* Build local.
* Testes básicos.

### Fase 4 — Kubernetes

Implementar:

* Kubeconfig.
* Contextos.
* Conexão.
* Namespaces.
* Escopos.
* Permissões.
* Tratamento de erros.
* DTOs.

### Fase 5 — Dashboard

Implementar:

* Resumo.
* Pods problemáticos.
* Pods com mais restarts.
* Workloads degradados.
* Eventos Warning.
* Scan limitado de logs.
* Métricas opcionais.

### Fase 6 — Recursos

Implementar:

* Workloads.
* Pods.
* Logs.
* Events.
* Services.
* Ingresses.
* ConfigMaps.
* YAML.

### Fase 7 — Ações

Implementar uma por vez:

* Restart.
* Scale.
* Delete Pod.
* Port-forward.
* Exec.

Cada ação precisa de:

* Validação RBAC.
* Confirmação.
* Testes.
* Cancelamento.
* Tratamento de erro.

### Fase 8 — Distribuição

Implementar:

* GoReleaser.
* GitHub Actions.
* Checksum.
* Instaladores.
* Atualização.
* Testes multiplataforma.
* README.

## Critérios de aceite do MVP

O MVP estará pronto somente quando:

1. `kubePeep` iniciar a aplicação local.
2. O navegador abrir automaticamente.
3. O frontend estiver embutido no binário.
4. Não houver dependência de Node.js em runtime.
5. O usuário puder selecionar um contexto.
6. O usuário puder cadastrar um namespace.
7. O usuário puder cadastrar vários namespaces de uma vez.
8. Duplicados forem removidos corretamente.
9. Entradas inválidas forem informadas.
10. A opção “Todos os namespaces” funcionar respeitando RBAC.
11. O dashboard mostrar pods problemáticos.
12. O dashboard mostrar pods com mais restarts.
13. O dashboard mostrar workloads degradados.
14. O dashboard mostrar eventos Warning.
15. O usuário autorizado conseguir ver logs.
16. O usuário não autorizado não conseguir ver logs.
17. O scan de logs possuir limites de segurança.
18. Metrics API indisponível não quebrar o dashboard.
19. Ações não autorizadas estiverem ocultas ou desabilitadas.
20. O backend validar novamente toda ação.
21. O produto funcionar com um usuário sem cluster-admin.
22. O SQLite não armazenar credenciais.
23. Os testes principais passarem.
24. `ginger doctor` não apresentar problemas não documentados.
25. O binário for gerado pelo GoReleaser.
26. Os instaladores validarem checksum.
27. Linux, macOS e Windows estiverem contemplados na configuração de release.

## Restrições

Não fazer:

* Não usar Electron.
* Não criar um servidor cloud obrigatório.
* Não exigir login próprio no MVP.
* Não armazenar kubeconfig completo.
* Não armazenar tokens.
* Não exigir cluster-admin.
* Não contornar RBAC.
* Não usar Gin.
* Não substituir Ginger por outro framework.
* Não fazer scan ilimitado de logs.
* Não carregar todos os recursos sem paginação.
* Não exibir Secrets.
* Não criar gráficos sem utilidade.
* Não implementar edição YAML irrestrita no MVP.
* Não implementar terminal exec antes da base de segurança estar pronta.
* Não adicionar dependências sem justificar.
* Não usar mocks no produto final.
* Não apresentar dados fictícios fora de Storybook ou testes.
* Não sobrescrever arquivos existentes sem revisar.
* Não usar `ginger --force` automaticamente.
* Não declarar uma fase concluída sem testes.

## Forma de execução esperada

Ao iniciar:

1. Apresente um resumo do que encontrou no DWYT.
2. Apresente um resumo do que o Ginger v1.4.4 suporta.
3. Identifique riscos técnicos.
4. Proponha a arquitetura final.
5. Crie os documentos de especificação.
6. Crie um plano dividido em tarefas pequenas.
7. Execute as tarefas em ordem.
8. Rode testes após cada bloco.
9. Corrija erros encontrados.
10. Atualize a documentação.
11. Mostre ao final:

* Arquivos criados.
* Arquivos alterados.
* Comandos executados.
* Testes realizados.
* Resultados.
* Pendências.
* Próxima tarefa recomendada.

Não pare apenas no planejamento, salvo quando houver uma impossibilidade técnica real.

Não afirme que algo funciona sem executar os testes correspondentes.

Mantenha o código idiomático, simples, legível e seguro.
