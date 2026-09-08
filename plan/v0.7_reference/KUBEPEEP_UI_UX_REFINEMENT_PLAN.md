# KubePeep — Refinamento de UI, navegação, filtros e ações Kubernetes

Faça uma revisão completa da interface atual do KubePeep com base no estado existente da aplicação.

O estilo visual atual ficou muito bom e DEVE SER PRESERVADO. Não quero um redesign completo. O objetivo agora é corrigir proporções, padronizar tipografia, melhorar o aproveitamento da tela, validar a navegação e tornar as ações sobre recursos Kubernetes realmente funcionais.

Antes de alterar qualquer coisa, revise a implementação atual, componentes, rotas, estados, filtros, chamadas ao backend e comportamento dos recursos.

Não crie regressões.

---

# 1. Padronização completa de fontes

Atualmente a fonte/tamanho usados nos:

- menus laterais;
- filtros;
- selects;
- navegação horizontal;
- botões;

estão excelentes.

Quero usar ESSA MESMA REFERÊNCIA VISUAL no restante da interface.

Revise:

- tabelas;
- nomes de recursos;
- namespaces;
- status;
- informações de Pods;
- cards;
- detalhes;
- modais;
- overlays;
- informações Kubernetes;
- formulários;
- YAML;
- ações;
- mensagens;
- tooltips;
- menus contextuais.

Não quero que algumas informações pareçam muito pequenas em comparação com os menus.

Crie tokens globais de tipografia e elimine tamanhos arbitrários espalhados pelos componentes.

A interface inteira deve utilizar:

- uma única família tipográfica;
- uma escala tipográfica consistente;
- pesos consistentes;
- line-height consistente.

Títulos podem continuar tendo hierarquia própria, porém o texto normal das informações deve utilizar o mesmo tamanho-base que hoje está funcionando bem nos menus e filtros.

---

# 2. Proporção da área de filtros

Os filtros atualmente estão ocupando espaço demais.

Quero que a área destinada a:

- filtros;
- controles;
- ordenação;
- saved filters;
- search;
- refresh;

utilize aproximadamente:

**20% a 30% da área útil da tela.**

E que a área destinada ao conteúdo principal utilize:

**70% a 80% da área útil.**

O conteúdo principal deve ser o protagonista.

Priorizar:

- tabelas;
- recursos;
- logs;
- detalhes;
- YAML;
- eventos;
- métricas;
- informações Kubernetes.

Não quero grandes blocos de filtros consumindo metade da tela.

Compacte verticalmente os filtros sem prejudicar a usabilidade.

Sempre que possível:

- manter filtros em uma ou duas linhas;
- diminuir espaços vazios;
- diminuir padding excessivo;
- agrupar controles relacionados;
- preservar legibilidade.

---

# 3. Manter os dois tipos de navegação

Gostei muito da combinação atual de:

## Menu lateral

Exemplo:

- Cluster
- Workloads
- Helm
- Network
- Configuration
- Storage

## Navegação horizontal

Dentro de uma categoria, por exemplo Network:

- Services
- Endpoints
- Ingresses
- IngressClasses
- EndpointSlices
- NetworkPolicies
- PortForwards

Quero manter esse conceito.

Porém revise TODOS os itens.

Cada clique precisa:

1. alterar corretamente o recurso selecionado;
2. carregar os dados correspondentes;
3. atualizar o estado visual;
4. atualizar filtros aplicáveis;
5. respeitar contexto/namespace;
6. manter a URL/rota/estado quando aplicável.

Hoje existem opções no menu lateral que aparentemente não executam corretamente a mudança esperada.

Faça uma validação item por item.

---

# 4. Namespace deve virar um filtro GLOBAL

Namespace não deve ficar repetido dentro de cada tela como filtro principal.

Quero uma barra global superior seguindo aproximadamente:

```text
Kubeconfig | Context | Scope | Namespace | Cluster/Identity | Search
```

