# KubePeep — UI/UX, Design System e expansão dos recursos Kubernetes

Quero realizar uma revisão e implementação completa da interface do KubePeep.

Use como referência conceitual de organização e usabilidade:

- Aptakube
- OpenLens / Lens

NÃO copiar a interface dessas aplicações.
O KubePeep deve manter sua identidade própria, minimalista, escura, moderna e baseada no atual visual purple do projeto.

Antes de alterar qualquer código:

1. Analise toda a estrutura atual do frontend.
2. Identifique componentes reutilizáveis existentes.
3. Identifique duplicações de CSS, estilos inline e valores hardcoded.
4. Analise todas as telas existentes.
5. Identifique inconsistências de:
   - fonte
   - tamanho
   - espaçamento
   - alinhamento
   - botões
   - inputs
   - cards
   - tabelas
   - estados
   - navegação
6. Verifique como Wails está integrando frontend e backend.
7. Preserve funcionalidades existentes.
8. Não faça apenas alterações cosméticas isoladas.
9. Crie um Design System centralizado e reutilizável.

A aplicação inteira deve parecer um único produto coerente.

---

# 1. Objetivo visual

Quero que o KubePeep tenha aparência de uma ferramenta profissional de Kubernetes para uso diário.

A interface deve ser:

- minimalista
- compacta
- organizada
- consistente
- moderna
- rápida de interpretar
- adequada para muitas horas de uso
- adequada para telas de notebook
- adequada para monitores grandes
- sem elementos desnecessariamente grandes
- sem componentes "espremidos"
- sem diferenças de estilo entre páginas

Inspirar-se na densidade e organização de:

Aptakube
OpenLens
Lens

Mas mantendo o KubePeep visualmente próprio.

---

# 2. Fonte única na aplicação

Atualmente há sensação de que diferentes famílias/tipos de fonte são utilizados.

Quero UMA única família tipográfica para toda a aplicação.

Ela deverá ser aplicada em:

- títulos
- subtítulos
- menu lateral
- botões
- labels
- inputs
- tabelas
- cards
- badges
- tooltips
- dropdowns
- filtros
- logs
- mensagens
- dashboard
- Kubernetes YAML
- nomes de recursos

Escolha uma fonte moderna e altamente legível.

Preferência:

Inter

ou outra equivalente caso já exista uma fonte adequada no projeto.

Não misturar:

sans-serif + monospace

como padrão visual da aplicação.

Monospace poderá ser usada SOMENTE onde tecnicamente fizer sentido:

- YAML
- JSON
- logs
- comandos
- valores técnicos muito específicos

Todo o restante deverá utilizar a família principal.

---

# 3. Escala tipográfica global

Criar tokens de tipografia.

Não definir tamanhos arbitrariamente em cada componente.

Criar uma escala consistente semelhante a:

```css
--font-xs: 11px;
--font-sm: 12px;
--font-md: 13px;
--font-base: 14px;
--font-lg: 16px;
--font-xl: 20px;
--font-2xl: 26px;
--font-3xl: 32px;
```

Avaliar os valores durante a implementação para garantir boa legibilidade.

Uso esperado:

11–12px  
metadados, badges, labels secundários.

13px  
menus, filtros, tabelas e elementos compactos.

14px  
texto padrão.

16px  
subtítulos e cabeçalhos de cards.

20–26px  
títulos das páginas.

Não utilizar fontes exageradamente grandes.

A aplicação precisa manter alta densidade de informação.

Também padronizar:

- font-weight
- line-height
- letter-spacing

---

# 4. Hierarquia das cores de texto

O rosa atualmente utilizado em muitos textos está forte demais.

Quero reduzir drasticamente esse efeito.

Criar aproximadamente esta hierarquia:

Texto principal:
quase branco.

Exemplo:

```text
#F4F1F7
```

Texto secundário:

```text
#C8C2D0
```

Texto terciário:

```text
#918A9E
```

Texto desabilitado:

```text
#686271
```

Purple/pink deve continuar existindo como identidade do KubePeep, mas não deve dominar todos os textos.

Usar purple principalmente para:

- item selecionado
- highlights
- links
- logo
- pequenos detalhes
- foco
- indicadores especiais

