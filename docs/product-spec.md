# Produto e experiência

Este documento descreve a base implementada. A entrega da versão 1, seus
recursos pendentes e critérios de aceite estão no [plano v1](../plan/README.md).
A [referência UI/UX](../plan/reference/KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md)
define a direção; um item desenhado no menu não significa uma funcionalidade entregue.

## Propósito

KubePeep ajuda pessoas desenvolvedoras e de operação a localizar problemas em
Kubernetes usando o acesso que já possuem. Roda localmente como aplicativo
desktop ou servidor web em loopback, sem login próprio, impersonation ou
credenciais adicionais. O princípio é mostrar somente o que a identidade pode
acessar e executar somente o que Kubernetes autoriza.

O produto não aplica YAML genérico, não gerencia credenciais e nunca apresenta
valores de Secret. Dados do cluster, logs de aplicação e sessões de terminal
permanecem em memória; exportações exigem ação explícita.

**Premissa básica:** atender operadores com permissões restritas, inclusive
sem poder listar namespaces ou administrar o cluster. Cadastrar namespaces
**em lote** como escopo local é uma jornada essencial: informar vários nomes,
revisar e salvar o conjunto, sem cadastro obrigatório um a um. Isso não cria
objetos Namespace no Kubernetes. Descoberta global é opcional; acesso efetivo
continua sujeito ao RBAC de cada recurso e namespace. A [Fase 0 da v1](../plan/v1/phase-00-acesso-restrito-e-lote.md)
protege esse comportamento e planeja os refinamentos da experiência existente.

## Base disponível

| Área | Comportamento implementado |
| --- | --- |
| Runtime | Desktop Wails e web `serve`, frontend embutido, diagnósticos, controle da instância web, instalação e atualização explícitas |
| Seleção | Profiles de kubeconfig, contexto ativo e escopos de namespaces `single`, `list` e `all`; importação validada de listas |
| Overview | Blocos independentes de saúde, problemas, restarts, workloads, eventos, scan limitado de logs e métricas opcionais |
| Workloads | Deployments, StatefulSets, DaemonSets, Jobs e CronJobs; Pods em lista e detalhe próprios |
| Network | Services, Ingresses, EndpointSlices e sessões de port-forward |
| Configuration | ConfigMaps e metadados allowlisted de Secrets |
| Operação | Events, logs atuais/anteriores/follow, YAML permitido somente leitura, restart de Deployment, scale de Deployment/StatefulSet, delete de Pod e exec |
| Interface | Tokens e componentes compartilhados, tabelas/detalhes com resource framework, grupos de navegação, paleta, favoritos/recentes e preferências allowlisted |

O código de referência é a árvore de navegação em
[`web/src/navigation/tree.tsx`](../web/src/navigation/tree.tsx), com rotas em
[`App.tsx`](../web/src/App.tsx), e o [contrato da API](api.md).
Itens sem `path` permanecem indisponíveis. As famílias adicionais de Cluster,
Storage, Access Control e Administration, junto das lacunas de Workloads,
Network e Configuration, pertencem ao plano v1. Helm e Gateway API são
extensões condicionais, não promessas da base atual. Agregação simultânea de
contextos não é funcionalidade entregue nem bloqueio da v1.

## Jornadas essenciais

1. **Abrir:** o desktop apresenta a janela; `serve` inicia a API local e pode
   abrir o navegador. A saúde local permanece separada da conexão Kubernetes.
2. **Escolher origem:** selecionar profile, contexto e escopo cancela consultas,
   streams e sessões da geração anterior. Respostas antigas nunca substituem
   a seleção atual. `--namespace` é uma seleção inicial efêmera.
3. **Definir escopo:** nomes manuais não exigem `list namespaces`. O modo `all`
   exige essa permissão e usa somente os namespaces retornados pela API.
   Importação de uma lista valida tudo antes de persistir a transação.
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
capability `unknown`, nunca uma permissão inventada. Secret tem DTO próprio
de metadados e não possui endpoint YAML.

## Interface e persistência

A navegação agrupa Cluster, Workloads, Helm, Network, Configuration, Storage,
Access Control, Observability e Administration, com Settings separado. Grupos
podem conter recursos ainda indisponíveis. Tokens, semântica de cores,
tipografia e componentes estão no [design system](design-system.md).

A imagem KubePeep.png fornecida pelo usuário define o estilo desejado para a
v1, registrado na [direção visual aprovada](../plan/reference/direcao-visual-e-premissa-de-acesso.md).
A seleção All namespaces e os dados ilustrados não representam permissões
presumidas nem devem substituir a cobertura real do escopo consultado.

Preferências, filtros e navegação usam schema fechado no SQLite; não usam
`localStorage` nem `sessionStorage`. Toda nova chave precisa de contrato,
limite e validação no [modelo de dados](data-model.md). Configuração da janela
e ampliação das preferências do shell seguem o plano e não devem ser
apresentadas como persistidas antes da implementação.

## Nomes e distribuição

- Produto: **KubePeep**; módulo Go: `github.com/fvmoraes/kubepeep`.
- CLI archives e instaladores por script: `kubePeep` / `kubePeep.exe`.
- Pacotes Linux do workflow: comando `kubepeep` em `/usr/bin`.
- Dados: `~/.kubePeep/` no Unix e `%LOCALAPPDATA%\kubePeep\` no Windows.
- Releases atuais: tags SemVer sem prefixo `v`, arquivos `kubepeep-<os>-<arch>`
  e variantes de pacote; scripts exigem versão explícita e SHA-256.

As matrizes e exemplos de instalação estão em [download.md](download.md).
O workflow de release é a fonte dos nomes de artefatos; pesquisas e relatos
históricos não substituem o contrato atual.
