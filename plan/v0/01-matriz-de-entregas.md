# Matriz de entregas e aceite 1.0

Fonte: [especificação original](../reference/KubePeep_UI_UX_Design_System_e_Recursos_Kubernetes.md). **Base** indica implementação existente a preservar; **planejado** exige execução da fase. Todas as linhas precisam de evidência no fechamento da F7. “Completo” significa recursos acessíveis conforme RBAC; permissões negadas não habilitam ações nem produzem conteúdo.

A [diretriz visual e de acesso do usuário](../reference/direcao-visual-e-premissa-de-acesso.md) complementa a referência: estilo da imagem KubePeep.png e cadastro de namespaces em lote sem acesso administrativo são obrigatórios. **U12 é a premissa básica/P0**, validada na F0 e preservada em todas as fases.

## Recursos e navegação

Escopo **N** = namespace; **C** = cluster. `list/get` são verbos distintos; uma lista autorizada não prova permissão de detalhe. Grupos virtuais (Overview, Permissions e sessões) mantêm seus contratos próprios.

| ID | Grupo / recurso | Escopo | Estado e fase | Aceite específico |
| --- | --- | --- | --- | --- |
| R01 | Cluster / Overview | N + contexto | base; F7 | resumo parcial, offline e Metrics API ausente sem bloquear shell |
| R02 | Cluster / Nodes | C | planejado; F1 | lista/detalhe/conditions/capacity; sem exigir scope |
| R03 | Cluster / Events | N | base; F5/F7 | filtro por objeto e navegação para recurso autorizado; sem segunda implementação em Observability |
| R04 | Cluster / Namespaces | C; scopes locais separados | parcial; F0/F2 | inspeção do objeto sujeita a RBAC; cadastro local em lote permanece disponível sem list/get Namespace (U12) |
| R05 | Cluster / Leases | N | planejado; F2 | holder/renewal; só namespaces autorizados |
| R06 | Workloads / Overview | N | base; F7 | filtros e links preservados |
| R07 | Deployments, DaemonSets, StatefulSets | N | base; F3/F5 | detalhe, Pods, eventos, YAML permitido e ações do catálogo atual |
| R08 | Pods | N | base; F5 | ready/status/restarts/node; containers/logs/events/YAML e ações permitidas |
| R09 | ReplicaSets | N | planejado; F3 | desired/ready/available; relações por owner UID |
| R10 | Jobs, CronJobs | N | base; F3/F7 | execução/condições corretas e relações sem saúde inventada |
| R11 | Network / Services | N | base; F4/F5 | endpoints/ports e port-forward autorizado |
| R12 | Network / Endpoints | N | planejado; F4 | contagens/ports e limitação de truncamento visível |
| R13 | Network / EndpointSlices, Ingresses | N | base; F4/F7 | relações com Service, preservando metadados seguros |
| R14 | Network / IngressClasses | C | planejado; F4 | controller/default e ausência de filtro namespace |
| R15 | Network / NetworkPolicies | N | planejado; F4 | selectors/tipos e detalhe de regras sanitizado |
| R16 | Network / Port Forwarding | sessão | parcial; F5 | listar/iniciar/parar uma/parar todas; dono, geração, loopback e cleanup |
| R17 | Configuration / ConfigMaps | N | base; F5/F7 | lista metadata-first; conteúdo apenas no detalhe autorizado |
| R18 | Configuration / Secrets | N | base; todas | somente metadados; sem YAML/diff/expansão de conteúdo |
| R19 | Configuration / ResourceQuotas | N | planejado; F3 | hard/used com unidades e ausência distinta de zero |
| R20 | Configuration / LimitRanges | N | planejado; F3 | tipo/limites/defaults legíveis |
| R21 | Configuration / HorizontalPodAutoscalers | N | planejado; F3 | target/min/max/current/desired/conditions; ligação ao workload |
| R22 | Configuration / PodDisruptionBudgets | N | planejado; F3 | healthy/desired/disruptionsAllowed e selectors |
| R23 | Storage / PersistentVolumes | C | planejado; F2 | phase/capacity/class/claim; sem credenciais de provider |
| R24 | Storage / PersistentVolumeClaims | N | planejado; F2 | volume/class/capacity/phase; respeita scope |
| R25 | Storage / VolumeAttachments | C | planejado; F2 | node/attacher/attached; sem attachmentMetadata livre |
| R26 | Storage / StorageClasses | C | planejado; F2 | provisioner/default/reclaim/bindingMode; parâmetros livres omitidos |
| R27 | Storage / CSINodes, CSIDrivers | C | planejado; F2 | drivers/attachRequired/capacidades; listas vazias legítimas |
| R28 | Access Control / ServiceAccounts | N | planejado; F3 | metadados/contagens; sem tokens, referências de Secret ou annotations livres |
| R29 | Access Control / Roles, RoleBindings | N | planejado; F4 | rules/subjects tipados no detalhe; não equivalem a decisões efetivas |
| R30 | Access Control / ClusterRoles, ClusterRoleBindings | C | planejado; F4 | list/get independentes e sem exigir scope |
| R31 | Access Control / Permissions | C/N | base; F4/F7 | SAR/capabilities da identidade atual; nunca inferir permissões a partir de Role |
| R32 | Administration / CustomResourceDefinitions | C | planejado; F4 | group/kind/scope/versions/conditions; não implica listar instâncias CR |
| R33 | Administration / PriorityClasses, RuntimeClasses | C | planejado; F4 | prioridade/preemption e handler/runtime; sem scope |
| R34 | Administration / MutatingWebhookConfigurations, ValidatingWebhookConfigurations | C | planejado; F4 | ambas famílias no destino Admission Webhooks; detalhes seguros |
| R35 | Observability / Logs | N | base/parcial; F5 | atuais/anteriores/follow, busca, wrap, timestamps, stop e exportação explícita |
| R36 | Application / Settings | local | base/parcial; F6 | shell/preferências persistem; sem apagar favoritos/filtros anteriores |

