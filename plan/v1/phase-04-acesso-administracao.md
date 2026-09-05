# Fase 4 — Access Control e Administration

> **Objetivo:** habilitar **Access Control** (Roles/Bindings cluster-scoped e namespaced, integrados à tela Permissions existente) e **Administration** (CRDs, PriorityClasses, RuntimeClasses, Admission Webhooks), além de IngressClasses, NetworkPolicies e Endpoints no grupo Network.
> **Prioridade:** P1. **Dependências:** Fase 1. **Complexidade agregada:** L.

## Tarefas

### Access Control

- [ ] **V4-01** `Roles` e `RoleBindings` (namespaced) e `ClusterRoles`/`ClusterRoleBindings` (cluster-scoped): DTO resumido (name, namespace, age; rules agregadas por contagem de verbs/resources — **nunca** regras cruas com resourceNames sensíveis no listing; regras completas apenas no detalhe/YAML).
- [ ] **V4-02** Capabilities `roles.list/get`, `rolebindings.list/get`, `clusterroles.list/get`, `clusterrolebindings.list/get`.
- [ ] **V4-03** Integração com **Permissions** (tela existente): na página da Role/ClusterRole, link "Ver decisões efetivas" filtrando a matriz de capabilities quando o escopo cobrir o namespace; acesso negado → estado desabilitado, nunca erro silencioso (§17).
- [ ] **V4-04** Cluster-scoped sem namespace scope: ClusterRoles/ClusterRoleBindings listam com qualquer escopo ativo.

### Administration

- [ ] **V4-05** `CustomResourceDefinitions`: cluster-scoped (name, group, kind, scope, version, established/accepted condition); capabilities `customresourcedefinitions.list/get`.
- [ ] **V4-06** `PriorityClasses` e `RuntimeClasses`: cluster-scoped (name, value/preemptionPolicy; handler/runtimeClass name), capabilities correspondentes.
- [ ] **V4-07** `MutatingWebhookConfigurations`/`ValidatingWebhookConfigurations`: cluster-scoped — DTO **sem URLs, CA bundles e service refs completos**: name, webhooks count, failurePolicy agregada, age. **YAML bloqueado** (contém CA bundle e endpoints internos); detalhe metadata-only com aviso.
- [ ] **V4-08** `ValidatingAdmissionPolicies`(+`Bindings`) atrás de detecção de versão/servidor (FEATURE_GATE por discovery); indisponível → nav desabilitada por cluster, não erro (padrão `FEATURE_UNAVAILABLE` do dashboard).

### Network (complemento do grupo)

- [ ] **V4-09** `IngressClasses` (cluster-scoped: name, controller, default) e `NetworkPolicies` (namespaced: name, namespace, podSelector resumido, types, age — rules completas só no YAML), capabilities próprias.
- [ ] **V4-10** `Endpoints` (namespaced: name, namespace, addresses count, ports) — EndpointsSlices já existe; Endpoints mantido por compatibilidade de navegação (§36 lista Endpoints).

### Frontend (comum)

- [ ] **V4-11** Páginas via framework; nav habilita todos os itens de Access Control (Roles… Permissions já ativo), Administration (CRDs, Priority Classes, Runtime Classes, Admission Webhooks) e Network (Endpoints, IngressClasses, NetworkPolicies).
- [ ] **V4-12** Cluster-scoped nas páginas certas: sem filtro namespace em CRDs/Priority/Runtime/Webhooks/IngressClasses/ClusterRoles.

## Critérios de aceite

- Webhook configurations não expõem URL, CA bundle ou service/host/port em nenhuma resposta (teste de allowlist + grep no envelope).
- Access Control completo conforme §36 com integração Permissions funcionando (link aparece só quando o escopo cobre).
- RBAC: listar ClusterRoles sem permissão → coverage denied; UI mostra estado honesto.
- E2E: CRDs lista→detalhe; Roles→link Permissions; 1280×720/1920×1080.

## Testes e rollback

- Padrão da Fase 1 + testes de allowlist de DTO por recurso sensível.
- Rollback por família; features por discovery (V4-08) degradam para desabilitado sem deploy.
