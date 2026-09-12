# UI/UX — refinamento e baseline auditável

Este documento é o inventário funcional e visual da Fase 0 do plano v0.7 e a
entrada da F4/F7. O dark theme, roxo KubePeep, sidebar e navegação horizontal
são preservados; a meta é correção e consistência, não redesign.

**Baseline:** 2026-09-12, branch `review/plan-v0.7`. Evidência automatizada e
SHA de fechamento ficam em `plan/v0.7/03-evidencias-execucao.md`.

## 1. Tokens, tipografia e desvios conhecidos

`web/src/tokens.css` é a fonte de verdade:

- tipografia: 10/11/12/13/14/16/20/26/32 px (`text-2xs`…`text-4xl`);
- controles: 28/32/36 px (`h-7`/`h-8`/`h-9`);
- spacing, radius, cores, sombras e z-index também são tokens;
- monospace fica reservado a logs, YAML/JSON e valores técnicos.

Busca estática por `font-size`, `fontSize` e classes arbitrárias encontrou
somente estas exceções intencionais:

| Local | Valor | Classificação / ação F4 |
| --- | ---: | --- |
| `styles.css` `.mono` e `code` | `0.92em` | relativo ao contexto técnico; avaliar token mono dedicado |
| `styles.css` `kbd` | `10px` | coincide com `text-2xs`, mas deve migrar para o token |
| `ExecTerminal.tsx` xterm | `12px` | API JS não usa classe Tailwind; coincide com `text-sm` |

Não há classe `text-[…]` na aplicação. Cores semânticas de ações continuam:
azul normal, verde positivo, vermelho destrutivo e âmbar disruptivo.

## 2. Proporção filtros × conteúdo

### Método

A medição geométrica usa o header de 56 px e a altura CSS real dos controles:
28 px de controle + 16 px de padding + borda = aproximadamente 46 px; a linha
de chips acrescenta aproximadamente 22 px. Em 1366 px de largura, toolbars
com muitos campos podem quebrar para duas linhas (~96–108 px). A proporção é
`altura dos filtros ÷ (altura da viewport − 56 px)`; título/descrição não são
contados como filtro nem conteúdo da tabela.

| Tela(s) | Bloco de filtro | 768 px | 900 px | 1080 px | Conteúdo restante |
| --- | ---: | ---: | ---: | ---: | ---: |
| Pods, Workloads, Events | 68–108 px | 9,6–15,2% | 8,1–12,8% | 6,6–10,5% | 84,8–93,4% |
| Network, Configuration, Storage | 68–108 px | 9,6–15,2% | 8,1–12,8% | 6,6–10,5% | 84,8–93,4% |
| Nodes, Leases, Service Accounts | 46–68 px | 6,5–9,6% | 5,5–8,1% | 4,5–6,6% | 90,4–95,5% |
| Access e Administration | 46–68 px | 6,5–9,6% | 5,5–8,1% | 4,5–6,6% | 90,4–95,5% |
| Dashboard (consulta de logs) | 72–96 px | 10,1–13,5% | 8,5–11,4% | 7,0–9,4% | 86,5–93,0% |
| Logs | 96–132 px | 13,5–18,5% | 11,4–15,6% | 9,4–12,9% | 81,5–90,6% |
| Settings, Contexts/Scopes, About | n/a | n/a | n/a | n/a | formulário/conteúdo próprio |

A afirmação anterior de “20–30%” não era uma medição. O baseline real fica
abaixo de 20% nos viewports de referência 1366×768, 1440×900, 1920×1080 e
2560×1440; isso é aceitável porque amplia o conteúdo. O risco residual é
quebra de linha em largura menor que 1100 px, a ser validado visualmente na F4.

## 3. Semântica dos filtros