Exemplo:

```text
~/.kube/config | dev | vmw-dev | totvs | support-element-communication
```

Adicionar um seletor global:

```text
Namespace
```

com:

```text
All
namespace-a
namespace-b
namespace-c
...
```

Esse namespace selecionado deve afetar globalmente todos os recursos namespaced.

Exemplos:

- Pods
- Deployments
- StatefulSets
- DaemonSets
- Jobs
- CronJobs
- Services
- Ingresses
- ConfigMaps
- Secrets
- PVCs
- NetworkPolicies
- etc.

Recursos cluster-scoped naturalmente devem ignorar esse filtro.

Exemplos:

- Nodes
- Namespaces
- StorageClasses
- ClusterRoles
- CRDs
- etc.

---

# 5. Scope configurado deve definir o contexto inicial

Quando existir um Scope de namespaces configurado no KubePeep, ele deve ser carregado automaticamente.

Exemplo:

Scope:

```text
totvs
```

Namespaces permitidos:

```text
support-element-communication
support-element-smartlink
framework-provisioning
```

Ao selecionar o Scope `totvs`, esses namespaces devem automaticamente se tornar o universo disponível para aquela sessão/contexto.

O seletor global de namespace deve mostrar:

```text
All
support-element-communication
support-element-smartlink
framework-provisioning
```

O `All` significa:

> todos os namespaces permitidos pelo Scope atual.

NÃO significa ignorar RBAC ou ultrapassar o escopo autorizado.

O KubePeep deve continuar respeitando rigorosamente:

- kubeconfig;
- contexto;
- RBAC;
- Scope;
- namespaces permitidos.

---

# 6. Hierarquia global de contexto

Quero uma hierarquia clara:

```text
Kubeconfig
   ↓
Context
   ↓
Scope
   ↓
Namespace
   ↓
Resource
```

Quando qualquer nível superior mudar, recarregar corretamente os inferiores.

Exemplo:

```text
Context mudou
   ↓
recalcular scopes
   ↓
recalcular namespaces
   ↓
recarregar recursos
```

Evitar estados inválidos ou filtros de um contexto anterior.

---

# 7. Ações rápidas nos recursos

Quero que o KubePeep deixe de ser somente uma interface de consulta e ofereça ações rápidas nos recursos quando elas fizerem sentido.

As ações devem ser contextuais.

NÃO mostrar uma ação apenas porque tecnicamente existe uma API para executá-la.

---

# 8. Pods

Para Pods, adicionar ações rápidas como:

```text
Logs
Exec
Port Forward
Delete
Restart
View YAML
Events
Describe / Details
Copy Name
Copy Namespace
```

Restart de Pod deve respeitar o comportamento Kubernetes.

Se o Pod possuir controller/owner, realizar a operação de forma segura para permitir sua recriação pelo controller.

Mostrar claramente ao usuário o que será feito.

Exemplo:

```text
Restart Pod
O Pod será removido e recriado pelo Deployment responsável.
```

Nunca executar ações destrutivas silenciosamente.

---

# 9. Deployments

Adicionar ações relevantes:

```text
Scale +
Scale -
Set replicas
Rollout Restart
Rollout Status
View Pods
View ReplicaSets
View YAML
Events
Edit
Delete
```

Replica deve poder ser alterada diretamente pela interface.

Exemplo:

```text
[-] 3 [+]
```

ou:

```text
Replicas: [ 3 ]
Apply
```

---

# 10. StatefulSets

Ações possíveis:

```text
Scale
Rollout Restart
View Pods
View PVCs
View YAML
Events
Edit
Delete
```

---

# 11. DaemonSets

Ações possíveis:

```text
Rollout Restart
View Pods
View YAML
Events
Edit
Delete
```

Não mostrar Scale convencional caso não faça sentido para o recurso.

---

# 12. Jobs e CronJobs

Jobs:

```text
View Pods
Logs
Events
View YAML
Delete
```

CronJobs:

```text
Run now
Suspend
Resume
View Jobs
View YAML
Events
Edit
Delete
```

---

# 13. Services

Ações possíveis:

```text
Port Forward
View Endpoints
View YAML
Edit
Delete
Copy ClusterIP
```

---

# 14. Ingresses

Ações relevantes:

```text
Open URL
Copy Host
View YAML
Edit
Events
Delete
```

Não adicionar ações sem sentido, como aumentar replicas.

---

# 15. Ações individuais e em massa

Onde fizer sentido, permitir selecionar múltiplos recursos utilizando checkbox.

Exemplo:

```text
☐ Pod A
☐ Pod B
☐ Pod C
```

Ao selecionar recursos, mostrar uma toolbar contextual.

Exemplo:

```text
3 Pods selected

[ View logs ] [ Restart ] [ Delete ]
```

Só habilitar ações compatíveis com TODOS os objetos selecionados.

Operações massivas destrutivas devem possuir confirmação clara.

---

# 16. RBAC e autorização

A UI deve refletir as permissões reais do usuário.

Se determinado usuário não puder:

```text
delete pods
```

não apresentar o botão como se a ação estivesse disponível.

Preferencialmente:

- ocultar a ação; ou
- desabilitar com tooltip explicando a ausência de permissão.

O backend deve SEMPRE validar novamente a autorização imediatamente antes da execução.

Nunca confiar apenas na UI.

---

# 17. Visualização de detalhes do recurso

Atualmente, ao clicar em um Pod, o detalhe aparece apenas em um painel estreito no canto direito.

Isso precisa mudar.

Ao clicar em:

- Pod;
- Deployment;
- Service;
- Ingress;
- Node;
- StatefulSet;
- DaemonSet;
- Job;
- CronJob;
- ConfigMap;
- Secret;
- ou qualquer outro recurso;

abrir uma área de detalhes grande e sobreposta ao conteúdo principal.

Quero aproximadamente:

```text
┌──────────────────────────────────────────┐
│                                          │
│          Resource Details                │
│                                          │
│                                          │
│                                          │
└──────────────────────────────────────────┘
```

ocupando aproximadamente **80% da viewport**, mantendo margem visual ao redor.

Não utilizar apenas um drawer estreito de 25%-30% no canto direito.

A intenção é fazer o recurso selecionado se tornar o foco da aplicação.

O restante da UI pode ficar escurecido/desfocado ao fundo.

---

# 18. Resource Workspace

Transforme essa visualização em algo próximo de um pequeno workspace do recurso.

Exemplo de Pod:

```text
Pod communication-service-core-xxxxx

Overview | Logs | YAML | Events | Metrics | Containers | Actions
```

Deployment:

```text
Deployment communication-service-core

Overview | Pods | ReplicaSets | YAML | Events | Rollout | Actions
```

Ingress:

```text
Ingress communication-service-core

Overview | Rules | Backends | YAML | Events | Actions
```

As abas devem variar dependendo do tipo de recurso.

---

# 19. Histórico de recursos abertos

Quero que os recursos abertos sejam preservados durante a navegação.

Exemplo:

```text
Deployment A
    ↓
Pod B
    ↓
Service C
```

Eu devo conseguir navegar:

```text
← voltar
→ avançar
```

entre os objetos anteriormente visualizados.

Algo semelhante ao comportamento de navegação já existente no aplicativo.

Se eu estiver vendo:

```text
Deployment
```

abrir um Pod relacionado e depois voltar, quero retornar ao Deployment anterior exatamente como estava.

Preservar:

- resource;
- namespace;
- aba ativa;
- filtros relevantes;
- posição quando possível.

---

# 20. Navegação entre recursos relacionados

Dentro dos detalhes, recursos relacionados devem ser clicáveis.

Exemplo:

```text
Deployment
 ├── ReplicaSet
 │    └── Pods
 ├── Service
 └── Ingress
```

Ao clicar em um recurso relacionado, ele deve abrir no mesmo Resource Workspace e entrar no histórico.

Isso deve permitir explorar Kubernetes naturalmente sem ficar voltando manualmente para listas.

---

# 21. Melhor aproveitamento horizontal

Em telas grandes, quero que o KubePeep aproveite toda a largura disponível.

Evitar situações onde:

```text
tabela ocupa 55%
espaço vazio ocupa 45%
```

As tabelas devem expandir.

Utilizar o espaço adicional para informações úteis como:

Pods:

```text
Namespace
Name
Status
Ready
Restarts
CPU
Memory
Node
Owner
Age
```

Deployments:

```text
Namespace
Name
Ready
Available
Replicas
Image
Age
Status
```

Ingress:

```text
Namespace
Name
Class
Hosts
Address
Ports
Age
```

Quando determinada coluna não couber, permitir configuração pelo menu:

```text
Columns
```

já existente.

---

# 22. Reduzir espaços mortos

Revise toda a UI buscando:

- paddings excessivos;
- margins excessivas;
- blocos muito altos;
- controles isolados;
- áreas vazias;
- tabelas pequenas demais.

O KubePeep deve continuar minimalista, mas ser muito eficiente em densidade de informação.

A referência conceitual continua sendo algo entre:

- Aptakube;
- OpenLens;
- k9s;

mantendo a identidade visual própria do KubePeep.

---

# 23. Estados dos botões

Padronizar semanticamente as cores.

### Azul

Ações primárias:

```text
Apply
Save
Open
Connect
Start
Refresh quando for ação principal
```

### Verde

Ações positivas:

```text
Start
Resume
Scale Up
Healthy
Running
Succeeded
```

### Vermelho

Ações destrutivas:

```text
Delete
Terminate
Stop
```

### Amarelo/Laranja

Atenção:

```text
Restart
Rollout Restart
Warning
Pending confirmation
```

Não exagerar nas cores.

Continuar mantendo a estética dark/minimalista atual.

---

# 24. Confirmação de ações destrutivas

Para Delete, restart em massa ou ações críticas, utilizar confirmação clara.

Exemplo:

```text
Delete Pod

communication-service-core-abc123
namespace: support-element-communication

This action cannot be undone.

[Cancel] [Delete Pod]
```

Para operações especialmente perigosas, considerar confirmação digitando o nome do recurso.

---

# 25. Loading e feedback

Toda ação precisa fornecer feedback.

Exemplo:

```text
Restarting deployment...
```

seguido de:

```text
Deployment restarted successfully.
```

ou erro real retornado pelo Kubernetes.

Nunca deixar o usuário clicar em um botão sem saber se a ação foi executada.

Utilizar:

- loading;
- toast;
- status;
- progress quando aplicável.

---

# 26. Validar funcionalidade completa

Faça uma revisão funcional, e não apenas visual.

Teste TODOS os menus laterais.

Teste TODAS as abas horizontais.

Teste:

```text
Cluster
Workloads
Helm
Network
Configuration
Storage
```

E todos os recursos existentes dentro deles.

Para cada tela, validar:

```text
navigation
fetch
filters
search
sorting
pagination
refresh
live updates
namespace
scope
saved filters
columns
details
actions
RBAC
error states
empty states
loading states
```

Não considere a tarefa concluída apenas porque a interface renderiza.

---

# 27. Responsividade

O foco principal é desktop/Wails, mas a UI deve reagir bem a diferentes resoluções.

Testar principalmente:

```text
1366x768
1440x900
1920x1080
2560x1440
```

O layout deve aproveitar espaço adicional sem aumentar exageradamente os componentes.

---

# 28. Preservar o estilo atual

IMPORTANTE:

O visual atual mostrado nas screenshots está muito próximo do que quero.