O texto principal das telas deve ficar muito mais próximo de branco.

---

# 5. Background e cards

Atualmente alguns cards parecem muito purple/cinza pesado.

Quero backgrounds mais neutros e suaves.

Criar uma escala semelhante a:

background principal:

```text
#111016
```

sidebar:

```text
#0E0D13
```

surface:

```text
#18161E
```

surface elevated:

```text
#1D1A24
```

surface hover:

```text
#25212D
```

border:

```text
#302A3A
```

Evitar caixas excessivamente roxas.

Purple deverá aparecer principalmente como detalhe da identidade.

Os cards precisam ficar mais discretos para que os dados sejam os elementos de maior destaque.

---

# 6. Cores semânticas dos botões

Parar de utilizar purple como única cor para praticamente todas as ações.

Criar cores semânticas.

## Azul

Usar para ações normais/primárias:

- Apply
- Refresh
- Open
- View
- Connect
- Select
- Create
- Edit

Exemplo:

```text
#3B82F6
```

## Verde

Usar para:

- sucesso
- healthy
- connected
- start
- enable
- save quando claramente positivo
- running

Exemplo:

```text
#22C55E
```

## Vermelho

Usar para:

- delete
- stop
- disconnect
- destructive actions
- errors
- failed resources

Exemplo:

```text
#EF4444
```

## Amarelo / âmbar

Usar para:

- warnings
- stale
- pending
- degraded

Exemplo:

```text
#F59E0B
```

Purple permanece como:

- branding
- seleção
- navegação
- destaque visual do KubePeep

Não transformar a aplicação em uma interface multicolorida.

Usar cores semânticas somente quando houver significado.

---

# 7. Botões

Criar um componente único de Button.

Variantes:

- Primary
- Secondary
- Success
- Danger
- Warning
- Ghost
- Icon

Padronizar:

- altura
- padding
- border-radius
- fonte
- ícones
- hover
- focus
- active
- disabled

Evitar botões de tamanhos diferentes sem necessidade.

Sugestão:

```text
small: 28px
normal: 32px
large: 36px
```

Na maioria da aplicação utilizar 32px.

---

# 8. Inputs e filtros

Criar padrão único para:

- input
- select
- combobox
- search
- textarea
- checkbox
- radio
- toggle

Todos devem possuir:

- mesma altura
- mesmo border-radius
- mesma borda
- mesmo background
- mesmo font-size
- mesmos estados de foco

Evitar telas como Pods, Events e Workloads terem layouts de filtros visualmente diferentes.

Criar um componente compartilhado:

```tsx
<ResourceFilters />
```

ou equivalente.

---

# 9. Header principal

O header atual contém:

- Kubeconfig
- Cluster
- Context
- Select
- Health

Reorganizar para reduzir ocupação horizontal.

Estrutura sugerida:

```text
[Kubeconfig] [Cluster] [Context] [Namespace Scope] [status]
```

O botão Select só deverá existir se realmente houver necessidade de confirmação.

Caso selecionar um Context já seja suficiente, remover etapas desnecessárias.

Garantir que nenhum elemento ultrapasse a janela.

---

# 10. Logo + versão

Na sidebar atualmente temos:

```text
KubePeep
local cluster view
```

Quero alterar para:

```text
[LOGO] KubePeep
       v0.2.2
```

A versão deve ser obtida automaticamente da aplicação/build.

NÃO hardcode:

```text
v0.2.2
```

A versão deverá vir de uma fonte única compartilhada com o build/release do projeto.

Exemplo visual:

```text
KubePeep
v0.2.2
```

A versão deve aparecer discretamente abaixo do nome.

Sugestão:

```text
font-size: 11px
color: texto terciário
```

Se for development:

```text
KubePeep
dev
```

ou:

```text
KubePeep
0.2.2-dev
```

---

# 11. Sidebar nova

A sidebar precisa evoluir para uma estrutura semelhante conceitualmente a Aptakube/OpenLens.

Hoje existem:

- Overview
- Workloads
- Pods
- Logs
- Events
- Network
- Config
- Namespaces
- Permissions
- Settings

Quero reorganizar em categorias Kubernetes.

