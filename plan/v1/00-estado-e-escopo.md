# Estado atual e limites da v1

Base de código: redesign `5ac7320`, seguido pela organização documental `fc9b052`. Inventário revisado por Codebase MCP e leitura da fonte em setembro de 2026. Um item implementado continua sujeito ao gate de regressão; números de testes de um commit anterior não são prova da release futura.

## Base a reutilizar

| Área | Evidência no código | O que falta para a v1 |
| --- | --- | --- |
| Design System | `web/src/tokens.css`, `components/ui/`, `components/resource/` | aplicar os mesmos tokens/componentes às novas famílias; revisar §38 da referência |
| Shell e árvore | `web/src/navigation/tree.tsx`, `components/Sidebar.tsx`, `hooks/useAppVersion.ts` | ligar os destinos novos e persistir compactação/grupos; entradas sem `path` hoje são futuras |
| Recursos existentes | `web/src/components/ResourcePages.tsx`, `internal/api/handlers/routes.go` | ampliar catálogo e detalhes conforme matriz; manter rotas e filtros atuais |
| Coleções e autorização | `internal/services/resources/`, `internal/integration/kubernetesruntime/resources_backend.go`, `internal/services/authorization/allowlist.go` | contrato para recursos cluster-scoped e capabilities novas; reutilizar `collectFilteredResource` e matriz existente |
| Busca e ordenação | `internal/services/resources/search.go`, `internal/integration/kubernetesruntime/resources_sort.go` | já têm termos/frases/negação e ordenação natural; integrar famílias novas, documentar e testar paginação |
| Preferências/favoritos | `internal/services/resources/preferences.go`, `internal/adapters/sqlite/preferences.go` | compactação/grupos/colunas/recentes e referências com origem e escopo; preservar schema/dados existentes |
| YAML/diff | `web/src/components/YamlViewer.tsx`, API `yaml-diff` | busca/recolhimento/cancelamento; diff atual é vivo × last-applied, não comparação arbitrária de duas origens |
| Port-forward/logs/exec | painel em `ResourcePages.tsx`, serviços de ações e streams existentes | organizar sessões e ações contextuais; preservar cancelamento, RBAC e limites |
| Namespaces | listagem opcional e editor de scopes com colagem, validação e deduplicação de listas | F0 comprova e destaca cadastro em lote sem descoberta; F2 distingue inspeção do objeto Namespace de edição de scopes; não recriar a gestão existente |

O helper `useResourceList` e o pacote `internal/services/preferences` citados no plano anterior não existem nesta base. As páginas usam React Query e helpers de geração; preferências pertencem a `internal/services/resources`. Usar os caminhos reais, sem criar duplicatas para satisfazer nomes de um plano antigo.

## Contrato do produto 1.0

- **Premissa básica/P0:** cadastrar namespaces em lote no aplicativo e operar em namespaces conhecidos sem `list/get namespaces` nem `cluster-admin`. Cadastro local não cria objetos Namespace. A [F0](phase-00-acesso-restrito-e-lote.md) é gate de entrada, e U12 permanece bloqueante até a release.
- A imagem KubePeep.png enviada pelo usuário define a [direção visual aprovada](../reference/direcao-visual-e-premissa-de-acesso.md). Seus dados e a seleção All namespaces são ilustrativos; o layout se adapta às permissões e à cobertura reais.
- Uma seleção ativa de profile/contexto. Recursos namespaced respeitam o scope; recursos cluster-scoped exigem contexto válido, mas funcionam sem scope de namespaces.
- Todas as famílias obrigatórias dos §§12/36 da referência têm lista, detalhe útil e navegação. YAML/eventos/ações só aparecem quando o contrato do recurso e a autorização os permitem.
- ServiceAccounts pertence a **Access Control**, mesmo sendo implementado junto das coleções namespaced na F3. Leases e PVCs são **namespaced**; a posição na sidebar não altera isso.
- Namespaces mantém a gestão de scopes existente e ganha inspeção do recurso sem confundir os dois conceitos.
- Ações mutáveis continuam limitadas ao catálogo existente: restart de Deployment, scale de Deployment/StatefulSet, delete de Pod, port-forward e exec. Os exemplos de delete de Service/Deployment na referência não ampliam esse catálogo automaticamente.
- Preferência “local” significa armazenamento allowlisted no backend local. `localStorage`, `sessionStorage`, IndexedDB e caches persistentes de conteúdo Kubernetes continuam proibidos.

## Decisões que evitam regressões

1. `meta.page.filterScope` descreve o alcance de **filtro/ordenação** (`page` ou `collection`), não o escopo Kubernetes. Não introduzir `filterScope: "cluster"`. F1 define informação separada quando necessária.
2. Não requisitar `list namespaces` para abrir Nodes/PVs/ClusterRoles. Permissões de coleção cluster-scoped são próprias e não se deduzem de um scope namespaced.
3. Não converter indisponibilidade em “lista vazia”: negar autoritativamente → proibido; autorização inconclusiva → unknown/indisponível; resposta parcial → coverage com causa sanitizada.
4. Secrets continuam metadata-only sem YAML/diff. Outras famílias recebem DTOs explícitos; YAML precisa de política por família, não serialização genérica do objeto cru.
5. VolumeAttachments é entrega obrigatória da F2 com campos seguros; não se remove a família da v1 por conter campos que devem ser omitidos.
6. Fases 0–6 são obrigatórias. Adiamentos estão enumerados no backlog; mudar o gate exige atualizar matriz, fase e produto, deixando a decisão visível.

## Evidência inicial a registrar na primeira execução

- SHA de partida, worktree e ferramentas disponíveis; teste/build de referência.
- Rotas/capabilities/componentes que cada tarefa vai alterar e seus consumidores.
- Features já implementadas que só precisam de integração, evitando reconstruir busca, sort, diff last-applied, favoritos, logs e sessões.
- Bloqueios de ambiente nativo/cluster registrados separadamente dos defeitos da aplicação.
