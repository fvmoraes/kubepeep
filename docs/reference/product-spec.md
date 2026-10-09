# Produto e experiência

Este documento descreve a base implementada. O [design system](../architecture/design-system.md)
define a direção visual; um item desenhado no menu não significa uma
funcionalidade entregue. Relatos de versões anteriores ficam no
[arquivo histórico](../archive/README.md).

## Propósito

KubePeep ajuda pessoas desenvolvedoras e de operação a localizar problemas em
Kubernetes usando o acesso que já possuem. Roda localmente como aplicativo
desktop ou servidor web em loopback, sem login próprio, impersonation ou
credenciais adicionais. O princípio é mostrar somente o que a identidade pode
acessar e executar somente o que Kubernetes autoriza.

O produto permite editar e salvar YAML de objetos existentes do catálogo nativo,
incluindo Secrets, ConfigMaps e Gateway API, com autorização de update,
confirmação e controle de concorrência. Visões dinâmicas de APIs/CRDs oferecem
YAML somente leitura. Não há criação ou aplicação arbitrária em lote.
Valores de Secret e documentos YAML exigem leitura explícita autorizada, sem
cache ou persistência. Dados do cluster, logs e sessões de terminal permanecem
em memória; exportações exigem ação explícita.

**Premissa básica:** atender operadores com permissões restritas, inclusive
sem poder listar namespaces ou administrar o cluster. Cadastrar namespaces
**em lote** como escopo local é uma jornada essencial: informar vários nomes,
revisar e salvar o conjunto, sem cadastro obrigatório um a um. Isso não cria
objetos Namespace no Kubernetes. Descoberta global é opcional; acesso efetivo
continua sujeito ao RBAC de cada recurso e namespace, conforme o
[contrato de permissões](rbac-requirements.md).

## Base disponível

| Área | Comportamento implementado |
| --- | --- |
| Runtime | Desktop Wails e web `serve`, frontend embutido, diagnósticos, controle da instância web, instalação e atualização explícitas |
| Seleção | Profiles de kubeconfig, contexto ativo e escopos `single`, `list` e `all`; importação validada e scope default obrigatório por contexto |
| Leitura | Paginação estratégica, cursor opaco, cancelamento, cache e watches sob demanda com limites de memória e autorização |
| Overview | Blocos independentes de saúde, problemas, restarts, workloads, eventos, scan limitado de logs e métricas opcionais |
| Workloads | Deployments, ReplicaSets, StatefulSets, DaemonSets, Jobs, CronJobs e Pods; visões dinâmicas de recursos nativos e CRDs fixadas por contexto |
| Network | Services, Ingresses, Endpoints, EndpointSlices, IngressClasses, NetworkPolicies, Gateway API e sessões de port-forward |
| Helm | Releases em Secrets/ConfigMaps, histórico, relações, valores/manifesto explícitos, upgrade de valores e rollback |
| Configuration | ConfigMaps com Data, metadados de Secrets e revelação explícita de Data autorizada; referências de Secret/ConfigMap e env nos detalhes de workloads |
| Operação | Events, logs atuais/anteriores/follow, editor YAML nativo autorizado, ações por kind, seleção em massa e exclusão em massa somente de Pods, exec e port-forward |
| Investigação | Problems Engine, relações locais, logs agregados por workload, busca local e diagnósticos de performance/cluster/namespace |
| Interface | Menus sincronizados, busca local/global, colunas iniciais compactas e personalização persistente, Auto fixo em 10 s, cinco inventários recentes em memória, tabelas virtualizadas e detalhes sobrepostos em 70% da altura da janela |
| Uso de recursos | Barras de CPU/memória com valor atual/configurado e percentual; alvo de HPA quando disponível, fallback de 80% e indisponibilidade explícita sem Metrics API |

O código de referência é a árvore de navegação em
[`web/src/navigation/tree.tsx`](../../web/src/navigation/tree.tsx), com rotas em
[`App.tsx`](../../web/src/App.tsx), e o [contrato da API](api.md).
Cluster, Storage, Access Control, Administration, Workloads, Network,
Configuration, Helm e Gateway API possuem rotas atuais. Gateway API depende das
APIs instaladas no cluster; Helm depende do armazenamento e das permissões da
identidade. Agregação simultânea de contextos não está implementada.

## Jornadas essenciais

1. **Abrir:** o desktop apresenta a janela; `serve` inicia a API local e pode
   abrir o navegador. A saúde local permanece separada da conexão Kubernetes.
2. **Escolher origem:** selecionar profile, contexto e escopo cancela consultas,
   streams e sessões da geração anterior. Respostas antigas nunca substituem
   a seleção atual. `--namespace` é uma seleção inicial efêmera. Add kubeconfig
   aceita arquivo/texto ou caminho local; cada contexto pode receber uma cor.
