# Fase 1 — Contrato de recursos e Nodes ponta a ponta

**Prioridade:** P0. **Entrada:** base preservada e checks locais conhecidos. **Desbloqueia:** F2–F5. **Matriz:** R02, U02–U04, U08–U09.

Entregar uma família completa antes de expandir o catálogo. O backend já possui coleções, paginação, geração, capabilities e helpers genéricos: ampliar o necessário sem uma refatoração global.

## Tarefas executáveis

- [ ] **V1-01 — Baseline e impacto.** Conferir rotas, serviços, `collectFilteredResource`, `SelectionGate`, seleção de contexto e consumidores do cursor pelo grafo e fonte. Registrar o comportamento atual de Pods/Workloads/Namespaces e os checks antes da alteração.
- [ ] **V1-02 — Contrato cluster-scoped.** Registrar ADR novo em `docs/decisions/` e atualizar `docs/api.md` antes da implementação: contexto válido sem scope; queries cluster-scoped rejeitam filtro `namespace`; identidade de cursor inclui profile/contexto/geração/coleção/query, sem scope artificial. Preservar `meta.page.filterScope=page|collection`; qualquer indicador de escopo Kubernetes usa campo distinto e compatível.
- [ ] **V1-03 — Coverage e erros.** Definir no contrato como representar lista cluster-scoped sem contagens de namespaces fictícios. `list` proibido retorna negação autoritativa; decisão unknown/servidor indisponível retorna indisponibilidade, sem “zero recursos”. Fixar payloads e testes antes de reutilizar em outras famílias.
- [ ] **V1-04 — Registro mínimo.** Definir coleção, GVR, escopo, capabilities `list/get`, sorts/filtros, política YAML e destinos. Reutilizar os catálogos existentes de `internal/services/resources` e `internal/services/authorization/allowlist.go`; não confundir ID de capability com plural Kubernetes. `watch` só entra se houver implementação e consumo limitado que o justifiquem.
- [ ] **V1-05 — Backend Nodes.** Adicionar portas/DTOs em `internal/services/resources`, integração typed client em `internal/integration/kubernetesruntime`, handlers e wiring de produção. Rotas propostas: `GET /api/v1/nodes`, `GET /api/v1/nodes/{name}` e YAML permitido por ação explícita; registrar GET/HEAD/Allow e parâmetros aceitos.
- [ ] **V1-06 — DTO e YAML.** Lista: nome, Ready/status, roles, versão, idade e IP interno autorizado. Detalhe: conditions, capacity/allocatable e taints com tamanhos limitados. Não serializar objeto Kubernetes cru, annotations arbitrárias ou material de autenticação; definir documento YAML seguro com política consistente ao backend existente e rotular omissões. Não enviar dados reais às evidências.
- [ ] **V1-07 — Frontend Nodes.** Estender `web/src/api/types.ts` e `client.ts`; compor página/lista/detalhe com componentes de `ui/` e `resource/`. Contexto selecionado sem scope deve permitir Nodes; contexto ausente ainda exibe seleção. Headers, filtros e loading/erro/vazio seguem o framework.
- [ ] **V1-08 — Rotas e identidade.** Habilitar `nodes` na árvore e rotas `/nodes` e `/nodes/:name`; corrigir catálogo de destinos/`resourceEntryPath` para aceitar identidade cluster-scoped sem namespace fictício. Manter deep link/reload/back, paleta e navegação por teclado. Integração da persistência de favoritos fica na F6.
- [ ] **V1-09 — Cancelamento e carga.** AbortSignal/context em toda leitura; troca de contexto ou geração cancela requests e limpa detalhe/YAML obsoleto. Lista grande mantém budget e cursor opaco; não criar watcher global para cada item da sidebar.
- [ ] **V1-10 — Guia de extensão.** Atualizar API, arquitetura e RBAC com contrato implementado e checklist de uma família: catálogo → DTO/porta → runtime → handler/wiring → cliente → página/rota/nav → casos de teste. Exemplo Nodes é fonte do guia; não copiar abstração fictícia do plano anterior.

## Aceite e testes

| Cenário | Resultado obrigatório |
| --- | --- |
| Contexto válido, nenhum scope ou scope restrito | Nodes lista/detalha; não pede `list namespaces` nem exibe filtro namespace |
| Sem contexto ativo | estado de seleção; nenhum request Kubernetes indevido |
| `list nodes` permitido e `get nodes` negado | lista disponível, detalhe negado sem conteúdo |
| Negação, unknown, timeout, lista vazia | quatro estados distintos; nenhuma inferência de autorização |
| Cursor adulterado, expirado ou de outra geração/query | erro compatível com contrato; nenhum dado de outra seleção |
| Duas páginas de fixture grande | limite cumprido, ordenação honesta (`page`), sem repetição/perda causada pelo novo código |
| Troca de contexto durante detalhe/YAML | resposta anterior descartada; nenhum flash de conteúdo obsoleto |
| Regressão Pods/Workloads | mesmos envelopes, filtros, RBAC, rotas e ações existentes |

Cobrir unitários de DTO/contrato, integração handler/runtime com API controlada e E2E lista → detalhe → YAML autorizado, inclusive 403 e contexto sem scope. Usar cliente metadata/typed conforme a política; testes negativos comprovam que campos proibidos não chegam à resposta. Executar o gate integrado do [plano](../README.md).

**Saída:** Nodes completa com guia e contrato revisados. **Rollback:** reverter o commit da fatia junto de registro/rotas/nav; manter as interfaces anteriores funcionando. Não remover migração ou dados do usuário para voltar ao comportamento anterior.