NÃO redesenhar toda a aplicação.

Preservar:

- dark theme;
- roxo do KubePeep;
- sidebar;
- navegação horizontal;
- cards suaves;
- bordas;
- status chips;
- estilo dos inputs;
- estilo dos selects;
- ícones;
- identidade atual.

O objetivo é:

```text
REFINAR
PADRONIZAR
COMPACTAR
CORRIGIR
TORNAR FUNCIONAL
```

e não substituir o design.

---

# 29. Design System

Antes de continuar criando estilos isolados, centralize tokens para:

```text
font-family
font-size
font-weight
line-height

spacing
padding
margin

border-radius
border-color

background
surface
surface-hover

primary
success
warning
danger

text-primary
text-secondary
text-muted
```

Evitar valores CSS arbitrários repetidos em dezenas de componentes.

---

# 30. Critérios de aceite

A implementação somente pode ser considerada concluída quando:

- toda a aplicação utilizar padrão tipográfico consistente;
- informações não ficarem menores que o necessário;
- filtros ocuparem no máximo ~20-30% da área útil;
- conteúdo utilizar aproximadamente 70-80%;
- namespace estiver no contexto global;
- existir opção `All`;
- Scope controlar os namespaces disponíveis;
- Scope configurado carregar automaticamente;
- navegação lateral estiver funcional;
- navegação horizontal estiver funcional;
- filtros realmente alterarem os resultados;
- tabelas aproveitarem melhor a largura disponível;
- Pods tiverem ações rápidas;
- Deployments tiverem Scale e Rollout;
- ações forem específicas ao tipo de recurso;
- houver operações em massa onde fizer sentido;
- RBAC for respeitado;
- detalhes dos recursos abrirem em workspace grande;
- não existir somente um drawer pequeno no canto;
- existir histórico de recursos abertos;
- for possível voltar/avançar entre recursos;
- recursos relacionados forem navegáveis;
- ações destrutivas exigirem confirmação;
- loading e feedback estiverem implementados;
- nenhuma funcionalidade existente tiver regressão.

---

# 31. Processo de implementação

Antes de implementar:

1. Vasculhe a estrutura atual do frontend e backend.
2. Identifique os componentes reutilizáveis existentes.
3. Identifique estilos duplicados.
4. Identifique navegações quebradas.
5. Identifique filtros que não alteram o backend/query.
6. Identifique ações Kubernetes já implementadas.
7. Identifique permissões/RBAC disponíveis.
8. Crie um plano de implementação.

Depois implemente em etapas pequenas.

Após cada etapa:

- build;
- lint;
- testes;
- validação da navegação;
- validação visual;
- validação das chamadas Kubernetes.

Não faça mocks para esconder funcionalidades quebradas.

Não remova funcionalidades existentes para simplificar o trabalho.

Não altere contratos do backend sem atualizar todos os consumidores.

Não introduza regressões.

Ao finalizar, gere também uma documentação resumindo:

- alterações realizadas;
- novos componentes;
- novo modelo de navegação;
- contexto global;
- Scope/Namespace;
- Resource Workspace;
- ações por tipo de recurso;
- operações em massa;
- RBAC;
- decisões de UI/UX;
- testes realizados;
- pendências encontradas.

---

# Diretriz arquitetural principal

A visualização de recursos deve evoluir de um simples painel lateral para um **Resource Workspace**.

A experiência desejada é:

```text
Deployment
   ↓
ReplicaSet
   ↓
Pod
   ↓
Logs / YAML / Events / Metrics
   ↓
Voltar para Pod
   ↓
Voltar para Deployment
```

O usuário deve conseguir navegar entre recursos relacionados sem perder contexto, mantendo histórico, filtros, namespace, scope e estado da tela.

Essa abordagem deve fazer o KubePeep se comportar mais como uma IDE Kubernetes minimalista do que como apenas uma tabela administrativa com drawers laterais.
