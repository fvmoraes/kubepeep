# Fase 4 — Fechamento do refinamento UI/UX e scope default

**Prioridade:** P1 (contrato de experiência da v0.7). **Entrada:** F0 (inventário de cliques/UX); pode avançar em paralelo a F1–F3 coordenando arquivos compartilhados. **Matriz:** R03, X01–X17, T04.

O estilo atual é aprovado e **não será redesenhado** — REFINAR, PADRONIZAR, COMPACTAR, CORRIGIR, TORNAR FUNCIONAL. Esta fase audita item a item contra o inventário da F0 e fecha lacunas, incluindo a **regra obrigatória do scope default por contexto**.

## Tarefas

- [ ] **F4-01 — Tokens de design.** Consolidar em `web/src/tokens.css`: font-family, escala de font-size, pesos, line-height, spacing, radius, superfícies e cores semânticas. Eliminar os tamanhos arbitrários apontados pelo inventário; o texto normal das informações usa o mesmo padrão dos menus/filtros.
- [ ] **F4-02 — Proporção filtros × conteúdo.** Auditar todas as telas: filtros/controles ≈ 20–30% da área útil, conteúdo ≈ 70–80%. Compactar verticalmente sem perder usabilidade; medir e registrar.
- [ ] **F4-03 — Navegação item a item (fecha R02 na UI).** Validar cada destino lateral e horizontal: recurso correto, dados carregados, estado visual, filtros aplicáveis, contexto/namespace, rota/estado preservada. Corrigir os desvios do inventário da F0.
- [ ] **F4-04 (R03) — Scope default obrigatório por contexto (regra do projeto).** Permitir marcar **um** dos scopes cadastrados de cada contexto como **default**. Ao abrir o KubePeep e ao selecionar/alternar contexto, o scope default é carregado **obrigatoriamente** como universo ativo da sessão — seletor global de namespace, semântica de `All` (= namespaces permitidos pelo scope default + RBAC) e carregamento de recursos seguem esse universo. Sem scope default definido, a UI conduz à escolha/marcação em vez de assumir All namespaces. A marcação é preferência local por contexto; nunca exige `list/get namespaces`, cria objetos Namespace ou amplia RBAC. Trocas de nível superior (kubeconfig → context → scope → namespace) recarregam os inferiores sem estado órfão.
- [ ] **F4-05 — Resource Workspace completo.** Fechar as abas por kind (Pods: Overview/Logs/YAML/Events/Metrics/Containers/Ações; Deployment: Overview/Pods/ReplicaSets/YAML/Events/Rollout; Ingress: Rules/Backends; etc.) e revalidar histórico back/forward + recursos relacionados clicáveis em todas as famílias.
- [ ] **F4-06 — Catálogo de ações por tipo (referência §8–14).** Completar/revisar: Pods (logs, exec, port-forward, restart com explicação de recriação pelo owner, YAML, events, copy name/namespace), Deployments (scale ±/set replicas, rollout restart/status), StatefulSets, DaemonSets, Jobs/CronJobs (run now, suspend/resume), Services (port-forward, endpoints, copy ClusterIP), Ingresses (open URL, copy host). Nenhuma ação sem sentido por tipo.
- [ ] **F4-07 — Ações em massa.** Toolbar contextual ("N selecionados"), apenas ações compatíveis com todos os selecionados, confirmação clara; estender além de delete onde o catálogo suporta.
- [ ] **F4-08 — RBAC na UI.** Auditar por família: ocultar ou desabilitar ação sem permissão, com tooltip explicativo; o backend sempre revalida imediatamente antes de executar — nunca confiar na UI.
- [ ] **F4-09 — Confirmações destrutivas.** Alvo + namespace + "cannot be undone"; para operações especialmente perigosas, confirmação digitando o nome do recurso.
- [ ] **F4-10 — Feedback completo.** Loading, toast, status e progress em toda ação; sucesso e erro real do Kubernetes exibidos; nenhum botão sem resultado perceptível.
- [ ] **F4-11 — Densidade e colunas.** Largura total aproveitada, colunas default por kind (referência §21), menu Columns funcional; reduzir paddings/margins excessivos e blocos mortos.
- [ ] **F4-12 — Cores semânticas.** Azul primário, verde positivo, vermelho destrutivo, amarelo/laranja atenção — sem exagero, dark theme preservado.
- [ ] **F4-13 — Responsividade.** 1366×768, 1440×900, 1920×1080 e 2560×1440 sem scroll horizontal global; espaço extra vira informação.
- [ ] **F4-14 — Validação funcional por tela.** Fetch, filtros, search, sorting, paginação, refresh, live updates, namespace, scope, saved filters, columns, details, actions, RBAC, error/empty/loading states — por tela, com evidência; renderizar não basta.
- [ ] **F4-15 — Documentação.** Atualizar `docs/ui-ux-refinement.md` com decisões finais (novo modelo de navegação, contexto global, scope default, workspace, ações, massa, RBAC, testes e pendências).

## Cenários obrigatórios de aceite

| Cenário | Resultado exigido |
| --- | --- |
| Marcar um scope como default de um contexto, fechar e reabrir o KubePeep | scope default ativo automaticamente ao abrir; universo do seletor global = escopo marcado |
| Alternar entre dois contextos com defaults diferentes | cada contexto carrega seu scope default obrigatoriamente; nenhum estado/filtro do contexto anterior |
| Contexto sem scope default definido | UI conduz à marcação do default; não assume All namespaces nem amplia RBAC |
| Operador sem `list/get namespaces` | marcação e carregamento do scope default funcionam; All = permitido pelo scope + RBAC por recurso |
| Filtros em todas as telas | ≤ ~20–30% da área útil; conteúdo ≥ ~70–80% |
| Tipografia da interface inteira | tokens globais; zero tamanhos arbitrários; hierarquia só em títulos |
| Clicar em recurso relacionado dentro do Workspace | abre no workspace, entra no histórico; voltar restaura aba/filtros |
| Ação sem permissão | oculta/desabilitada com motivo; backend revalida |
| Delete/restart em massa | confirmação com alvo e namespace; feedback de resultado |

**Saída:** critérios de aceite da referência UI/UX (§30) verdes, scope default obrigatório funcionando e zero regressão. **Rollback:** nenhuma funcionalidade removida para simplificar; sem mocks escondendo quebra.