| Controle | Onde executa | Efeito real |
| --- | --- | --- |
| Search/status/sort/order das listas | request ao backend | altera query key e parâmetros; resultado continua `filterScope=page` (C01) |
| Namespace global | resolução de seleção + backend | altera origens autorizadas; nunca amplia RBAC |
| Tabs Network/Configuration/Storage/Access/Admin | router | atualiza URL canônica e a coleção consultada |
| Colunas | apresentação local + Preferences PUT | não refaz LIST; primeira coluna permanece visível |
| Saved filters | Preferences + toolbar | persiste somente campos allowlisted; Apply muda a query ativa |
| Logs: target/previous/since/tail | backend | abre leitura/stream autorizado para o alvo escolhido |
| Logs: busca/nível/wrap/pausa | browser | filtra/apresenta apenas o buffer já recebido |
| Command center | browser | busca páginas, favoritos, recentes e recursos já visíveis; não varre cluster |
| Dashboard log scan | backend | inicia scan limitado; filtros da lista resultante são locais |

Após a Fase 0 não há filtro conhecido cujo **Apply** não altere query/estado.
Access/Admin agora promovem `draft → applied`; chips Search duplicados foram
removidos em Configuration, Service Accounts, Access e Administration.

## 4. Inventário funcional de cliques

Resultado da auditoria: cada controle abaixo executa sua função ou fica
desabilitado com `title`/tooltip. `Button` fornece fallback central e os casos
condicionais críticos usam `disabledReason` específico.

| Tela | Elementos auditados | Resultado F0 |
| --- | --- | --- |
| Shell/sidebar | grupos, páginas, Settings, compact mode, Context/Scope/Namespace | navegação funcional; itens sem implementação permanecem indisponíveis e identificados |
| Command center | abrir, pesquisar, opções, recentes, limpar, ajuda, fechar | funcional por mouse/teclado; opções ocultas/desabilitadas não recebem atalho |
| Dashboard | cards/links, Refresh, scan de logs e parâmetros | funcional; Refresh/scan desabilitados durante requisito pendente |
| Pods | busca/filtros/sort, tabela, detalhes, Logs, paginação, colunas, ações | funcional; tela vazia distingue vazio, parcial, proibido e indisponível |
| Workloads | tabs/kinds, filtros, detalhes, relacionados, rollout e ações | funcional |
| Events | filtros, sort, refresh, paginação e detalhe | funcional |
| Network | tabs Services/Ingress/EndpointSlice/Endpoints/Policies, filtros e detalhe | tabs atualizam rota canônica; zero tab “visual-only” |
| Configuration | tabs ConfigMaps/Secrets/Quotas/Limits, filtros e detalhe | tabs atualizam rota; Secret continua metadata-only |
| Storage | seis tabs, filtros aplicáveis, colunas e detalhe | catálogo completo permanece no chooser; tabs sem status não exibem Select vazio |
| Nodes e Leases | filtros, links, colunas, paginação | funcional |
| Service Accounts | busca, detalhe, colunas, paginação | funcional; chip Search único |
| Access | Roles/Bindings/Cluster*, filtros, detalhes, colunas | Apply/Clear funcionais; query cercada por geração |
| Administration | CRDs/classes/webhooks/IngressClasses, filtros e detalhe | Apply/Clear funcionais; query cercada por geração |
| Workspace | Back/Forward, tabs, favorito, relacionados, YAML e Close | histórico funcional; Back/Forward explicam ausência de entrada anterior/próxima |
| Logs | Read/Follow/Stop/Pause, Copy/Download/Clear, filtros locais | funcional e limitado; estados inválidos ficam explicados/visíveis |
| Settings | edição, remoção, Reset e Save | Reset/Save desabilitados sem dirty state; Reset restaura o snapshot salvo |
| Ações em massa | seleção, select-all, logs, delete e clear | delete exige confirmação e reporta sucesso/falhas por toast |

Defeitos fechados pela varredura F0:

1. Apply de Access/Admin não aplicava o draft;
2. tabs Network/Configuration alteravam estado, mas a URL prevalecia;
3. First page podia parecer clicável sem executar transição;
4. coluna Storage oculta desaparecia do próprio chooser;
5. tabs Storage sem campo de status exibiam Select vazio;
6. Reset/Save de Settings não representavam dirty state;
7. falha de persistência de coluna era silenciosa;
8. chips Search apareciam duplicados em quatro superfícies.

## 5. Estado desabilitado e feedback

- `Button.disabledReason` vira `title`; se o consumidor não especificar texto,
  o fallback informa que os requisitos atuais não foram satisfeitos.
- First page: “Already on the first page.”; Next: “The current result has no
  next page.”
