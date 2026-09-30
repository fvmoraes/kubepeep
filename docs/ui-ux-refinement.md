# UI/UX — refinamento final da v0.7

Este documento consolida o inventário da Fase 0 e as decisões finais da Fase 4
do plano v0.7. O dark theme, roxo KubePeep, sidebar e navegação horizontal são
preservados, com correções de consistência, densidade e comportamento.

**Fechamento:** 2026-09-29, branch `review/plan-v0.7`. Evidência automatizada e
SHA de fechamento ficam em `plan/v0.7/03-evidencias-execucao.md`.

## 1. Tokens, tipografia e desvios conhecidos

`web/src/tokens.css` é a fonte de verdade:

- tipografia: família, escala 10/11/12/13/14/16/20/26/32 px
  (`text-2xs`…`text-4xl`), pesos e line-height;
- controles: 28/32/36 px (`h-7`/`h-8`/`h-9`);
- spacing, radius, cores, sombras e z-index também são tokens;
- monospace fica reservado a logs, YAML/JSON, terminal e valores técnicos;
- o terminal xterm lê família, tamanho e paleta diretamente dos custom
  properties, mantendo o canvas alinhado ao restante da aplicação.

A busca estática por `font-size`, `fontSize` e classes `text-[…]` encontrou zero
tamanhos arbitrários fora de `tokens.css`. Cores semânticas de ações continuam:
azul normal, verde positivo, vermelho destrutivo e âmbar disruptivo. Literais de
cor ficaram restritos à definição dos tokens e aos SVGs de marca.

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

A validação Playwright final mediu a tela de Pods com 30 itens e a mesma matriz
usada no aceite. Não houve overflow horizontal global:

| Viewport | Controles / área útil | Largura do conteúdo / viewport |
| --- | ---: | ---: |
| 1366×768 | 5,56% | 96,27% |
| 1440×900 | 5,56% | 96,50% |
| 1920×1080 | 3,72% | 97,50% |
| 2560×1440 | 3,72% | 98,19% |

Os controles ficam abaixo do teto de 30% e o conteúdo supera o mínimo de 70%.
O espaço extra amplia tabela e colunas em vez de criar blocos vazios.

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
| Ações em massa | seleção, select-all, logs, restart, delete e clear | catálogo compatível com toda a seleção; confirmação e resultado parcial por toast |

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
As ações disruptivas ou destrutivas do catálogo final abrem confirmação visual:

| Ação | Confirmação visual | Observação |
| --- | --- | --- |
| Delete workload | `ConfirmDialog` + nome digitado | alvo/namespace e consequência irreversível; UID/RV quando aplicável |
| Delete Pod | `ConfirmDialog` | alvo/namespace, UID/RV quando aplicável; consequência explícita |
| Restart Pod | `ConfirmDialog` | delete controlado; controller pode recriar |
| Bulk delete/restart | `ConfirmDialog` | lista alvos/namespaces e resultado parcial |
| Restart Deployment/StatefulSet/DaemonSet | `ConfirmDialog` | alvo/namespace + toast |
| Scale | direta | input validado, HPA warning e Apply explícito |
| Suspend/Resume CronJob | direta | botão semântico + toast |
| Run now CronJob | direta | cria Job e mostra toast |
| Port-forward de Pod/Service | direta | valida portas, bind somente loopback e mostra sessão criada |
| Exec | direta | valida container/comando e usa ticket efêmero |

Scale, suspend/resume, run now, port-forward e exec preservam ação direta porque
possuem campos explícitos, validação e feedback, e não removem o recurso alvo.
O backend revalida geração, preconditions e RBAC imediatamente antes da ação.

## 7. Resource Workspace e relacionados

O workspace ocupa aproximadamente 82% da viewport, tem histórico global,
fecha por Esc/backdrop e preserva aba por entrada. Deep links abrem o mesmo
workspace. Relacionados clicáveis hoje:

- Pods/ReplicaSets de Deployment;
- Pods de ReplicaSet/Job e Jobs de CronJob;
- owner de Pod quando há rota suportada;
- PVCs de StatefulSet;
- Services referenciados por regras e backends de Ingress.

A aba **Endpoints** de Service mostra facts agregados (endereços prontos/não
prontos, portas e truncamento). Services também oferecem port-forward para uma
porta TCP: o backend resolve um Pod Running/Ready compatível e o `targetPort`
inteiro ou nomeado, e então revalida `create pods/portforward` para o Pod exato.
Ingress expõe Rules/Backends, abre URL HTTP(S) e copia host. Referências
desconhecidas são renderizadas desabilitadas com motivo, sem rota inventada.

O catálogo de tabs final inclui Pods (Overview, Logs, YAML, Events, Metrics,
Containers e Actions), workloads (relações e rollout quando aplicável), Service
(Overview, Endpoints, YAML e Actions) e Ingress (Overview, Rules, Backends, YAML
e Actions). O histórico Back/Forward preserva recurso e aba.

