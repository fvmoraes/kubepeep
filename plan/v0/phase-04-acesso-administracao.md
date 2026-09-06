# Fase 4 — Network, Access Control e Administration

**Prioridade:** P1, obrigatória para v1. **Entrada:** F1; ServiceAccounts da F3 para fechar Access Control. **Matriz:** R11–R15, R29–R34.

Concluir os grupos obrigatórios da referência com leitura autorizada e DTOs específicos. Listar regras RBAC não calcula permissões efetivas; listar CRDs não habilita leitura de instâncias CR.

## Tarefas

- [ ] **V4-01 — Roles e bindings.** Implementar `Roles`/`RoleBindings` namespaced e `ClusterRoles`/`ClusterRoleBindings` cluster-scoped (`rbac.authorization.k8s.io`), com lista/detalhe/capabilities e rotas. Listas mostram contagens; detalhes mostram regras, roleRef e subjects aprovados em estruturas tipadas/limitadas. Nunca logar subjects privados ou regras reais em evidência.
- [ ] **V4-02 — Permissions.** Integrar links para a tela existente preservando contexto/scope aplicável. Exibir decisões SAR da identidade ativa, incluindo allowed/denied/unknown. **Não** afirmar que uma Role isolada concede todas as capabilities nem que a matriz representa outro subject. Restringir/descrever o link quando não houver escopo adequado.
- [ ] **V4-03 — CRDs.** Implementar `apiextensions.k8s.io/customresourcedefinitions`: group, kind, scope, versões served/storage e conditions. Detalhe tipado; schemas/defaults/examples arbitrários não entram na listagem. Nenhum discovery amplo deve ler instâncias CR ou contornar RBAC.
- [ ] **V4-04 — PriorityClasses e RuntimeClasses.** Adicionar recursos cluster-scoped de `scheduling.k8s.io` e `node.k8s.io`; priority/globalDefault/preemptionPolicy e handler/overhead/scheduling aprovados, com estados de campos ausentes.
- [ ] **V4-05 — Admission Webhooks.** Implementar ambas as famílias `MutatingWebhookConfigurations` e `ValidatingWebhookConfigurations` (`admissionregistration.k8s.io`) no mesmo destino com tabs distinguíveis. Lista: nome/quantidade/idade; detalhe: policies/regras aprovadas. V1 não expõe YAML bruto, CA bundles, URLs ou service refs completos; mostrar indisponibilidade de YAML de forma explícita. Não confundir estes recursos com admission policies do backlog.
- [ ] **V4-06 — IngressClasses e NetworkPolicies.** `IngressClasses` cluster-scoped: controller/default/parâmetros referenciados aprovados. `NetworkPolicies` namespaced: selector, policy types e regras tipadas no detalhe. Relações com Ingress/Pods são condicionadas a acesso; omitir metadados livres.
- [ ] **V4-07 — Endpoints.** Implementar coleção namespaced legada com subsets, contagens e ports limitados; sinalizar truncamento quando fornecido pela API. Preservar EndpointSlices e relacionar Service → Endpoints/EndpointSlices sem assumir equivalência integral entre os formatos.
- [ ] **V4-08 — Disponibilidade por API.** Diferenciar API ausente (feature indisponível), RBAC negado e falha de discovery. Cache efêmero por contexto/geração; nunca mapear 403 para “API não existe”. Recursos condicionais do backlog não são implementados por esta tarefa.
- [ ] **V4-09 — Integração de todas as superfícies.** DTOs/portas/runtime/handlers/wiring/cliente, allowed methods, capabilities, destinos, sidebar e páginas pelo framework. Documento YAML de cada família precisa cumprir a política da F1; ocultar ação não suportada, sem fallback para objeto cru.
- [ ] **V4-10 — Docs e fixtures.** Atualizar API/RBAC/arquitetura. Fixtures sintéticas com RoleBindings homônimos, CRD com duas versões, API indisponível, webhook com campos sentinela e Endpoints truncado.

## Aceite

- Todos os recursos R11–R15/R29–R34 navegam; ServiceAccounts integra Access Control sem duplicar página. Admission Webhooks cobre as duas famílias.
- ClusterRoles/CRDs/Classes/Webhooks abrem sem scope; Roles/Bindings/Policies/Endpoints respeitam namespaces autorizados.
- Testes separam `list` de `get`, cluster de namespace, API ausente de negação e unknown. A tela Permissions continua baseada na autorização efetiva.
- Campos sentinela de webhook/CRD não aparecem em DTOs de lista, logs ou persistência. YAML indisponível é recusado também por request direto.
- E2E cobre Role → Permissions, CRD lista → detalhe e Service → Endpoints/Slices; cada contrato restante tem teste de integração e cobertura dos estados relevantes. Gate integrado conforme [plano](../README.md).

**Saída:** famílias obrigatórias de Network/Access Control/Administration completas. **Rollback:** reverter por família com nav/capabilities correspondentes; cache de discovery é efêmero e não migra dados locais.