A sidebar deve suportar:

- grupos
- expand/collapse
- submenus
- scroll
- indicação da página ativa
- ícones
- tooltips quando compactada
- estado persistido localmente

---

# 12. Estrutura da navegação Kubernetes

Implementar aproximadamente:

## Cluster

- Overview
- Nodes
- Events
- Namespaces
- Leases

---

## Workloads

- Overview
- Deployments
- Pods
- ReplicaSets
- DaemonSets
- StatefulSets
- Jobs
- CronJobs

---

## Helm

- Releases

Se Helm ainda não estiver implementado, preparar a arquitetura/navigation para implementação posterior.

---

## Network

- Services
- Endpoints
- EndpointSlices
- Ingresses
- IngressClasses
- NetworkPolicies

### Gateway API

Quando Gateway API estiver disponível:

- Gateways
- GatewayClasses
- HTTPRoutes
- GRPCRoutes

- Port Forwarding

---

## Configuration

- ConfigMaps
- Secrets
- ResourceQuotas
- LimitRanges
- HorizontalPodAutoscalers
- PodDisruptionBudgets

---

## Storage

- PersistentVolumes
- PersistentVolumeClaims
- VolumeAttachments
- StorageClasses
- CSINodes
- CSIDrivers

Quando suportado:

- VolumeAttributesClasses

---

## Access Control

- ServiceAccounts
- Roles
- RoleBindings
- ClusterRoles
- ClusterRoleBindings
- Permissions

Manter a funcionalidade de Permissions existente integrada nesta área.

---

## Administration

- CustomResourceDefinitions
- PriorityClasses
- RuntimeClasses
- MutatingWebhookConfigurations
- ValidatingWebhookConfigurations

Quando suportado pela versão Kubernetes:

- ValidatingAdmissionPolicies
- ValidatingAdmissionPolicyBindings

---

## Observability

- Logs
- Events

Avaliar se Events ficará somente em Cluster ou também poderá ser acessado por Observability.

Evitar duplicação real de implementação.

---

## Application

- Settings

---

# 13. Recursos Cluster Scoped

A arquitetura precisa compreender corretamente recursos:

- Namespace scoped
- Cluster scoped

Exemplo:

- Deployment
- Pod
- Service
- ConfigMap

dependem de namespace.

Enquanto:

- Nodes
- Namespaces
- PersistentVolumes
- ClusterRoles
- CRDs
- StorageClasses

são cluster scoped.

Não forçar `namespace scope` em recursos cluster-scoped.

---

# 14. Página genérica de recursos Kubernetes

Não quero implementar cada recurso com componentes completamente independentes.

Criar uma arquitetura reutilizável.

Exemplo conceitual:

```tsx
<ResourcePage>
<ResourceToolbar>
<ResourceFilters>
<ResourceTable>
<ResourceStatus>
<ResourceDetails>
<ResourceYaml>
<ResourceEvents>
```

As páginas deverão reaproveitar esta estrutura.

---

# 15. Visualização de recursos

Ao clicar em um recurso Kubernetes, abrir detalhes.

Estrutura desejada:

- Overview
- Details
- YAML
- Events

Dependendo do recurso:

- Logs
- Containers
- Metrics
- Conditions
- Volumes
- Environment
- Ports

Exemplo para Pod:

- Overview
- Containers
- Logs
- Events
- YAML

Deployment:

- Overview
- Pods
- Events
- YAML

Service:

- Overview
- Endpoints
- YAML

---

# 16. Tabelas

Recursos Kubernetes devem utilizar tabelas compactas.

Exemplo Pods:

- Name
- Namespace
- Ready
- Status
- Restarts
- Age
- Node

Deployments:

- Name
- Namespace
- Ready
- Up-to-date
- Available
- Age

Services:

- Name
- Namespace
- Type
- Cluster IP
- External IP
- Ports
- Age

Padronizar:

- altura de linha
- header
- sorting
- hover
- seleção
- menus de contexto
- empty state
- loading

---

# 17. Actions

Permitir ações contextuais de forma organizada.

Exemplo Pod:

- View logs
- Describe
- View YAML
- Delete

Deployment:

- Scale
- Restart rollout
- View YAML
- Delete

Service:

- Port forward
- View YAML
- Delete

Sempre respeitar RBAC.

Ações não permitidas deverão:

- aparecer desabilitadas

ou

- não aparecer

conforme decisão arquitetural.

Nunca tentar executar silenciosamente uma operação que o usuário não possui permissão.

---

# 18. Status visual

Padronizar status Kubernetes.

Running / Healthy:
verde.

Warning / Pending:
amarelo.

Failed / Error:
vermelho.

Unknown:
cinza.

Informational:
azul.

Não utilizar purple para representar health.

---

# 19. Layout

Todas as telas precisam compartilhar exatamente a mesma estrutura.

Definir:

```text
SIDEBAR_WIDTH
HEADER_HEIGHT
CONTENT_MAX_WIDTH
PAGE_PADDING
SECTION_GAP
CARD_PADDING
CONTROL_HEIGHT
```

Não utilizar valores diferentes arbitrariamente.

A mudança entre:

- Overview
- Workloads
- Pods
- Events
- Network
- Config
- Namespaces
- Permissions
- Settings

não pode causar sensação de mudança de aplicativo.

---

# 20. Responsividade desktop

O KubePeep é uma aplicação desktop Wails.

Priorizar:

- 1280x720
- 1366x768
- 1440x900
- 1920x1080
- 2560x1440

A interface precisa continuar utilizável em 1280x720.

Não permitir:

- botão cortado
- input ultrapassando container
- menu inacessível
- header quebrando
- scroll horizontal da aplicação inteira

Quando necessário, tabelas podem possuir scroll horizontal interno.

---

# 21. Sidebar compactável

Adicionar possibilidade de recolher a sidebar.

Expandida:

- ícone + texto + submenus.

Compacta:

- somente ícones.

Hover:

- tooltip mostrando o nome.

Persistir preferência localmente.

---

# 22. Namespace Scope

O Namespace Scope é uma funcionalidade importante e diferenciada do KubePeep.

Preservá-la.

Porém melhorar a UX.

O usuário precisa entender claramente:

- Cluster
- Context
- Namespace Scope

Exemplo no header:

```text
vmw-dev
Finance workloads
```

ou:

```text
vmw-dev / finance
```

Se Scope = All:

```text
vmw-dev / All namespaces
```

Evitar mensagens pouco amigáveis como:

```text
GENERATION_CHANGED: No active Kubernetes resource scope is available.
```

Para o usuário final usar algo como:

```text
No namespace scope selected.
```

Detalhes técnicos podem ficar:

- tooltip
- details
- debug log

---

# 23. Empty states

Padronizar empty states.

Não utilizar grandes caixas vazias sem necessidade.

Exemplo:

```text
No namespace scope selected

Select or create a namespace scope to view workloads.

[Select scope]
```

Outro exemplo:

```text
No Pods found

No pods match the current filters.

[Clear filters]
```

---

# 24. Erros

Criar padrão:

- ErrorBanner
- WarningBanner
- InfoBanner
- SuccessBanner

Erro:

- ícone vermelho
- título simples
- mensagem legível
- detalhes técnicos opcionalmente expansíveis

Evitar expor códigos internos como mensagem principal.

---

# 25. Overview

O dashboard atual deverá continuar compacto.

Organizar cards como:

- Namespaces
- Pods
- Healthy
- Problem Pods
- Degraded Workloads
- Restarts
- Warning Events
- Log Matches

Reduzir quantidade de bordas e cores.

Destaque deve estar nos números.

Exemplo:

```text
24
Pods
```

e não em caixas excessivamente chamativas.

---

# 26. Logs

Manter Logs como funcionalidade importante.

Melhorar layout.

Área de logs deve aproveitar mais espaço vertical.

Controles:

- Namespace
- Pod
- Container
- Tail
- Since
- Previous container

ações:

- Read
- Follow
- Stop

viewer:

- Search
- Level
- Wrap
- Timestamps
- Copy
- Download
- Clear

Usar fonte monospace SOMENTE no conteúdo do log.

Os controles devem usar a fonte principal.

---

# 27. Consistência de ícones