## 8. Navegação e contexto global

- paths canônicos de lista/detalhe ficam em `web/src/navigation/paths.ts`;
- Access lê `:tab`, não `:namespace`;
- rotas de detalhe de Access, Storage, Network e Service Accounts existem;
- query keys de Access/Admin incluem `generation`;
- a barra superior explicita Kubeconfig → Context → Scope → Namespace;
- `All` significa somente namespaces do scope/RBAC atual;
- troca incompatível de contexto/scope/geração remove dados anteriores.

Cada `(cluster profile, contexto)` possui no máximo um scope local marcado como
default. Ao abrir a aplicação ou trocar de contexto, `DefaultScopeGate` ativa
esse scope com fence de geração antes de liberar as páginas de recursos. Um
contexto sem default direciona o usuário à tela de scopes e nunca assume todos
os namespaces. A preferência não consulta nem cria Namespace; um scope manual
continua funcional para operadores sem `list/get namespaces`.

Excluir o default exige escolher outro default do mesmo contexto ou confirmar o
retorno ao setup sem seleção ativa. A troca entre contextos descarta o default,
namespace, filtros e dados do contexto anterior antes de carregar o próximo.

## 9. Segurança e limites preservados

- nenhum clique bypassa backend, CSRF, fence, SSAR ou preconditions;
- Secret segue sem valores e sem YAML;
- `filterScope=page` permanece honesto;
- paginação recebe cursor explícito; não infere primeira página de `next`;
- nenhuma preferência, métrica UX ou estado de navegação usa local/session
  storage para dados de cluster;
- `scripts/security_check.sh HEAD` permanece gate obrigatório.

## 10. Validação e limites deliberados

A regressão automatizada cobre navegação, Pods, rotas/tabs, Apply/Clear,
paginação, saved filters, colunas, Settings, scope default, RBAC, confirmações,
ações individuais/em massa e port-forward de Service. Playwright valida a
matriz de quatro viewports e as tabs completas do workspace. O harness Kind
exercita o binário contra cluster real; contagens e comandos ficam no registro
de evidências para não congelar números obsoletos neste documento.

Limites deliberados, sem controles que prometam essas operações:

- edição/aplicação arbitrária de YAML requer contrato e política próprios;
- CPU/memória de Pods dependem de Metrics API real;
- Helm releases e Gateway API continuam fora da implementação atual;
- a API pública continua limitada e paginada; a lista virtualizada mantém o DOM
  bounded sem fingir que resultados não carregados estão presentes.

## 11. Auditoria final da Fase 7 — checklist §30

A auditoria final repetiu o inventário inteiro, em vez de validar somente Pods.
`phase07.spec.ts` percorreu os 44 destinos habilitados da navegação em
1366×768, 1440×900, 1920×1080 e 2560×1440. Cada clique atualizou URL,
`aria-current`, heading da tela e manteve a largura do documento dentro da
viewport. O E2E preexistente foi alinhado ao contrato atual e também repetiu
navegação, reload e History API com seleção resolvida.

| Critério final | Evidência | Resultado |
| --- | --- | --- |
| hierarquia e navegação | 44 destinos × 4 viewports; reload e back | aprovado, sem clique morto nem overflow global |
| densidade e espaço útil | medidas F4 + varredura F7 em todas as telas | filtros abaixo do teto e conteúdo predominante |
| loading/empty/error/partial/forbidden | Vitest das páginas e E2E offline/Kind | estados distintos e com ação de recuperação |
| scope default | browser troca dois contextos; Kind reabre, descobre e ativa o default antes da LIST | aprovado sem assumir All ou ampliar RBAC |
| workspace | tabs por kind, histórico, overlays e fechamento | aprovado; overlays limitam altura e contêm overscroll |
| catálogo por workload | catálogo fechado para Deployment, StatefulSet, DaemonSet, Job, CronJob e ReplicaSet | aprovado; somente ações semanticamente válidas aparecem |
| RBAC visual | 6 kinds × allowed/denied/unknown/error | 24 combinações aprovadas; backend continua revalidando |
| confirmação destrutiva | diálogo, foco visível, alvo, consequência e nome quando crítico | aprovado |
| controles indisponíveis | tooltip/título e razão visível, inclusive Events sem detalhe | aprovado |
| responsividade/acessibilidade | landmarks, headings, foco, teclado, tabela e quatro resoluções | aprovado |
| Pods web | Playwright e suíte de recursos | dados reais/sintéticos visíveis; vazio não mascara erro |
| Pods desktop | Wails nativo + AT-SPI em perfil isolado e Kind real | tabela `Authorized Pod pages` com nove linhas autorizadas |

O gate inicial também impede que rotas de recursos montem enquanto status e
seleção ainda não foram resolvidos. Rotas de setup (`/` e `/namespaces`) ficam
acessíveis para carregar ou reparar o contexto. Assim, a restauração de default
acontece antes da primeira LIST, e estados offline/erro continuam alcançáveis.