- links de relacionados sem rota suportada: “Detail navigation is unavailable
  for this reference.”
- Back/Forward do workspace explicam ausência de histórico;
- decisões RBAC denied/unknown aparecem ao lado das ações e não são convertidas
  em lista vazia;
- falha ao salvar colunas mantém a mudança otimista, mas mostra alerta e sugere
  retry/reload;
- toasts possuem `aria-live`; confirmações destrutivas usam `alertdialog`.

## 6. Ações, confirmação e segurança

Todas as mutações enviam o envelope backend `confirmed: true`, CSRF,
`expectedGeneration`, consequência allowlisted e revalidação SSAR fail-closed.
Esse campo de protocolo **não significa** que toda ação abre um diálogo visual.
Inventário real:

| Ação | Confirmação visual | Observação |
| --- | --- | --- |
| Delete workload / Pod | `ConfirmDialog` | UID/RV quando aplicável; consequência explícita |
| Restart Pod | `ConfirmDialog` | delete controlado; controller pode recriar |
| Bulk delete | `ConfirmDialog` | lista alvos e resultado parcial |
| Restart Deployment/StatefulSet/DaemonSet | direta | botão âmbar + toast; não abre dialog |
| Scale | direta | input validado, HPA warning e Apply explícito |
| Suspend/Resume CronJob | direta | botão semântico + toast |
| Run now CronJob | direta | cria Job e mostra toast |
| Port-forward | direta | valida portas, bind somente loopback |
| Exec | direta | valida container/comando e usa ticket efêmero |

Portanto, a frase antiga “todas com confirmação” foi removida. A F4 pode
decidir se Restart/Scale/Suspend/Run now exigem novo UX de confirmação; a Fase
0 apenas registra o comportamento verdadeiro e garante que o clique funciona.

## 7. Resource Workspace e relacionados

O workspace ocupa aproximadamente 82% da viewport, tem histórico global,
fecha por Esc/backdrop e preserva aba por entrada. Deep links abrem o mesmo
workspace. Relacionados clicáveis hoje:

- Pods/ReplicaSets de Deployment;
- Pods de ReplicaSet/Job e Jobs de CronJob;
- owner de Pod quando há rota suportada;
- PVCs de StatefulSet.

A aba **Endpoints** de Service mostra facts agregados (endereços prontos/não
prontos, portas e truncamento); ela **não contém links de Endpoint**. A afirmação
anterior de “Endpoints de Service clicáveis” era falsa e foi corrigida aqui.
Referências desconhecidas são renderizadas desabilitadas com motivo, sem rota
inventada.

## 8. Navegação e contexto global

- paths canônicos de lista/detalhe ficam em `web/src/navigation/paths.ts`;
- Access lê `:tab`, não `:namespace`;
- rotas de detalhe de Access, Storage, Network e Service Accounts existem;
- query keys de Access/Admin incluem `generation`;
- a barra superior explicita Kubeconfig → Context → Scope → Namespace;
- `All` significa somente namespaces do scope/RBAC atual;
- troca incompatível de contexto/scope/geração remove dados anteriores.

## 9. Segurança e limites preservados

- nenhum clique bypassa backend, CSRF, fence, SSAR ou preconditions;
- Secret segue sem valores e sem YAML;
- `filterScope=page` permanece honesto;
- paginação recebe cursor explícito; não infere primeira página de `next`;
- nenhuma preferência, métrica UX ou estado de navegação usa local/session
  storage para dados de cluster;
- `scripts/security_check.sh HEAD` permanece gate obrigatório.

## 10. Validação e próximos passos

A regressão automatizada cobre Pods, rotas/tabs, Apply/Clear, paginação,
Settings, persistência e reativação de coluna Storage. Contagens e comandos do
fechamento ficam no registro de evidências para não congelar números obsoletos
neste documento.

Pendências deliberadas para fases seguintes, não cliques mortos da F0:

- edição/aplicação arbitrária de YAML requer contrato e política próprios;
- CPU/memória de Pods dependem de Metrics API real;
- Helm releases e Gateway API continuam fora da implementação atual;
- F4 executará smoke visual comparativo e decidirá tokens relativos/UX de
  confirmação adicional;
- F3 fará virtualização; na F0 a página pública continua limitada a 100 linhas.