Padronizar uma única biblioteca de ícones.

Não misturar bibliotecas.

Todos devem seguir:

```text
16px normalmente
18px para navegação quando necessário
```

stroke width consistente.

---

# 28. Design Tokens

Centralizar tokens.

Exemplo conceitual:

- colors
- typography
- spacing
- radius
- borders
- shadows
- control sizes
- z-index

Exemplo:

```text
--space-1
--space-2
--space-3
--space-4
--space-6
--space-8

--radius-sm
--radius-md
--radius-lg

--control-sm
--control-md
--control-lg
```

Evitar:

```css
padding: 13px 17px;
margin: 19px;
font-size: 13.5px;
```

espalhados arbitrariamente.

---

# 29. Componentes compartilhados

Identificar e criar componentes reutilizáveis, como:

- Button
- IconButton
- Input
- Select
- Checkbox
- Badge
- StatusBadge
- Card
- Table
- Tabs
- Tooltip
- Dialog
- Drawer
- ContextMenu
- SearchInput
- ResourceTable
- ResourceHeader
- FilterBar
- EmptyState
- ErrorState
- LoadingState
- PageHeader
- SidebarGroup
- SidebarItem

Não criar abstrações exageradamente complexas.

---

# 30. Não duplicar código

Workloads, Pods, Network, Config e demais recursos possuem muitas características iguais.

Reutilize estruturas.

Idealmente a implementação de novos recursos Kubernetes deverá exigir principalmente:

- definição das colunas
- endpoint/backend
- filtros
- ações específicas

e não uma nova página inteira feita do zero.

---

# 31. Performance

A expansão da quantidade de recursos Kubernetes não deve fazer o KubePeep ficar pesado.

Considerar:

- virtualização de listas/tabelas quando necessário
- paginação
- watch Kubernetes controlado
- cancelamento de requests anteriores
- cache temporário
- debounce de search
- não disparar watchers desnecessários

---

# 32. Kubernetes API

Não implementar utilizando execuções de `kubectl` para funcionalidades principais se o projeto já utiliza Kubernetes Client/API.

Preferir Kubernetes API/client oficial.

O comportamento deve respeitar:

- kubeconfig
- context
- RBAC
- namespace scope
- timeouts
- cancelamento

---

# 33. Segurança

Secrets nunca devem aparecer acidentalmente em:

- Overview
- table
- logs da aplicação
- telemetria

Na listagem de Secrets mostrar apenas metadata.

Para visualização de conteúdo, manter a política de segurança atual do projeto ou exigir ação explícita.

Nunca armazenar credenciais Kubernetes desnecessariamente.

---

# 34. Site KubePeep

A identidade visual do aplicativo deve permanecer consistente com o site oficial.

Porém não precisam ser exatamente iguais.

Compartilhar conceitualmente:

- logo
- purple da marca
- tipografia
- cores principais
- nomenclatura

O site possui visual mais promocional.

O desktop precisa ser mais técnico e neutro.

---

# 35. Nome da aplicação

Padronizar em todos os lugares:

```text
KubePeep
```

Não usar variações como:

```text
Kube Peep
kubepeep
Kubepeep
```

exceto quando tecnicamente necessário em:

- package
- binary
- URL
- nome de arquivo

Na interface sempre:

```text
KubePeep
```

Verificar inclusive o title da janela Wails.

---

# 36. Resultado esperado da sidebar

Conceitualmente:

```text
KubePeep
v0.2.2

─────────────

Cluster
  Overview
  Nodes
  Events
  Namespaces
  Leases

Workloads
  Overview
  Deployments
  Pods
  ReplicaSets
  DaemonSets
  StatefulSets
  Jobs
  CronJobs

Helm
  Releases

Network
  Services
  Endpoints
  EndpointSlices
  Ingresses
  IngressClasses
  NetworkPolicies
  Gateway API
  Port Forwarding

Configuration
  ConfigMaps
  Secrets
  ResourceQuotas
  LimitRanges
  Horizontal Pod Autoscalers
  Pod Disruption Budgets

Storage
  Persistent Volumes
  Persistent Volume Claims
  Volume Attachments
  Storage Classes
  CSI Nodes
  CSI Drivers

Access Control
  Service Accounts
  Roles
  Role Bindings
  Cluster Roles
  Cluster Role Bindings
  Permissions

Observability
  Logs

Administration
  CRDs
  Priority Classes
  Runtime Classes
  Admission Webhooks

─────────────

Settings
```