3. **Definir escopo:** nomes manuais não exigem `list namespaces`. O modo `all`
   exige essa permissão e usa somente os namespaces retornados pela API.
   Importação de uma lista valida tudo antes de persistir a transação.
   A lista inicia no namespace padrão; All é uma escolha explícita e prioriza
   esse namespace na coleta. Scopes legados sem padrão usam `default`, se
   presente, ou o primeiro disponível, sem uma consulta inicial em All.
4. **Investigar:** Overview leva à lista ou detalhe correspondente; filtros,
   paginação, cobertura parcial e origem permanecem visíveis. YAML e logs são
   carregados sob demanda e seguem limites de tamanho e duração.
5. **Agir:** capabilities orientam os controles, mas o backend reautoriza o
   alvo. Confirmações mostram contexto, namespace, recurso e consequência.
   Uma ação aceita não significa que o rollout ou outra operação assíncrona
   já terminou.
6. **Encerrar:** sair do fluxo, trocar geração ou fechar a aplicação encerra as
   sessões associadas. Atualizar ou remover exige ação explícita; desinstalação
   preserva dados locais por padrão.

## Estados e autorização

| Estado | Informação para a pessoa usuária |
| --- | --- |
| Carregando | A geração atual ainda não recebeu resposta; mostrar skeleton compacto |
| Vazio | Consulta permitida e concluída sem resultados; oferecer ajuste do filtro |
| Offline | Dependência Kubernetes indisponível, com motivo sanitizado e retry |
| Proibido | Negação autoritativa do Kubernetes; abrir Permissions quando útil |
| Desconhecido | Revisão de autorização inconclusiva; ação indisponível até nova avaliação |
| Parcial | Preservar resultados válidos e indicar origens/blocos que falharam |
| Cancelado | Descartar trabalho antigo sem toast de erro |
| Truncado | Explicitar limite e cobertura; permitir refinar filtro ou paginar |
| Obsoleto | Identificar dados anteriores durante refresh, sem misturar gerações |

`FORBIDDEN` representa negação autoritativa. Uma resposta HTTP 403 por
`CSRF_REJECTED` é rejeição local distinta; timeout de autorização produz
capability `unknown`, nunca uma permissão inventada. Uma leitura real limitada
pode comprovar acesso quando a revisão é inconclusiva; um 403 explícito impede
essa recuperação. Secret tem DTO padrão de metadados e rotas separadas de Data
e YAML, carregadas somente por ação explícita.

## Interface e persistência

A navegação agrupa Cluster, Workloads, Helm, Network, Configuration, Storage,
Access Control, Observability e Administration, com Settings separado. Grupos
podem conter recursos ainda indisponíveis. Tokens, semântica de cores,
tipografia e componentes estão no [design system](../architecture/design-system.md).

A paleta escura e o roxo KubePeep definem o estilo aprovado, documentado no
[design system](../architecture/design-system.md).
A seleção All namespaces e os dados ilustrados não representam permissões
presumidas nem devem substituir a cobertura real do escopo consultado.

Lista e filtros ocupam toda a área útil. Detalhes abrem sobre a lista, ancorados
embaixo e fixos em 70% da altura da janela. Fechar remove a sobreposição.
Relações selecionam a família/namespace do destino, filtram seu nome exato e
abrem o detalhe correspondente. O guia de [workspaces](../guides/resource-workspaces.md)
descreve as jornadas e os limites.

Preferências, filtros e navegação usam schema fechado no SQLite; não usam
`localStorage` nem `sessionStorage`. Toda nova chave precisa de contrato,
limite e validação no [modelo de dados](../architecture/data-model.md). Visibilidade
e ordem das colunas persistem por coleção, sem expiração; o set inicial é usado
somente na ausência de personalização. Cores persistem por profile/contexto,
e visões customizadas por profile/contexto/cluster. Dados dos inventários
permanecem apenas em memória e são invalidados na troca de geração.

## Nomes e distribuição

- Produto: **KubePeep**; módulo Go: `github.com/fvmoraes/kubepeep`.
- CLI archives e instaladores por script: `kubePeep` / `kubePeep.exe`.
- Pacotes Linux do workflow: comando `kubepeep` em `/usr/bin`.
- Dados: `~/.kubePeep/` no Unix e `%LOCALAPPDATA%\kubePeep\` no Windows.
- Releases atuais: tags SemVer sem prefixo `v`, arquivos `kubepeep-<os>-<arch>`
  e variantes de pacote; scripts exigem versão explícita e SHA-256.

As matrizes e exemplos de instalação estão em [download.md](../download.md).
O workflow de release é a fonte dos nomes de artefatos; pesquisas e relatos
históricos não substituem o contrato atual.