Helm Releases, Gateways, GatewayClasses, HTTPRoutes, GRPCRoutes, VolumeAttributesClasses, ValidatingAdmissionPolicies e ValidatingAdmissionPolicyBindings estão individualmente cobertos no [backlog](02-backlog-pos-v1.md).

## Requisitos transversais da referência

| ID | Seções | Entrega/critério verificável | Fase responsável |
| --- | --- | --- | --- |
| U01 | 1–8, 18–19, 27–30, 39–40 + imagem aprovada | aderência ao estilo KubePeep.png: superfícies escuras, sidebar/topbar compactas, cards/gráficos/tabelas coerentes; Inter/tokens/componentes existentes; contraste e dados honestos | base + todas; alinhamento F5, auditoria F7 |
| U02 | 9–11, 21–22, 35–36 | contexto/scope distintos, grupos compactáveis, tooltips, versão do build, nome KubePeep, preferências de shell | F1/F6 |
| U03 | 12–13, 32 | matriz R01–R36 com escopo e RBAC corretos; cliente oficial; sem kubectl como backend novo | F1–F4 |
| U04 | 14–16, 23–24 | listas/detalhes/filtros compartilhados; loading/vazio/403/unknown/offline/parcial/stale/truncado legíveis | F1–F6 |
| U05 | 15, 17 | tabs relevantes por kind, relações autorizadas, catálogo contextual e confirmação com alvo/origem | F3–F5 |
| U06 | 20, 37–38 | todas as páginas em 1280×720, 1366×768, 1440×900, 1920×1080; smoke adicional 2560×1440; sem scroll horizontal global | F7 |
| U07 | 25–26 | dashboard compacto; logs com altura útil e controles na fonte principal; conteúdo monoespaçado | F5/F7 |
| U08 | 31 + relato de timeout | requests canceláveis, cache por geração, paginação/concorrência limitadas, busca sem scan global, watchers sob demanda; ajustar timeout de 60 s relatado com múltiplos namespaces, deadlines coerentes e resultados/progresso preservados (V0-07) | F0/F1/F5/F7 |
| U09 | 33 | sentinelas sintéticas ausentes de logs internos/persistência/artefatos; Secret sempre metadata-only; exportação somente por gesto | todas; gate F7 |
| U10 | 34–35 | logo, purple da marca, nomenclatura e tipografia coerentes com assets oficiais existentes; janela Wails com nome correto | F7 |
| U11 | 37–38 | testes/build/CLI/Wails/instalação sem regressão e validação acessível por teclado/foco/zoom | F7 |
| U12 | premissa explícita do usuário | colar/revisar/deduplicar/salvar namespaces em lote sem list/get/create Namespace ou cluster-admin; cadastro distinto de acesso; RBAC por recurso, limites claros e resultados parciais preservados | F0 P0; todas preservam; gate F7 |

## Evidência e regra de conclusão

Ao concluir uma entrega, anexar à linha ou ao registro da fase: `ID | SHA | teste/comando | resultado | limitação`. Evidências de uma versão anterior continuam históricas. Não marcar teste de mock como teste de cluster real, cross-build como smoke nativo, nem preparação local como publicação.

Todas as linhas R/U são gate obrigatório da v1. O backlog é explícito e não conta como entregue. Falhas de ambiente e gates externos ficam pendentes com próximo passo identificado.