---

# 37. Ordem de implementação

Realizar por fases.

## Fase 1 — Auditoria

Analisar frontend atual.

Documentar inconsistências.

Identificar componentes compartilháveis.

---

## Fase 2 — Design System

Criar:

- cores
- fonte
- escala tipográfica
- spacing
- buttons
- inputs
- cards
- badges
- status
- tables

---

## Fase 3 — Shell da aplicação

Padronizar:

- sidebar
- header
- page container
- page header
- scroll
- responsive desktop

Adicionar versão abaixo do logo.

---

## Fase 4 — Navegação

Criar nova árvore de recursos Kubernetes.

Implementar grupos recolhíveis.

---

## Fase 5 — Resource Framework

Criar componentes genéricos para recursos Kubernetes.

---

## Fase 6 — Recursos principais

Primeiro:

- Nodes
- Deployments
- Pods
- ReplicaSets
- DaemonSets
- StatefulSets
- Jobs
- CronJobs

Depois:

- Network
- Configuration
- Storage
- Access Control
- Administration

---

## Fase 7 — UX

Revisar:

- loading
- errors
- empty states
- tooltips
- status
- RBAC
- ações
- feedback visual

---

## Fase 8 — Validação

Testar todas as páginas nas resoluções:

- 1280x720
- 1366x768
- 1440x900
- 1920x1080

Capturar screenshots das principais telas e comparar visualmente.

---

# 38. Critérios obrigatórios de conclusão

A tarefa somente estará concluída quando:

- toda a aplicação utilizar uma família principal de fonte;
- existir uma escala tipográfica global;
- não houver tamanhos arbitrários de fonte espalhados pela UI;
- KubePeep aparecer sempre escrito corretamente;
- a versão real aparecer abaixo do logo;
- purple/rosa não for mais a cor dominante dos textos;
- textos principais forem próximos ao branco;
- cards forem mais neutros e suaves;
- botões utilizarem cores semânticas;
- verde representar sucesso/healthy;
- vermelho representar erro/destrutivo;
- azul representar ações normais;
- amarelo representar warning;
- sidebar possuir categorias Kubernetes;
- recursos cluster-scoped funcionarem sem namespace scope;
- recursos namespace-scoped respeitarem o scope ativo;
- todas as telas utilizarem o mesmo layout;
- filtros utilizarem componentes compartilhados;
- tabelas possuírem aparência consistente;
- não existir scroll horizontal global;
- layout funcionar em 1280x720;
- funcionalidades atuais continuarem funcionando;
- Wails continuar funcionando;
- build continuar funcionando;
- testes existentes continuarem passando.

---

# 39. Regra principal

Não quero apenas "deixar mais bonito".

Quero transformar a interface atual em um sistema visual e arquitetural consistente.

Cada novo recurso Kubernetes adicionado no futuro deve naturalmente seguir o mesmo padrão sem exigir novos estilos, novos layouts e novos componentes redundantes.

O resultado precisa transmitir:

> "KubePeep é uma aplicação Kubernetes profissional, compacta e simples."

e não:

> "cada página foi criada separadamente".

Preserve a identidade atual do KubePeep, principalmente:

- logo
- dark theme
- purple como marca

mas torne a aplicação muito mais neutra, legível, organizada e consistente.

---

# 40. Diretriz visual final

Uma mudança especialmente importante é tirar o purple da função de “cor para tudo”.

Manter purple em:

- logo
- item selecionado da sidebar
- links
- foco
- pequenos highlights

Usar:

- branco e cinza neutro para a maior parte da interface;
- azul para ações normais;
- verde para sucesso/healthy;
- vermelho para erro/destrutivo;
- amarelo para warning/pending.

O objetivo é aproximar o KubePeep da sensação profissional, organizada e técnica de ferramentas como Aptakube e OpenLens e lens, sem perder sua identidade própria, e completo e previsivel como um k9s.
